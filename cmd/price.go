package cmd

import (
	"fmt"
	"strings"
)

// Pricing constants (in IDR per month)
const (
	PricePerCPU         = 20000 // Rp 20.000 per vCPU
	PricePerGBRAM       = 5000  // Rp 5.000 per GB RAM
	PricePerGBStorage   = 1000  // Rp 1.000 per GB storage
	FreeBandwidthMbps   = 20    // 20 Mbps included free
	PricePer20Mbps      = 20000 // Rp 20.000 per additional 20 Mbps
)

// CalculateVPSPrice calculates the monthly price for a VPS
// cpu: number of vCPUs
// ramMB: RAM in megabytes
// storageGB: storage in gigabytes
// bandwidthMbps: bandwidth in Mbps
func CalculateVPSPrice(cpu, ramMB, storageGB, bandwidthMbps int) int {
	// Convert RAM from MB to GB for calculation (round up, minimum 1GB)
	ramGB := ramMB / 1024
	if ramMB%1024 > 0 {
		ramGB++
	}
	if ramGB < 1 {
		ramGB = 1
	}

	cpuPrice := cpu * PricePerCPU
	ramPrice := ramGB * PricePerGBRAM
	storagePrice := storageGB * PricePerGBStorage

	// Bandwidth: 20 Mbps free, +20,000 per additional 20 Mbps
	bandwidthPrice := 0
	if bandwidthMbps > FreeBandwidthMbps {
		extraBandwidth := (bandwidthMbps - FreeBandwidthMbps) / 20
		bandwidthPrice = extraBandwidth * PricePer20Mbps
	}

	return cpuPrice + ramPrice + storagePrice + bandwidthPrice
}

func cmdPrice(args []string) error {
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printPriceHelp()
		return nil
	}

	// If specific specs provided, calculate price
	if len(args) >= 1 {
		return calculateCustomPrice(args)
	}

	// Show pricing table
	printPricingInfo()
	return nil
}

func calculateCustomPrice(args []string) error {
	var cpu, ram, storage, bandwidth int

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--cpu", "-c":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &cpu)
				i++
			}
		case "--ram", "-r":
			if i+1 < len(args) {
				ram = parseSize(args[i+1])
				i++
			}
		case "--storage", "-s":
			if i+1 < len(args) {
				storage = parseSize(args[i+1]) / 1024 // Convert MB to GB
				i++
			}
		case "--bandwidth", "-b":
			if i+1 < len(args) {
				fmt.Sscanf(args[i+1], "%d", &bandwidth)
				i++
			}
		}
	}

	// Set defaults if not specified
	if cpu == 0 {
		cpu = 1
	}
	if ram == 0 {
		ram = 512
	}
	if storage == 0 {
		storage = 5
	}
	if bandwidth == 0 {
		bandwidth = 20
	}

	price := CalculateVPSPrice(cpu, ram, storage, bandwidth)

	fmt.Printf("\n%sVPS Price Calculation%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 45))
	fmt.Printf("  %-20s %10s %12s\n", "Resource", "Quantity", "Cost")
	fmt.Println(strings.Repeat("─", 45))

	ramGB := ram / 1024
	if ram%1024 > 0 {
		ramGB++
	}
	if ramGB < 1 {
		ramGB = 1
	}

	fmt.Printf("  %-20s %10d %12s\n", "vCPU", cpu, formatIDR(int64(cpu*PricePerCPU)))
	fmt.Printf("  %-20s %10d %12s\n", "RAM (GB)", ramGB, formatIDR(int64(ramGB*PricePerGBRAM)))
	fmt.Printf("  %-20s %10d %12s\n", "Storage (GB)", storage, formatIDR(int64(storage*PricePerGBStorage)))

	// Bandwidth calculation
	bandwidthCost := 0
	if bandwidth > FreeBandwidthMbps {
		extraBandwidth := (bandwidth - FreeBandwidthMbps) / 20
		bandwidthCost = extraBandwidth * PricePer20Mbps
	}
	bandwidthDisplay := fmt.Sprintf("%d (20 free)", bandwidth)
	fmt.Printf("  %-20s %10s %12s\n", "Bandwidth (Mbps)", bandwidthDisplay, formatIDR(int64(bandwidthCost)))

	fmt.Println(strings.Repeat("─", 45))
	fmt.Printf("  %-20s %10s %s%12s%s\n", "Total/month", "", colorGreen+colorBold, formatIDR(int64(price)), colorReset)
	fmt.Println()

	return nil
}

