package collect

import (
	"strings"
	"testing"
)

func TestParseCPUStat(t *testing.T) {
	in := "cpu  100 0 50 800 50 0 0 0 30 0\ncpu0 1 2 3 4\nbtime 1700000000\n"
	ct, boot, err := parseCPUStat(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if ct.total != 1000 || ct.idle != 850 || boot != 1700000000 {
		t.Fatalf("got %+v boot=%d", ct, boot)
	}
	next := cpuTimes{total: 1100, idle: 900}
	if got := cpuPercent(ct, next); got != 50 {
		t.Fatalf("cpuPercent = %v, want 50", got)
	}
	if got := cpuPercent(next, ct); got != 0 {
		t.Fatalf("cpuPercent backwards = %v, want 0", got)
	}
}

func TestParseMeminfo(t *testing.T) {
	in := `MemTotal:        2000 kB
MemFree:          100 kB
MemAvailable:     500 kB
SwapTotal:       1000 kB
SwapFree:         750 kB
`
	m, err := parseMeminfo(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if m.total != 2000*1024 || m.used != 1500*1024 || m.swapTotal != 1000*1024 || m.swapUsed != 250*1024 {
		t.Fatalf("got %+v", m)
	}
}

func TestParseMeminfoWithoutAvailable(t *testing.T) {
	in := "MemTotal: 1000 kB\nMemFree: 100 kB\nBuffers: 50 kB\nCached: 250 kB\n"
	m, err := parseMeminfo(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if m.used != 600*1024 {
		t.Fatalf("used = %d", m.used)
	}
}

func TestParseNetDev(t *testing.T) {
	in := `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:  999 1 0 0 0 0 0 0  999 1 0 0 0 0 0 0
  eth0: 1000 10 0 0 0 0 0 0 2000 20 0 0 0 0 0 0
docker0: 500 5 0 0 0 0 0 0  500 5 0 0 0 0 0 0
vethab12: 7 1 0 0 0 0 0 0 7 1 0 0 0 0 0 0
  ens4: 1 1 0 0 0 0 0 0 2 1 0 0 0 0 0 0
`
	rx, tx, err := parseNetDev(strings.NewReader(in), DefaultNIC)
	if err != nil {
		t.Fatal(err)
	}
	if rx != 1001 || tx != 2002 {
		t.Fatalf("rx=%d tx=%d", rx, tx)
	}
	only := func(n string) bool { return n == "docker0" }
	rx, tx, _ = parseNetDev(strings.NewReader(in), only)
	if rx != 500 || tx != 500 {
		t.Fatalf("explicit rx=%d tx=%d", rx, tx)
	}
}

func TestParseSockstat(t *testing.T) {
	in := "sockets: used 300\nTCP: inuse 12 orphan 0 tw 3 alloc 20 mem 4\nUDP: inuse 4 mem 2\nUDPLITE: inuse 0\nTCP6: inuse 3\nUDP6: inuse 1\n"
	tcp, udp := parseSockstat(strings.NewReader(in))
	if tcp != 15 || udp != 5 {
		t.Fatalf("tcp=%d udp=%d", tcp, udp)
	}
}

func TestParseMounts(t *testing.T) {
	in := `/dev/vda1 / ext4 rw,relatime 0 0
proc /proc proc rw 0 0
tmpfs /run tmpfs rw 0 0
/dev/vda1 /var/lib/docker/overlay ext4 rw 0 0
overlay /var/lib/docker/overlay2/x/merged overlay rw 0 0
/dev/vdb1 /mnt/my\040data xfs rw 0 0
`
	got := parseMounts(strings.NewReader(in))
	if len(got) != 2 || got[0].point != "/" || got[1].point != "/mnt/my data" {
		t.Fatalf("got %+v", got)
	}
}

func TestParseOSRelease(t *testing.T) {
	in := "NAME=\"Debian GNU/Linux\"\nVERSION_ID=\"13\"\nPRETTY_NAME=\"Debian GNU/Linux 13 (trixie)\"\n"
	if got := parseOSRelease(strings.NewReader(in)); got != "Debian GNU/Linux 13 (trixie)" {
		t.Fatalf("got %q", got)
	}
	if got := parseOSRelease(strings.NewReader("NAME=Alpine\nVERSION_ID=3.22\n")); got != "Alpine 3.22" {
		t.Fatalf("got %q", got)
	}
}

func TestParseCPUInfo(t *testing.T) {
	x86 := "processor\t: 0\nmodel name\t: AMD EPYC 7B13\nflags\t\t: fpu vme hypervisor lahf_lm\n"
	model, hv := parseCPUInfo(strings.NewReader(x86))
	if model != "AMD EPYC 7B13" || !hv {
		t.Fatalf("x86 got %q %v", model, hv)
	}
	arm := "processor\t: 0\nCPU implementer\t: 0x41\nCPU part\t: 0xd0c\n"
	model, hv = parseCPUInfo(strings.NewReader(arm))
	if model != "Neoverse-N1" || hv {
		t.Fatalf("arm got %q %v", model, hv)
	}
}

func TestParseLoadUptime(t *testing.T) {
	l1, l5, l15, err := parseLoadavg("0.10 0.20 0.30 1/200 12345\n")
	if err != nil || l1 != 0.1 || l5 != 0.2 || l15 != 0.3 {
		t.Fatalf("load %v %v %v %v", l1, l5, l15, err)
	}
	up, err := parseUptime("12345.67 2000.00\n")
	if err != nil || up != 12345 {
		t.Fatalf("uptime %v %v", up, err)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("ab中文", 4); got != "ab" {
		t.Fatalf("got %q", got)
	}
	if got := truncate("abc", 10); got != "abc" {
		t.Fatalf("got %q", got)
	}
}
