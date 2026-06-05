package cmd

import "testing"

func TestParseVMMetrics(t *testing.T) {
	out := "12345.67 98765.43\n0.20 0.15 0.10 1/234 5678\n4\n"
	m := parseVMMetrics(out)

	if m.uptimeSec != 12345.67 {
		t.Errorf("uptimeSec = %v, want 12345.67", m.uptimeSec)
	}
	if m.load1 != 0.20 || m.load5 != 0.15 || m.load15 != 0.10 {
		t.Errorf("load = %v/%v/%v, want 0.20/0.15/0.10", m.load1, m.load5, m.load15)
	}
	if m.nproc != 4 {
		t.Errorf("nproc = %d, want 4", m.nproc)
	}
}

func TestParseVMMetricsToleratesNoise(t *testing.T) {
	// Extra echo/prompt lines should be ignored.
	out := "$ cat /proc/uptime\n999.50 100.0\nuser pts/0\n0.00 0.01 0.05 2/100 42\n2\nlogout\n"
	m := parseVMMetrics(out)

	if m.uptimeSec != 999.50 {
		t.Errorf("uptimeSec = %v, want 999.50", m.uptimeSec)
	}
	if m.nproc != 2 {
		t.Errorf("nproc = %d, want 2", m.nproc)
	}
	if m.load1 != 0.00 || m.load15 != 0.05 {
		t.Errorf("load = %v/%v/%v", m.load1, m.load5, m.load15)
	}
}

func TestFormatUptime(t *testing.T) {
	cases := []struct {
		sec  float64
		want string
	}{
		{0, "0m"},
		{59, "0m"},
		{60, "1m"},
		{3600, "1h 0m"},
		{3661, "1h 1m"},
		{90061, "1d 1h 1m"},
		{5*86400 + 2*3600 + 11*60, "5d 2h 11m"},
	}
	for _, tc := range cases {
		if got := formatUptime(tc.sec); got != tc.want {
			t.Errorf("formatUptime(%v) = %q, want %q", tc.sec, got, tc.want)
		}
	}
}