func printPricingInfo() {
	fmt.Printf("\n%sDalang VPS Pricing%s\n", colorBold, colorReset)
	fmt.Println(strings.Repeat("─", 55))
	fmt.Println()
	fmt.Printf("%sPay-as-you-go pricing based on resources:%s\n\n", colorYellow, colorReset)

	fmt.Printf("  %-25s %s\n", "vCPU", formatIDR(PricePerCPU)+"/vCPU/month")
	fmt.Printf("  %-25s %s\n", "RAM", formatIDR(PricePerGBRAM)+"/GB/month")
	fmt.Printf("  %-25s %s\n", "Storage (SSD)", formatIDR(PricePerGBStorage)+"/GB/month")
	fmt.Printf("  %-25s %s\n", "Bandwidth", "20 Mbps included FREE")
	fmt.Printf("  %-25s %s\n", "", "+"+formatIDR(PricePer20Mbps)+" per additional 20 Mbps")
	fmt.Println()

	fmt.Printf("%sExample Configurations:%s\n\n", colorYellow, colorReset)

	// Example 1: Starter
	p1 := CalculateVPSPrice(1, 512, 5, 20)
	fmt.Printf("  %sStarter%s (1 vCPU, 1GB RAM, 5GB, 20Mbps)\n", colorCyan, colorReset)
	fmt.Printf("    → %s%s/month%s\n\n", colorGreen, formatIDR(int64(p1)), colorReset)

	// Example 2: Basic
	p2 := CalculateVPSPrice(1, 1024, 10, 20)
	fmt.Printf("  %sBasic%s (1 vCPU, 1GB RAM, 10GB, 20Mbps)\n", colorCyan, colorReset)
	fmt.Printf("    → %s%s/month%s\n\n", colorGreen, formatIDR(int64(p2)), colorReset)

	// Example 3: Standard
	p3 := CalculateVPSPrice(2, 2048, 20, 40)
	fmt.Printf("  %sStandard%s (2 vCPU, 2GB RAM, 20GB, 40Mbps)\n", colorCyan, colorReset)
	fmt.Printf("    → %s%s/month%s\n\n", colorGreen, formatIDR(int64(p3)), colorReset)

	// Example 4: Pro
	p4 := CalculateVPSPrice(4, 4096, 50, 100)
	fmt.Printf("  %sPro%s (4 vCPU, 4GB RAM, 50GB, 100Mbps)\n", colorCyan, colorReset)
	fmt.Printf("    → %s%s/month%s\n\n", colorGreen, formatIDR(int64(p4)), colorReset)

	fmt.Printf("%sCalculate your price:%s\n", colorYellow, colorReset)
	fmt.Printf("  dalang price --cpu 2 --ram 2G --storage 20G --bandwidth 40\n\n")

	fmt.Printf("%sFormula:%s\n", colorYellow, colorReset)
	fmt.Println("  Price = (vCPU × 20K) + (RAM_GB × 5K) + (Storage_GB × 1K)")
	fmt.Println("        + Extra bandwidth: (Mbps - 20) / 20 × 20K")
	fmt.Println()
}

func printPriceHelp() {
	fmt.Printf(`%sdalang price%s - Calculate VPS pricing

%sUSAGE:%s
    dalang price                    Show pricing table
    dalang price [options]          Calculate price for specific config

%sOPTIONS:%s
    --cpu, -c <count>        Number of vCPUs
    --ram, -r <size>         RAM size (e.g., 512M, 1G, 2G)
    --storage, -s <size>     Storage size (e.g., 10G, 20G)
    --bandwidth, -b <mbps>   Bandwidth in Mbps

%sPRICING:%s
    vCPU:       Rp 20.000/vCPU/month
    RAM:        Rp 5.000/GB/month
    Storage:    Rp 1.000/GB/month
    Bandwidth:  20 Mbps included FREE
                +Rp 20.000 per additional 20 Mbps

%sEXAMPLES:%s
    dalang price
    dalang price --cpu 2 --ram 2G --storage 20G
    dalang price -c 4 -r 4G -s 50G -b 100
`,
		colorCyan, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
		colorYellow, colorReset,
	)
}
