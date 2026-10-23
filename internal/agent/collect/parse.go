// Package collect reads host metrics directly from /proc and statfs.
// It deliberately uses only the standard library and never executes commands.
package collect

import (
	"bufio"
	"errors"
	"io"
	"math"
	"strconv"
	"strings"
)

type cpuTimes struct{ total, idle uint64 }

// parseCPUStat parses /proc/stat and returns aggregate CPU times and btime.
func parseCPUStat(r io.Reader) (cpuTimes, int64, error) {
	var ct cpuTimes
	var boot int64
	found := false
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "cpu "):
			for i, f := range strings.Fields(line)[1:] {
				// guest and guest_nice (fields 9, 10) are already counted in user/nice.
				if i >= 8 {
					break
				}
				v, err := strconv.ParseUint(f, 10, 64)
				if err != nil {
					return ct, 0, err
				}
				ct.total += v
				if i == 3 || i == 4 { // idle, iowait
					ct.idle += v
				}
			}
			found = true
		case strings.HasPrefix(line, "btime "):
			boot, _ = strconv.ParseInt(strings.TrimSpace(line[len("btime "):]), 10, 64)
		}
	}
	if err := sc.Err(); err != nil {
		return ct, 0, err
	}
	if !found {
		return ct, 0, errors.New("no cpu line in /proc/stat")
	}
	return ct, boot, nil
}

func cpuPercent(prev, cur cpuTimes) float64 {
	if cur.total <= prev.total {
		return 0
	}
	dt := cur.total - prev.total
	var di uint64
	if cur.idle > prev.idle {
		di = cur.idle - prev.idle
	}
	if di > dt {
		di = dt
	}
	return round2(float64(dt-di) / float64(dt) * 100)
}

type memInfo struct{ total, used, swapTotal, swapUsed uint64 }

func parseMeminfo(r io.Reader) (memInfo, error) {
	vals := map[string]uint64{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		key, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		f := strings.Fields(rest)
		if len(f) == 0 {
			continue
		}
		v, err := strconv.ParseUint(f[0], 10, 64)
		if err != nil {
			continue
		}
		if len(f) > 1 && f[1] == "kB" {
			v *= 1024
		}
		vals[key] = v
	}
	if err := sc.Err(); err != nil {
		return memInfo{}, err
	}
	total, ok := vals["MemTotal"]
	if !ok {
		return memInfo{}, errors.New("no MemTotal in /proc/meminfo")
	}
	avail, ok := vals["MemAvailable"]
	if !ok {
		avail = vals["MemFree"] + vals["Buffers"] + vals["Cached"]
	}
	avail = min(avail, total)
	swapTotal := vals["SwapTotal"]
	swapFree := min(vals["SwapFree"], swapTotal)
	return memInfo{total: total, used: total - avail, swapTotal: swapTotal, swapUsed: swapTotal - swapFree}, nil
}

func parseLoadavg(s string) (l1, l5, l15 float64, err error) {
	f := strings.Fields(s)
	if len(f) < 3 {
		return 0, 0, 0, errors.New("malformed /proc/loadavg")
	}
	if l1, err = strconv.ParseFloat(f[0], 64); err != nil {
		return
	}
	if l5, err = strconv.ParseFloat(f[1], 64); err != nil {
		return
	}
	l15, err = strconv.ParseFloat(f[2], 64)
	return
}

func parseUptime(s string) (uint64, error) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return 0, errors.New("malformed /proc/uptime")
	}
	v, err := strconv.ParseFloat(f[0], 64)
	if err != nil || v < 0 {
		return 0, errors.New("malformed /proc/uptime")
	}
	return uint64(v), nil
}

// virtualNICPrefixes lists interfaces whose traffic is already counted on a
// physical interface (bridges, tunnels, container veths) or is local only.
var virtualNICPrefixes = []string{
	"docker", "veth", "br-", "virbr", "vnet", "vmbr", "fwbr", "fwpr", "fwln",
	"tun", "tap", "wg", "tailscale", "zt", "cni", "flannel", "cali", "kube",
	"lxc", "lxd", "ifb", "dummy", "sit", "ip6tnl", "gre", "erspan", "nlmon",
}

// DefaultNIC reports whether an interface is counted when no explicit list is configured.
func DefaultNIC(name string) bool {
	if name == "lo" {
		return false
	}
	for _, p := range virtualNICPrefixes {
		if strings.HasPrefix(name, p) {
			return false
		}
	}
	return true
}

