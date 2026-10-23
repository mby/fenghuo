package proto

import "testing"

func validReport() Report {
	return Report{
		V: Version, CPU: 12.5, Load1: 0.3,
		MemTotal: 100, MemUsed: 50, SwapTotal: 10, SwapUsed: 1, DiskTotal: 1000, DiskUsed: 10,
		Info: &Info{OS: "Debian GNU/Linux 13", Arch: "amd64", CPUCores: 2},
		Ping: []Ping{{Name: "CT", MS: 12.3}, {Name: "CU", MS: -1}},
	}
}

func TestValidate(t *testing.T) {
	ok := validReport()
	if err := ok.Validate(); err != nil {
		t.Fatalf("valid report rejected: %v", err)
	}

	cases := map[string]func(r *Report){
		"version":       func(r *Report) { r.V = 99 },
		"cpu":           func(r *Report) { r.CPU = 101 },
		"negative load": func(r *Report) { r.Load5 = -1 },
		"mem":           func(r *Report) { r.MemUsed = 101 },
		"disk":          func(r *Report) { r.DiskUsed = 1001 },
		"control char":  func(r *Report) { r.Info.OS = "bad\x1b[31m" },
		"long text":     func(r *Report) { r.Info.CPUModel = string(make([]byte, MaxTextLen+1)) },
		"ping range":    func(r *Report) { r.Ping[0].MS = 70000 },
		"ping name":     func(r *Report) { r.Ping[0].Name = "" },
		"too many pings": func(r *Report) {
			r.Ping = make([]Ping, MaxPingTargets+1)
			for i := range r.Ping {
				r.Ping[i] = Ping{Name: "x", MS: 1}
			}
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			r := validReport()
			r.Info = &Info{OS: r.Info.OS, Arch: r.Info.Arch, CPUCores: r.Info.CPUCores}
			r.Ping = append([]Ping(nil), r.Ping...)
			mutate(&r)
			if err := r.Validate(); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
