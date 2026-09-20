package cmd

import "testing"

func TestCalculateVPSPrice(t *testing.T) {
	tests := []struct {
		name          string
		cpu           int
		ramMB         int
		storageGB     int
		bandwidthMbps int
		want          int
	}{
		{
			name:          "starter: 1 vCPU, 512MB, 5GB, 20Mbps",
			cpu:           1,
			ramMB:         512,
			storageGB:     5,
			bandwidthMbps: 20,
			want:          20000 + 5000 + 5000, // 1 CPU + 1GB RAM (rounded up) + 5GB storage
		},
		{
			name:          "basic: 1 vCPU, 1024MB, 10GB, 20Mbps",
			cpu:           1,
			ramMB:         1024,
			storageGB:     10,
			bandwidthMbps: 20,
			want:          20000 + 5000 + 10000,
		},
		{
			name:          "standard: 2 vCPU, 2048MB, 20GB, 40Mbps",
			cpu:           2,
			ramMB:         2048,
			storageGB:     20,
			bandwidthMbps: 40,
			want:          40000 + 10000 + 20000 + 20000, // +20K for extra 20Mbps
		},
		{
			name:          "pro: 4 vCPU, 4096MB, 50GB, 100Mbps",
			cpu:           4,
			ramMB:         4096,
			storageGB:     50,
			bandwidthMbps: 100,
			want:          80000 + 20000 + 50000 + 80000, // +80K for 80Mbps extra (4 blocks)
		},
		{
			name:          "fractional RAM rounds up",
			cpu:           1,
			ramMB:         1500,
			storageGB:     5,
			bandwidthMbps: 20,
			want:          20000 + 10000 + 5000, // 1500MB -> 2GB
		},
		{
			name:          "zero RAM defaults to 1GB minimum",
			cpu:           1,
			ramMB:         0,
			storageGB:     5,
			bandwidthMbps: 20,
			want:          20000 + 5000 + 5000,
		},
		{
			name:          "no extra bandwidth for exactly free tier",
			cpu:           1,
			ramMB:         1024,
			storageGB:     5,
			bandwidthMbps: 20,
			want:          20000 + 5000 + 5000,
		},
		{
			name:          "bandwidth below free tier no charge",
			cpu:           1,
			ramMB:         1024,
			storageGB:     5,
			bandwidthMbps: 10,
			want:          20000 + 5000 + 5000,
		},
		{
			// A partial block is FREE, because the backend truncates. This case
			// used to assert the opposite and so locked in a Rp 20.000 over-quote
			// for every bandwidth between 21 and 39 Mbps.
			name:          "partial bandwidth block is not charged",
			cpu:           1,
			ramMB:         1024,
			storageGB:     5,
			bandwidthMbps: 30,                  // 10 extra Mbps: (30-20)/20 = 0 blocks
			want:          20000 + 5000 + 5000, // same as 20 Mbps
		},
		{
			name:          "exact extra bandwidth block",
			cpu:           1,
			ramMB:         1024,
			storageGB:     5,
			bandwidthMbps: 60, // 40 extra = 2 blocks
			want:          20000 + 5000 + 5000 + 40000,
		},
		{
			name:          "large config",
			cpu:           8,
			ramMB:         16384,
			storageGB:     200,
			bandwidthMbps: 200,
			want:          160000 + 80000 + 200000 + 180000,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateVPSPrice(tt.cpu, tt.ramMB, tt.storageGB, tt.bandwidthMbps)
			if got != tt.want {
				t.Fatalf("CalculateVPSPrice(%d, %d, %d, %d) = %d, want %d",
					tt.cpu, tt.ramMB, tt.storageGB, tt.bandwidthMbps, got, tt.want)
			}
		})
	}
}

func TestCalculateCustomPrice(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		wantErr bool
	}{
		{
			name: "all flags",
			args: []string{"--cpu", "2", "--ram", "2G", "--storage", "20G", "--bandwidth", "40"},
		},
		{
			name: "short flags",
			args: []string{"-c", "2", "-r", "2G", "-s", "20G", "-b", "40"},
		},
		{
			name: "defaults when no flags",
			args: []string{"--cpu", "1"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := calculateCustomPrice(tt.args)
			if (err != nil) != tt.wantErr {
				t.Fatalf("calculateCustomPrice() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCmdPriceHelp(t *testing.T) {
	err := cmdPrice([]string{"--help"})
	if err != nil {
		t.Fatalf("cmdPrice(--help) returned error: %v", err)
	}

	err = cmdPrice([]string{"-h"})
	if err != nil {
		t.Fatalf("cmdPrice(-h) returned error: %v", err)
	}
}

// The CLI only ever displays an estimate — the customer is charged what the API
// computes — so an estimate that disagrees with the API is the one thing
// `dalang price` must not do. This mirrors calculateVPSPrice in
// api.dalang.io/handlers/vps_order.go, which the order path uses.
func backendVPSPrice(cpu, ramMB, storageGB, bandwidthMbps int) int {
	ramGB := ramMB / 1024
	if ramMB%1024 > 0 {
		ramGB++
	}
	if ramGB < 1 {
		ramGB = 1
	}
	bandwidthPrice := 0
	if bandwidthMbps > 20 {
		bandwidthPrice = ((bandwidthMbps - 20) / 20) * 20000
	}
	return cpu*20000 + ramGB*5000 + storageGB*1000 + bandwidthPrice
}

func TestCalculateVPSPrice_AgreesWithBackend(t *testing.T) {
	for _, tc := range []struct{ cpu, ramMB, storageGB, bw int }{
		{1, 1024, 5, 20},   // Starter
		{1, 1024, 10, 20},  // Basic
		{2, 2048, 20, 40},  // Standard
		{4, 4096, 50, 100}, // Pro
		{1, 1024, 10, 30},  // not a multiple of 20 — this is where they diverged
		{2, 2048, 20, 21},  // one Mbps over the free allowance
		{1, 1024, 10, 39},  // just under the second block
		{1, 512, 5, 20},    // under 1 GB of RAM
		{8, 16384, 200, 200},
	} {
		got := CalculateVPSPrice(tc.cpu, tc.ramMB, tc.storageGB, tc.bw)
		want := backendVPSPrice(tc.cpu, tc.ramMB, tc.storageGB, tc.bw)
		if got != want {
			t.Errorf("cpu=%d ram=%dMB storage=%dGB bw=%dMbps: CLI quotes %d, API charges %d (diff %+d)",
				tc.cpu, tc.ramMB, tc.storageGB, tc.bw, got, want, got-want)
		}
	}
}

func TestBandwidthBlocks_TruncatesLikeTheBackend(t *testing.T) {
	for bw, want := range map[int]int{
		0: 0, 20: 0, 21: 0, 30: 0, 39: 0, 40: 1, 59: 1, 60: 2, 100: 4, 200: 9,
	} {
		if got := bandwidthBlocks(bw); got != want {
			t.Errorf("bandwidthBlocks(%d) = %d, want %d", bw, got, want)
		}
	}
}