func parseNetDev(r io.Reader, include func(string) bool) (rx, tx uint64, err error) {
	sc := bufio.NewScanner(r)
	for line := 0; sc.Scan(); line++ {
		if line < 2 {
			continue
		}
		name, rest, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		if !include(strings.TrimSpace(name)) {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 9 {
			continue
		}
		r, err1 := strconv.ParseUint(f[0], 10, 64)
		t, err2 := strconv.ParseUint(f[8], 10, 64)
		if err1 != nil || err2 != nil {
			continue
		}
		rx += r
		tx += t
	}
	return rx, tx, sc.Err()
}

// parseSockstat sums "inuse" sockets from /proc/net/sockstat and sockstat6.
func parseSockstat(r io.Reader) (tcp, udp uint32) {
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || f[1] != "inuse" {
			continue
		}
		v, err := strconv.ParseUint(f[2], 10, 32)
		if err != nil {
			continue
		}
		switch f[0] {
		case "TCP:", "TCP6:":
			tcp += uint32(v)
		case "UDP:", "UDP6:":
			udp += uint32(v)
		}
	}
	return tcp, udp
}

type mount struct{ device, point, fstype string }

var diskFSTypes = map[string]bool{
	"ext2": true, "ext3": true, "ext4": true, "xfs": true, "btrfs": true, "zfs": true,
	"f2fs": true, "jfs": true, "reiserfs": true, "bcachefs": true, "simfs": true,
	"vfat": true, "exfat": true, "ntfs": true, "ntfs3": true, "fuseblk": true,
}

var mountUnescaper = strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)

// parseMounts returns real disk filesystems from /proc/mounts, one per device.
func parseMounts(r io.Reader) []mount {
	seen := map[string]bool{}
	var out []mount
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		f := strings.Fields(sc.Text())
		if len(f) < 3 || !diskFSTypes[f[2]] || seen[f[0]] {
			continue
		}
		seen[f[0]] = true
		out = append(out, mount{device: f[0], point: mountUnescaper.Replace(f[1]), fstype: f[2]})
	}
	return out
}

func parseOSRelease(r io.Reader) string {
	vals := map[string]string{}
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), "=")
		if !ok {
			continue
		}
		if u, err := strconv.Unquote(v); err == nil {
			v = u
		} else {
			v = strings.Trim(v, `"'`)
		}
		vals[k] = v
	}
	if v := vals["PRETTY_NAME"]; v != "" {
		return v
	}
	return strings.TrimSpace(vals["NAME"] + " " + vals["VERSION_ID"])
}

var armParts = map[string]string{
	"0xd03": "Cortex-A53", "0xd04": "Cortex-A35", "0xd05": "Cortex-A55", "0xd07": "Cortex-A57",
	"0xd08": "Cortex-A72", "0xd09": "Cortex-A73", "0xd0a": "Cortex-A75", "0xd0b": "Cortex-A76",
	"0xd0c": "Neoverse-N1", "0xd0d": "Cortex-A77", "0xd40": "Neoverse-V1", "0xd41": "Cortex-A78",
	"0xd49": "Neoverse-N2", "0xd4f": "Neoverse-V2", "0xd8e": "Neoverse-N3", "0xd84": "Neoverse-V3",
}

// parseCPUInfo extracts a CPU model name and whether the hypervisor flag is present.
func parseCPUInfo(r io.Reader) (model string, hypervisor bool) {
	var part string
	sc := bufio.NewScanner(r)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		k, v = strings.TrimSpace(k), strings.TrimSpace(v)
		switch k {
		case "model name", "Model", "cpu model":
			if model == "" {
				model = v
			}
		case "CPU part":
			if part == "" {
				part = v
			}
		case "flags":
			if !hypervisor {
				hypervisor = strings.Contains(" "+v+" ", " hypervisor ")
			}
		}
	}
	if model == "" && part != "" {
		model = armParts[strings.ToLower(part)]
	}
	return model, hypervisor
}

func round2(v float64) float64 { return math.Round(v*100) / 100 }

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !isRuneStart(s[n]) {
		n--
	}
	return s[:n]
}

func isRuneStart(b byte) bool { return b&0xC0 != 0x80 }
