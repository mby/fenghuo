//go:build linux

package collect

import (
	"io"
	"os"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/mby/fenghuo/internal/proto"
	"github.com/mby/fenghuo/internal/version"
)

type Collector struct {
	includeNIC func(string) bool
	prevCPU    cpuTimes
	prevRx     uint64
	prevTx     uint64
	prevAt     time.Time
	info       proto.Info
}

// New creates a collector. nics restricts traffic accounting to the given
// interfaces; when empty, physical-looking interfaces are detected automatically.
func New(nics []string) (*Collector, error) {
	c := &Collector{includeNIC: DefaultNIC}
	if len(nics) > 0 {
		set := map[string]bool{}
		for _, n := range nics {
			set[n] = true
		}
		c.includeNIC = func(name string) bool { return set[name] }
	}
	c.info = readInfo()
	if _, err := c.Collect(time.Now()); err != nil {
		return nil, err
	}
	return c, nil
}

func (c *Collector) Info() proto.Info { return c.info }

func (c *Collector) Collect(now time.Time) (proto.Report, error) {
	r := proto.Report{V: proto.Version, Time: now.UnixMilli()}

	var ct cpuTimes
	err := withFile("/proc/stat", func(rd io.Reader) (err error) {
		ct, r.BootTime, err = parseCPUStat(rd)
		return err
	})
	if err != nil {
		return r, err
	}
	if !c.prevAt.IsZero() {
		r.CPU = cpuPercent(c.prevCPU, ct)
	}

	var mem memInfo
	err = withFile("/proc/meminfo", func(rd io.Reader) (err error) {
		mem, err = parseMeminfo(rd)
		return err
	})
	if err != nil {
		return r, err
	}
	r.MemTotal, r.MemUsed, r.SwapTotal, r.SwapUsed = mem.total, mem.used, mem.swapTotal, mem.swapUsed

	if b, err := os.ReadFile("/proc/loadavg"); err == nil {
		r.Load1, r.Load5, r.Load15, _ = parseLoadavg(string(b))
	}
	if b, err := os.ReadFile("/proc/uptime"); err == nil {
		r.Uptime, _ = parseUptime(string(b))
	}

	var rx, tx uint64
	err = withFile("/proc/net/dev", func(rd io.Reader) (err error) {
		rx, tx, err = parseNetDev(rd, c.includeNIC)
		return err
	})
	if err != nil {
		return r, err
	}
	r.NetRx, r.NetTx = rx, tx
	if !c.prevAt.IsZero() {
		if dt := now.Sub(c.prevAt).Seconds(); dt > 0 {
			if rx >= c.prevRx {
				r.NetIn = uint64(float64(rx-c.prevRx) / dt)
			}
			if tx >= c.prevTx {
				r.NetOut = uint64(float64(tx-c.prevTx) / dt)
			}
		}
	}

	for _, p := range []string{"/proc/net/sockstat", "/proc/net/sockstat6"} {
		if f, err := os.Open(p); err == nil {
			t, u := parseSockstat(f)
			f.Close()
			r.TCP += t
			r.UDP += u
		}
	}
	r.Procs = countProcs()
	r.DiskTotal, r.DiskUsed = diskUsage()

	c.prevCPU, c.prevRx, c.prevTx, c.prevAt = ct, rx, tx, now
	return r, nil
}

func withFile(path string, fn func(io.Reader) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return fn(f)
}

func countProcs() uint32 {
	f, err := os.Open("/proc")
	if err != nil {
		return 0
	}
	defer f.Close()
	names, _ := f.Readdirnames(-1)
	var n uint32
	for _, name := range names {
		if name != "" && name[0] >= '0' && name[0] <= '9' {
			n++
		}
	}
	return n
}

func diskUsage() (total, used uint64) {
	var mounts []mount
	if f, err := os.Open("/proc/self/mounts"); err == nil {
		mounts = parseMounts(f)
		f.Close()
	}
	if len(mounts) == 0 {
		mounts = []mount{{point: "/"}}
	}
	for _, m := range mounts {
		var st syscall.Statfs_t
		if syscall.Statfs(m.point, &st) != nil {
			continue
		}
		bs := uint64(st.Frsize)
		if bs == 0 {
			bs = uint64(st.Bsize)
		}
		if st.Blocks < st.Bfree {
			continue
		}
		u := (st.Blocks - st.Bfree) * bs
		// Like df: size = used + available to unprivileged users.
		total += u + st.Bavail*bs
		used += u
	}
	return total, used
}

func readInfo() proto.Info {
	info := proto.Info{
		Arch:         runtime.GOARCH,
		CPUCores:     runtime.NumCPU(),
		AgentVersion: version.Version,
	}
	for _, p := range []string{"/etc/os-release", "/usr/lib/os-release"} {
		if f, err := os.Open(p); err == nil {
			info.OS = parseOSRelease(f)
			f.Close()
			break
		}
	}
	info.Kernel = readTrim("/proc/sys/kernel/osrelease")
	hypervisor := false
	if f, err := os.Open("/proc/cpuinfo"); err == nil {
		info.CPUModel, hypervisor = parseCPUInfo(f)
		f.Close()
	}
	info.Virt = detectVirt(hypervisor)

	info.OS = truncate(info.OS, proto.MaxTextLen)
	info.Kernel = truncate(info.Kernel, proto.MaxTextLen)
	info.CPUModel = truncate(strings.Join(strings.Fields(info.CPUModel), " "), proto.MaxTextLen)
	info.Virt = truncate(info.Virt, proto.MaxTextLen)
	return info
}

func detectVirt(hypervisorFlag bool) string {
	if s := readTrim("/run/systemd/container"); s != "" {
		return strings.ToLower(s)
	}
	if exists("/proc/vz") && !exists("/proc/bc") {
		return "openvz"
	}
	if exists("/.dockerenv") {
		return "docker"
	}
	if exists("/run/.containerenv") {
		return "podman"
	}
	dmi := strings.ToLower(readTrim("/sys/class/dmi/id/sys_vendor") + " " + readTrim("/sys/class/dmi/id/product_name"))
	switch {
	case strings.Contains(dmi, "microsoft") && strings.Contains(dmi, "virtual"):
		return "hyper-v"
	case strings.Contains(dmi, "vmware"):
		return "vmware"
	case strings.Contains(dmi, "virtualbox"), strings.Contains(dmi, "innotek"):
		return "virtualbox"
	case strings.Contains(dmi, "parallels"):
		return "parallels"
	case strings.Contains(dmi, "xen"):
		return "xen"
	case strings.Contains(dmi, "bochs"):
		return "bochs"
	case strings.Contains(dmi, "kvm"), strings.Contains(dmi, "qemu"), strings.Contains(dmi, "openstack"),
		strings.Contains(dmi, "amazon ec2"), strings.Contains(dmi, "google"), strings.Contains(dmi, "alibaba"),
		strings.Contains(dmi, "tencent"):
		return "kvm"
	}
	if exists("/proc/xen") {
		return "xen"
	}
	if hypervisorFlag {
		return "vm"
	}
	return "none"
}

func readTrim(path string) string {
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
