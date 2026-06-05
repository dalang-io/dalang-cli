package cmd

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/dalang-io/dalang-cli/internal/api"
)

// vmMetrics holds live VM stats not exposed by /vps/usage. They are read from
// inside the VM via the existing exec endpoint (no API change required).
type vmMetrics struct {
	uptimeSec float64
	load1     float64
	load5     float64
	load15    float64
	nproc     int
}

// fetchVMMetrics runs a tiny read-only command in the VM to collect uptime, load
// average, and CPU count. It is best-effort: any failure returns ok=false and
// the caller simply skips the extra display.
func fetchVMMetrics(client *api.Client, uuid string) (vmMetrics, bool) {
	resp, err := client.PostWithTimeout("/vps/session/exec", map[string]string{
		"uuid":    uuid,
		"command": "cat /proc/uptime; cat /proc/loadavg; nproc",
	}, 8*time.Second)
	if err != nil {
		return vmMetrics{}, false
	}

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Output string `json:"output"`
		} `json:"data"`
	}
	if json.Unmarshal(resp, &result) != nil || !result.Success {
		return vmMetrics{}, false
	}

	m := parseVMMetrics(result.Data.Output)
	if m.uptimeSec == 0 && m.nproc == 0 {
		return vmMetrics{}, false
	}
	return m, true
}

// parseVMMetrics extracts metrics from the combined output of
// `cat /proc/uptime; cat /proc/loadavg; nproc`. Lines are matched by content
// (not position) so it tolerates extra prompt/echo noise.
func parseVMMetrics(out string) vmMetrics {
	var m vmMetrics
	for _, line := range strings.Split(out, "\n") {
		f := strings.Fields(line)
		switch {
		case len(f) == 1:
			// nproc: a lone integer
			if n, err := strconv.Atoi(f[0]); err == nil {
				m.nproc = n
			}
		case len(f) == 2:
			// /proc/uptime: "<up_seconds> <idle_seconds>"
			if up, err := strconv.ParseFloat(f[0], 64); err == nil {
				if _, err2 := strconv.ParseFloat(f[1], 64); err2 == nil {
					m.uptimeSec = up
				}
			}
		case len(f) >= 5 && strings.Contains(f[3], "/"):
			// /proc/loadavg: "0.20 0.15 0.10 1/234 5678"
			m.load1, _ = strconv.ParseFloat(f[0], 64)
			m.load5, _ = strconv.ParseFloat(f[1], 64)
			m.load15, _ = strconv.ParseFloat(f[2], 64)
		}
	}
	return m
}

// formatUptime renders a duration in seconds as a compact "5d 2h 11m" string.
func formatUptime(sec float64) string {
	total := int(sec)
	d := total / 86400
	h := (total % 86400) / 3600
	min := (total % 3600) / 60
	switch {
	case d > 0:
		return fmt.Sprintf("%dd %dh %dm", d, h, min)
	case h > 0:
		return fmt.Sprintf("%dh %dm", h, min)
	default:
		return fmt.Sprintf("%dm", min)
	}
}

// printVMMetrics prints the Uptime and CPU-load lines for the resource panel.
func printVMMetrics(m vmMetrics) {
	if m.uptimeSec > 0 {
		fmt.Printf("  Uptime: %s\n", formatUptime(m.uptimeSec))
	}
	if m.nproc > 0 {
		pct := m.load1 / float64(m.nproc) * 100
		fmt.Printf("  CPU:    %s %.0f%% (load %.2f, %.2f, %.2f over %d vCPU)\n",
			renderBar(pct, 20), pct, m.load1, m.load5, m.load15, m.nproc)
	}
}
