use crate::commands::credit::format_idr;
use crate::output::*;

const PRICE_PER_CPU: i64 = 20000;
const PRICE_PER_GB_RAM: i64 = 5000;
const PRICE_PER_GB_STORAGE: i64 = 1000;
const FREE_BANDWIDTH_MBPS: i64 = 20;
const PRICE_PER_20MBPS: i64 = 20000;

pub fn calculate_vps_price(cpu: i64, ram_mb: i64, storage_gb: i64, bandwidth_mbps: i64) -> i64 {
    // Convert RAM from MB to GB (round up, minimum 1GB)
    let mut ram_gb = ram_mb / 1024;
    if ram_mb % 1024 > 0 {
        ram_gb += 1;
    }
    if ram_gb < 1 {
        ram_gb = 1;
    }

    let cpu_price = cpu * PRICE_PER_CPU;
    let ram_price = ram_gb * PRICE_PER_GB_RAM;
    let storage_price = storage_gb * PRICE_PER_GB_STORAGE;

    let bandwidth_price = if bandwidth_mbps > FREE_BANDWIDTH_MBPS {
        let extra = (bandwidth_mbps - FREE_BANDWIDTH_MBPS) / 20;
        extra * PRICE_PER_20MBPS
    } else {
        0
    };

    cpu_price + ram_price + storage_price + bandwidth_price
}

pub fn cmd_price(args: &[String]) -> Result<(), String> {
    if !args.is_empty() && (args[0] == "--help" || args[0] == "-h") {
        crate::commands::help::print_price_help();
        return Ok(());
    }

    if !args.is_empty() {
        return calculate_custom_price(args);
    }

    print_pricing_info();
    Ok(())
}

fn calculate_custom_price(args: &[String]) -> Result<(), String> {
    let mut cpu: i64 = 0;
    let mut ram: i64 = 0;
    let mut storage: i64 = 0;
    let mut bandwidth: i64 = 0;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--cpu" | "-c" => {
                if i + 1 < args.len() {
                    cpu = args[i + 1].parse().unwrap_or(0);
                    i += 1;
                }
            }
            "--ram" | "-r" => {
                if i + 1 < args.len() {
                    ram = parse_size(&args[i + 1]);
                    i += 1;
                }
            }
            "--storage" | "-s" => {
                if i + 1 < args.len() {
                    storage = parse_size(&args[i + 1]) / 1024; // Convert MB to GB
                    i += 1;
                }
            }
            "--bandwidth" | "-b" => {
                if i + 1 < args.len() {
                    bandwidth = args[i + 1].parse().unwrap_or(0);
                    i += 1;
                }
            }
            _ => {}
        }
        i += 1;
    }

    // Set defaults if not specified
    if cpu == 0 {
        cpu = 1;
    }
    if ram == 0 {
        ram = 512;
    }
    if storage == 0 {
        storage = 5;
    }
    if bandwidth == 0 {
        bandwidth = 20;
    }

    let price = calculate_vps_price(cpu, ram, storage, bandwidth);

    println!("\n{}VPS Price Calculation{}", BOLD, RESET);
    println!("{}", "\u{2500}".repeat(45));
    println!("  {:<20} {:>10} {:>12}", "Resource", "Quantity", "Cost");
    println!("{}", "\u{2500}".repeat(45));

    let mut ram_gb = ram / 1024;
    if ram % 1024 > 0 {
        ram_gb += 1;
    }
    if ram_gb < 1 {
        ram_gb = 1;
    }

    println!(
        "  {:<20} {:>10} {:>12}",
        "vCPU",
        cpu,
        format_idr(cpu * PRICE_PER_CPU)
    );
    println!(
        "  {:<20} {:>10} {:>12}",
        "RAM (GB)",
        ram_gb,
        format_idr(ram_gb * PRICE_PER_GB_RAM)
    );
    println!(
        "  {:<20} {:>10} {:>12}",
        "Storage (GB)",
        storage,
        format_idr(storage * PRICE_PER_GB_STORAGE)
    );

    let bandwidth_cost = if bandwidth > FREE_BANDWIDTH_MBPS {
        let extra = (bandwidth - FREE_BANDWIDTH_MBPS) / 20;
        extra * PRICE_PER_20MBPS
    } else {
        0
    };
    let bw_display = format!("{} (20 free)", bandwidth);
    println!(
        "  {:<20} {:>10} {:>12}",
        "Bandwidth (Mbps)",
        bw_display,
        format_idr(bandwidth_cost)
    );

    println!("{}", "\u{2500}".repeat(45));
    println!(
        "  {:<20} {:>10} {}{}{:>12}{}",
        "Total/month",
        "",
        GREEN,
        BOLD,
        format_idr(price),
        RESET
    );
    println!();

    Ok(())
}

fn print_pricing_info() {
    println!("\n{}Dalang VPS Pricing{}", BOLD, RESET);
    println!("{}", "\u{2500}".repeat(55));
    println!();
    println!(
        "{}Pay-as-you-go pricing based on resources:{}",
        YELLOW, RESET
    );
    println!();

    println!(
        "  {:<25} {}",
        "vCPU",
        format!("{}/vCPU/month", format_idr(PRICE_PER_CPU))
    );
    println!(
        "  {:<25} {}",
        "RAM",
        format!("{}/GB/month", format_idr(PRICE_PER_GB_RAM))
    );
    println!(
        "  {:<25} {}",
        "Storage (SSD)",
        format!("{}/GB/month", format_idr(PRICE_PER_GB_STORAGE))
    );
    println!("  {:<25} {}", "Bandwidth", "20 Mbps included FREE");
    println!(
        "  {:<25} {}",
        "",
        format!("+{} per additional 20 Mbps", format_idr(PRICE_PER_20MBPS))
    );
    println!();

    println!("{}Example Configurations:{}", YELLOW, RESET);
    println!();

    let p1 = calculate_vps_price(1, 512, 5, 20);
    println!(
        "  {}Starter{} (1 vCPU, 1GB RAM, 5GB, 20Mbps)",
        CYAN, RESET
    );
    println!("    \u{2192} {}{}/month{}\n", GREEN, format_idr(p1), RESET);

    let p2 = calculate_vps_price(1, 1024, 10, 20);
    println!(
        "  {}Basic{} (1 vCPU, 1GB RAM, 10GB, 20Mbps)",
        CYAN, RESET
    );
    println!("    \u{2192} {}{}/month{}\n", GREEN, format_idr(p2), RESET);

    let p3 = calculate_vps_price(2, 2048, 20, 40);
    println!(
        "  {}Standard{} (2 vCPU, 2GB RAM, 20GB, 40Mbps)",
        CYAN, RESET
    );
    println!("    \u{2192} {}{}/month{}\n", GREEN, format_idr(p3), RESET);

    let p4 = calculate_vps_price(4, 4096, 50, 100);
    println!(
        "  {}Pro{} (4 vCPU, 4GB RAM, 50GB, 100Mbps)",
        CYAN, RESET
    );
    println!("    \u{2192} {}{}/month{}\n", GREEN, format_idr(p4), RESET);

    println!("{}Calculate your price:{}", YELLOW, RESET);
    println!("  dalang price --cpu 2 --ram 2G --storage 20G --bandwidth 40\n");

    println!("{}Formula:{}", YELLOW, RESET);
    println!("  Price = (vCPU \u{00d7} 20K) + (RAM_GB \u{00d7} 5K) + (Storage_GB \u{00d7} 1K)");
    println!("        + Extra bandwidth: (Mbps - 20) / 20 \u{00d7} 20K");
    println!();
}

pub fn parse_size(s: &str) -> i64 {
    let s = s.trim().to_uppercase();
    let mut value_str = String::new();
    let mut unit = String::new();

    for c in s.chars() {
        if c.is_ascii_digit() {
            value_str.push(c);
        } else {
            unit.push(c);
        }
    }

    let value: i64 = value_str.parse().unwrap_or(0);

    match unit.as_str() {
        "G" | "GB" => value * 1024, // Return in MB
        "M" | "MB" => value,
        _ => value,
    }
}
