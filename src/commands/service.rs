use serde::Serialize;

use crate::api::{self, Client};
use crate::commands::credit::{format_date, format_expiry_with_days, format_idr};
use crate::commands::price::{calculate_vps_price, parse_size};
use crate::output::*;

#[derive(Serialize)]
struct UnifiedService {
    #[serde(rename = "type")]
    svc_type: String,
    id: String,
    name: String,
    status: String,
    expires_at: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    details: Option<String>,
}

pub fn cmd_service(args: &[String]) -> Result<(), String> {
    if args.is_empty() {
        return service_list();
    }

    match args[0].as_str() {
        "list" | "ls" => service_list(),
        "info" => {
            if args.len() < 2 {
                print_error("Usage: dalang service info <name>");
                return Err("missing service name".to_string());
            }
            service_info(&args[1])
        }
        "create" => service_create(&args[1..]),
        "upgrade" => {
            if args.len() < 2 {
                print_error("Usage: dalang service upgrade <name> [options]");
                return Err("missing service name".to_string());
            }
            service_upgrade(&args[1], &args[2..])
        }
        "extend" => {
            if args.len() < 2 {
                print_error("Usage: dalang service extend <name> [--months N]");
                return Err("missing service name".to_string());
            }
            service_extend(&args[1], &args[2..])
        }
        "--help" | "-h" => {
            crate::commands::help::print_service_help();
            Ok(())
        }
        other => {
            print_error(&format!("Unknown service subcommand: {}", other));
            Err(format!("unknown subcommand: {}", other))
        }
    }
}

fn service_list() -> Result<(), String> {
    let client = Client::new_authenticated()?;

    let mut services: Vec<UnifiedService> = Vec::new();

    // Fetch VPS
    print_debug("Fetching VPS list...");
    match client.get("/vps/list") {
        Ok(resp) => match serde_json::from_slice::<api::VPSListResponse>(&resp) {
            Ok(vps_data) => {
                print_debug(&format!("Found {} VPS", vps_data.data.len()));
                for v in &vps_data.data {
                    let name = if !v.display_name.is_empty() {
                        &v.display_name
                    } else {
                        &v.name
                    };
                    services.push(UnifiedService {
                        svc_type: "vps".to_string(),
                        id: v.id.clone(),
                        name: name.to_string(),
                        status: v.status.clone(),
                        expires_at: v.expires_at.clone(),
                        details: Some(format!("{}C/{}MB/{}GB", v.vcpu, v.ram, v.storage)),
                    });
                }
            }
            Err(e) => print_debug(&format!("VPS parse error: {}", e)),
        },
        Err(e) => print_debug(&format!("VPS fetch error: {}", e)),
    }

    // Fetch containers
    print_debug("Fetching containers list...");
    match client.get("/containers/list") {
        Ok(resp) => match serde_json::from_slice::<api::ContainerListResponse>(&resp) {
            Ok(cont_data) => {
                print_debug(&format!(
                    "Found {} containers",
                    cont_data.data.containers.len()
                ));
                for c in &cont_data.data.containers {
                    services.push(UnifiedService {
                        svc_type: "container".to_string(),
                        id: c.id.clone(),
                        name: c.name.clone(),
                        status: c.status.clone(),
                        expires_at: c.expires_at.clone(),
                        details: Some(format!("{}/{}", c.container_type, c.plan)),
                    });
                }
            }
            Err(e) => print_debug(&format!("Containers parse error: {}", e)),
        },
        Err(e) => print_debug(&format!("Containers fetch error: {}", e)),
    }

    // Fetch deployments/apps
    print_debug("Fetching deployments list...");
    match client.get("/github/deployments") {
        Ok(resp) => match serde_json::from_slice::<api::DeploymentListResponse>(&resp) {
            Ok(app_data) => {
                print_debug(&format!("Found {} deployments", app_data.data.len()));
                for d in &app_data.data {
                    services.push(UnifiedService {
                        svc_type: "app".to_string(),
                        id: d.id.clone(),
                        name: d.name.clone(),
                        status: d.status.clone(),
                        expires_at: d.expires_at.clone(),
                        details: Some(d.branch.clone()),
                    });
                }
            }
            Err(e) => print_debug(&format!("Deployments parse error: {}", e)),
        },
        Err(e) => print_debug(&format!("Deployments fetch error: {}", e)),
    }

    if json_output() {
        println!(
            "{}",
            serde_json::to_string_pretty(&services).unwrap()
        );
        return Ok(());
    }

    if services.is_empty() {
        print_info("No services found");
        println!("\nCreate your first service with:");
        println!(
            "  {}dalang service create --name MyVM --cpu 2 --ram 1G{}\n",
            CYAN, RESET
        );
        return Ok(());
    }

    println!(
        "\n{}Your Services{} ({} total)",
        BOLD,
        RESET,
        services.len()
    );
    println!("{}", "\u{2500}".repeat(75));
    println!(
        "{:<10} {:<20} {:<12} {:<15} {}",
        "TYPE", "NAME", "STATUS", "EXPIRES", "DETAILS"
    );
    println!("{}", "\u{2500}".repeat(75));

    for s in &services {
        let status_color = match s.status.to_uppercase().as_str() {
            "RUNNING" | "ACTIVE" => GREEN,
            "STOPPED" => YELLOW,
            "CREATING" | "PENDING" => CYAN,
            "ERROR" | "FAILED" => RED,
            _ => RESET,
        };

        let name = if s.name.len() > 18 {
            format!("{}...", &s.name[..15])
        } else {
            s.name.clone()
        };

        let expires = format_date(&s.expires_at);
        let details = s.details.as_deref().unwrap_or("");

        println!(
            "{:<10} {:<20} {}{:<12}{} {:<15} {}",
            s.svc_type, name, status_color, s.status, RESET, expires, details,
        );
    }
    println!();

    Ok(())
}

pub fn cmd_info(name: &str) -> Result<(), String> {
    service_info(name)
}

fn service_info(name: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;

    let vps_resp = client
        .get("/vps/list")
        .map_err(|e| format!("failed to fetch services: {}", e))?;

    let vps_data: api::VPSListResponse = serde_json::from_slice(&vps_resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    let found_vps = vps_data
        .data
        .iter()
        .find(|v| v.name == name || v.display_name == name);

    let found_vps = match found_vps {
        Some(v) => v,
        None => {
            print_error(&format!("Service '{}' not found", name));
            return Err(format!("service not found: {}", name));
        }
    };

    if json_output() {
        println!(
            "{}",
            serde_json::to_string_pretty(found_vps).unwrap()
        );
        return Ok(());
    }

    let display_name = if !found_vps.display_name.is_empty() {
        &found_vps.display_name
    } else {
        &found_vps.name
    };

    let status_color = match found_vps.status.to_uppercase().as_str() {
        "RUNNING" => GREEN,
        "STOPPED" => YELLOW,
        "CREATING" => CYAN,
        "ERROR" | "FAILED" => RED,
        _ => RESET,
    };

    println!("\n{}{}{} (VPS)", BOLD, display_name, RESET);
    println!("{}", "\u{2500}".repeat(50));
    println!(
        "  Status:     {}{}{}",
        status_color, found_vps.status, RESET
    );
    println!("  ID:         {}", found_vps.id);
    println!("  Region:     {}", found_vps.region);
    if !found_vps.node.is_empty() {
        println!("  Node:       {}", found_vps.node);
    }
    println!();
    println!("  {}Specs:{}", BOLD, RESET);
    println!("    CPU:       {} vCPU", found_vps.vcpu);
    println!("    RAM:       {} MB", found_vps.ram);
    println!(
        "    Storage:   {} GB ({})",
        found_vps.storage, found_vps.storage_type
    );
    println!("    Bandwidth: {} Mbps", found_vps.bandwidth);
    println!();
    println!("  {}Network:{}", BOLD, RESET);
    let public_ip = if found_vps.public_ip.is_empty() {
        "N/A"
    } else {
        &found_vps.public_ip
    };
    let local_ip = if found_vps.local_ip.is_empty() {
        "N/A"
    } else {
        &found_vps.local_ip
    };
    println!("    Public IP: {}{}{}", CYAN, public_ip, RESET);
    println!("    Local IP:  {}", local_ip);
    println!();
    println!("  {}Domains:{}", BOLD, RESET);
    if !found_vps.domain.is_empty() {
        println!(
            "    Public:    {}{}{} (free)",
            CYAN, found_vps.domain, RESET
        );
    }
    if found_vps.custom_domain_enabled == 1 {
        println!(
            "    Custom:    {}Enabled{} (+{}/month)",
            GREEN,
            RESET,
            format_idr(found_vps.custom_domain_price as i64)
        );
        println!(
            "               Run '{}dalang domain list {}{}' to see custom domains",
            CYAN, name, RESET
        );
    } else {
        println!("    Custom:    {}Disabled{}", YELLOW, RESET);
        println!(
            "               Run '{}dalang domain enable {}{}' to activate (+{}/month)",
            CYAN,
            name,
            RESET,
            format_idr(found_vps.custom_domain_price as i64)
        );
    }
    println!();
    println!("  {}Subscription:{}", BOLD, RESET);
    println!(
        "    Price:     {}/month",
        format_idr(found_vps.price as i64)
    );
    println!(
        "    Expires:   {}",
        format_expiry_with_days(&found_vps.expires_at)
    );
    println!();

    // Fetch resource usage
    if found_vps.status.to_uppercase() == "RUNNING" {
        let usage_path = format!("/vps/usage?vps_id={}", found_vps.id);
        match client.get(&usage_path) {
            Ok(resp) => {
                if let Ok(usage_resp) = serde_json::from_slice::<api::VPSUsageResponse>(&resp) {
                    let u = &usage_resp.data;
                    if u.memory_total > 0 || u.disk_total > 0 {
                        println!("  {}Resource Usage:{}", BOLD, RESET);

                        // Memory usage
                        if u.memory_total > 0 {
                            let mem_pct = (u.memory_used as f64 / u.memory_total as f64 * 100.0) as u8;
                            let used_str = format_bytes(u.memory_used);
                            let total_str = format_bytes(u.memory_total);
                            print_usage_bar("RAM", mem_pct, &used_str, &total_str);
                        }

                        // Disk usage
                        if u.disk_total > 0 {
                            let disk_pct = (u.disk_used as f64 / u.disk_total as f64 * 100.0) as u8;
                            let used_str = format_bytes(u.disk_used);
                            let total_str = format_bytes(u.disk_total);
                            print_usage_bar("Disk", disk_pct, &used_str, &total_str);
                        }

                        // Warnings for high usage
                        if u.memory_total > 0 {
                            let mem_pct = (u.memory_used as f64 / u.memory_total as f64 * 100.0) as u8;
                            if mem_pct >= 90 {
                                println!("    {}! RAM usage critical ({}%) - VM may be unresponsive{}", YELLOW, mem_pct, RESET);
                            } else if mem_pct >= 70 {
                                println!("    {}! RAM usage high ({}%) - performance may be degraded{}", YELLOW, mem_pct, RESET);
                            }
                        }
                        if u.disk_total > 0 {
                            let disk_pct = (u.disk_used as f64 / u.disk_total as f64 * 100.0) as u8;
                            if disk_pct >= 90 {
                                println!("    {}! Disk usage critical ({}%) - VM may fail to write{}", YELLOW, disk_pct, RESET);
                            } else if disk_pct >= 70 {
                                println!("    {}! Disk usage high ({}%) - consider freeing space{}", YELLOW, disk_pct, RESET);
                            }
                        }

                        // CPU (show cumulative seconds)
                        if u.cpu_seconds > 0.0 {
                            let cpu_str = if u.cpu_seconds > 3600.0 {
                                format!("{:.1}h", u.cpu_seconds / 3600.0)
                            } else if u.cpu_seconds > 60.0 {
                                format!("{:.1}m", u.cpu_seconds / 60.0)
                            } else {
                                format!("{:.1}s", u.cpu_seconds)
                            };
                            println!("    CPU Time:  {}", cpu_str);
                        }

                        println!();
                    }
                }
            }
            Err(_) => {} // silently skip if usage unavailable
        }
    }

    Ok(())
}

fn service_create(args: &[String]) -> Result<(), String> {
    let mut name = String::new();
    let mut image = String::new();
    let mut region = String::new();
    let mut cpu: i64 = 0;
    let mut ram: i64 = 0;
    let mut storage: i64 = 0;
    let mut bandwidth: i64 = 0;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--name" | "-n" => {
                if i + 1 < args.len() {
                    name = args[i + 1].clone();
                    i += 1;
                }
            }
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
                    storage = parse_size(&args[i + 1]) / 1024;
                    i += 1;
                }
            }
            "--bandwidth" | "-b" => {
                if i + 1 < args.len() {
                    bandwidth = parse_size(&args[i + 1]);
                    i += 1;
                }
            }
            "--image" | "-i" => {
                if i + 1 < args.len() {
                    image = args[i + 1].clone();
                    i += 1;
                }
            }
            "--region" => {
                if i + 1 < args.len() {
                    region = args[i + 1].clone();
                    i += 1;
                }
            }
            "--help" | "-h" => {
                crate::commands::help::print_service_create_help();
                return Ok(());
            }
            _ => {}
        }
        i += 1;
    }

    if name.is_empty() {
        print_error("--name is required");
        return Err("missing required flag: --name".to_string());
    }

    // Set defaults
    if cpu == 0 { cpu = 1; }
    if ram == 0 { ram = 512; }
    if storage == 0 { storage = 5; }
    if bandwidth == 0 { bandwidth = 20; }
    if image.is_empty() { image = "ubuntu".to_string(); }
    if region.is_empty() { region = "ID-BANTEN-02".to_string(); }

    let price = calculate_vps_price(cpu, ram, storage, bandwidth);

    // Show summary
    println!("\n{}Create VPS{}", BOLD, RESET);
    println!("{}", "\u{2500}".repeat(45));
    println!("  Name:      {}", name);
    println!("  CPU:       {} vCPU", cpu);
    println!("  RAM:       {} MB", ram);
    println!("  Storage:   {} GB", storage);
    println!("  Bandwidth: {} Mbps", bandwidth);
    println!("  Image:     {}", image);
    println!("  Region:    {}", region);
    println!("{}", "\u{2500}".repeat(45));
    println!(
        "  {}Estimated Price: {}{}/month{}",
        BOLD,
        GREEN,
        format_idr(price),
        RESET
    );
    println!();
    println!(
        "  {}Tip:{} Run '{}dalang price{}' to see pricing details",
        YELLOW, RESET, CYAN, RESET
    );
    println!();

    if !yes_flag() && !confirm_prompt("Create this VPS?") {
        print_info("Cancelled");
        return Ok(());
    }

    let client = Client::new_authenticated()?;

    print_info("Creating VPS...");

    let body = serde_json::json!({
        "name": name,
        "cpu": cpu,
        "ram": ram,
        "storage": storage,
        "bandwidth": bandwidth,
        "image": image,
        "region": region,
        "pay_method": "credits",
    });

    let resp = client
        .post("/vps/order", Some(&body))
        .map_err(|e| format!("failed to create VPS: {}", e))?;

    let order_resp: api::OrderResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    if order_resp.data.status == "paid" || order_resp.data.status == "provisioning" {
        print_success("VPS creation initiated!");
        println!("  Order ID: {}", order_resp.data.order_id);
        println!("  Price: {}/month", format_idr(order_resp.data.price));
        print_info(&format!(
            "VPS will be ready shortly. Check status with 'dalang service info {}'",
            name
        ));
    } else if !order_resp.data.invoice_url.is_empty() {
        print_success("Invoice created!");
        println!("  Price: {}/month", format_idr(order_resp.data.price));
        println!(
            "  Payment URL: {}{}{}",
            CYAN, order_resp.data.invoice_url, RESET
        );
    }

    Ok(())
}

fn service_upgrade(name: &str, args: &[String]) -> Result<(), String> {
    let client = Client::new_authenticated()?;

    let vps_resp = client
        .get("/vps/list")
        .map_err(|e| format!("failed to fetch services: {}", e))?;

    let vps_data: api::VPSListResponse = serde_json::from_slice(&vps_resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    let found_vps = match vps_data
        .data
        .iter()
        .find(|v| v.name == name || v.display_name == name)
    {
        Some(v) => v.clone(),
        None => {
            print_error(&format!("VPS '{}' not found", name));
            return Err(format!("VPS not found: {}", name));
        }
    };

    // Parse upgrade flags - start with current values
    let mut cpu = found_vps.vcpu as i64;
    let mut ram = (found_vps.ram / 1024) as i64;
    let mut storage = found_vps.storage as i64;
    let mut bandwidth = found_vps.bandwidth as i64;

    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--cpu" | "-c" => {
                if i + 1 < args.len() {
                    cpu = args[i + 1].parse().unwrap_or(cpu);
                    i += 1;
                }
            }
            "--ram" | "-r" => {
                if i + 1 < args.len() {
                    let parsed = parse_size(&args[i + 1]) / 1024;
                    if parsed > 0 {
                        ram = parsed;
                    } else {
                        ram = parse_size(&args[i + 1]);
                    }
                    i += 1;
                }
            }
            "--storage" | "-s" => {
                if i + 1 < args.len() {
                    let parsed = parse_size(&args[i + 1]) / 1024;
                    if parsed > 0 {
                        storage = parsed;
                    } else {
                        storage = parse_size(&args[i + 1]);
                    }
                    i += 1;
                }
            }
            "--bandwidth" | "-b" => {
                if i + 1 < args.len() {
                    bandwidth = args[i + 1].parse().unwrap_or(bandwidth);
                    i += 1;
                }
            }
            "--help" | "-h" => {
                crate::commands::help::print_upgrade_help();
                return Ok(());
            }
            _ => {}
        }
        i += 1;
    }

    let current_ram = (found_vps.ram / 1024) as i64;
    if cpu <= found_vps.vcpu as i64
        && ram <= current_ram
        && storage <= found_vps.storage as i64
        && bandwidth <= found_vps.bandwidth as i64
    {
        print_error("No upgrade specified. New values must be higher than current.");
        println!("\nCurrent specs:");
        println!("  CPU:       {} vCPU", found_vps.vcpu);
        println!("  RAM:       {} GB", current_ram);
        println!("  Storage:   {} GB", found_vps.storage);
        println!("  Bandwidth: {} Mbps", found_vps.bandwidth);
        println!("\nUsage: dalang service upgrade <name> --cpu 4 --ram 4G --storage 50G");
        return Err("no upgrade specified".to_string());
    }

    let current_price = calculate_vps_price(
        found_vps.vcpu as i64,
        found_vps.ram as i64,
        found_vps.storage as i64,
        found_vps.bandwidth as i64,
    );
    let new_price = calculate_vps_price(cpu, ram * 1024, storage, bandwidth);
    let price_diff = new_price - current_price;

    let display_name = if !found_vps.display_name.is_empty() {
        &found_vps.display_name
    } else {
        &found_vps.name
    };

    println!("\n{}Upgrade VPS: {}{}", BOLD, display_name, RESET);
    println!("{}", "\u{2500}".repeat(50));
    println!(
        "  {:<12} {}{:<8}{} \u{2192} {}{:<8}{}",
        "CPU:",
        YELLOW,
        format!("{} vCPU", found_vps.vcpu),
        RESET,
        GREEN,
        format!("{} vCPU", cpu),
        RESET
    );
    println!(
        "  {:<12} {}{:<8}{} \u{2192} {}{:<8}{}",
        "RAM:",
        YELLOW,
        format!("{} GB", current_ram),
        RESET,
        GREEN,
        format!("{} GB", ram),
        RESET
    );
    println!(
        "  {:<12} {}{:<8}{} \u{2192} {}{:<8}{}",
        "Storage:",
        YELLOW,
        format!("{} GB", found_vps.storage),
        RESET,
        GREEN,
        format!("{} GB", storage),
        RESET
    );
    println!(
        "  {:<12} {}{:<8}{} \u{2192} {}{:<8}{}",
        "Bandwidth:",
        YELLOW,
        format!("{} Mbps", found_vps.bandwidth),
        RESET,
        GREEN,
        format!("{} Mbps", bandwidth),
        RESET
    );
    println!("{}", "\u{2500}".repeat(50));
    println!("  Current price: {}/month", format_idr(current_price));
    println!("  New price:     {}/month", format_idr(new_price));
    println!(
        "  {}Difference:    +{}/month{}",
        GREEN,
        format_idr(price_diff),
        RESET
    );
    println!();

    if !yes_flag() && !confirm_prompt("Proceed with upgrade?") {
        print_info("Cancelled");
        return Ok(());
    }

    print_info("Creating upgrade invoice...");

    let body = serde_json::json!({
        "vps_id": found_vps.id,
        "vcpu": cpu,
        "ram": ram,
        "storage": storage,
        "bandwidth": bandwidth,
        "pay_method": "credits",
    });

    let resp = client
        .post("/vps/upgrade", Some(&body))
        .map_err(|e| format!("failed to create upgrade invoice: {}", e))?;

    let upgrade_resp: api::UpgradeResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    if !upgrade_resp.success {
        let err_msg = if !upgrade_resp.error.is_empty() {
            &upgrade_resp.error
        } else if !upgrade_resp.message.is_empty() {
            &upgrade_resp.message
        } else {
            "Failed to upgrade VPS"
        };
        return Err(err_msg.to_string());
    }

    let final_price = if upgrade_resp.data.price > 0 {
        upgrade_resp.data.price
    } else {
        price_diff
    };
    print_success("VPS upgraded successfully!");
    println!(
        "  Amount paid: {} (from credits)",
        format_idr(final_price)
    );
    println!();

    Ok(())
}

fn service_extend(name: &str, args: &[String]) -> Result<(), String> {
    let client = Client::new_authenticated()?;

    let vps_resp = client
        .get("/vps/list")
        .map_err(|e| format!("failed to fetch services: {}", e))?;

    let vps_data: api::VPSListResponse = serde_json::from_slice(&vps_resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    let found_vps = match vps_data
        .data
        .iter()
        .find(|v| v.name == name || v.display_name == name)
    {
        Some(v) => v.clone(),
        None => {
            print_error(&format!("VPS '{}' not found", name));
            return Err(format!("VPS not found: {}", name));
        }
    };

    let mut months: i64 = 1;
    let mut i = 0;
    while i < args.len() {
        match args[i].as_str() {
            "--months" | "-m" => {
                if i + 1 < args.len() {
                    months = args[i + 1].parse().unwrap_or(1);
                    i += 1;
                }
            }
            "--help" | "-h" => {
                crate::commands::help::print_extend_help();
                return Ok(());
            }
            _ => {}
        }
        i += 1;
    }

    let valid_months = [1, 3, 6, 12];
    if !valid_months.contains(&months) {
        print_error("Invalid billing period. Use 1, 3, 6, or 12 months.");
        return Err(format!("invalid billing period: {}", months));
    }

    let mut monthly_price = found_vps.price as i64;
    if monthly_price == 0 {
        monthly_price = calculate_vps_price(
            found_vps.vcpu as i64,
            found_vps.ram as i64,
            found_vps.storage as i64,
            found_vps.bandwidth as i64,
        );
    }
    let total_price = monthly_price * months;

    let display_name = if !found_vps.display_name.is_empty() {
        &found_vps.display_name
    } else {
        &found_vps.name
    };

    let period_label = if months > 1 {
        format!("{} months", months)
    } else {
        format!("{} month", months)
    };

    println!(
        "\n{}Extend Subscription: {}{}",
        BOLD, display_name, RESET
    );
    println!("{}", "\u{2500}".repeat(50));
    println!(
        "  Current expiry: {}",
        format_date(&found_vps.expires_at)
    );
    println!("  Extension:      {}", period_label);
    println!("  Monthly price:  {}", format_idr(monthly_price));
    println!("{}", "\u{2500}".repeat(50));
    println!("  {}Total: {}{}", BOLD, format_idr(total_price), RESET);
    println!();

    if !yes_flag() && !confirm_prompt("Proceed with extension?") {
        print_info("Cancelled");
        return Ok(());
    }

    print_info("Creating extension invoice...");

    let body = serde_json::json!({
        "vps_id": found_vps.id,
        "billing_period": months,
        "pay_method": "credits",
    });

    let resp = client
        .post("/bills/extend-subscription", Some(&body))
        .map_err(|e| format!("failed to create extension invoice: {}", e))?;

    let extend_resp: api::ExtendResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    if !extend_resp.success {
        let err_msg = if !extend_resp.error.is_empty() {
            &extend_resp.error
        } else if !extend_resp.message.is_empty() {
            &extend_resp.message
        } else {
            "Failed to extend subscription"
        };
        return Err(err_msg.to_string());
    }

    print_success("Subscription extended successfully!");
    println!(
        "  Amount paid: {} (from credits)",
        format_idr(total_price)
    );
    if !extend_resp.data.new_expiry.is_empty() {
        println!(
            "  New expiry: {}",
            format_date(&extend_resp.data.new_expiry)
        );
    }
    println!();

    Ok(())
}

fn print_usage_bar(label: &str, percent: u8, used: &str, total: &str) {
    let pct = if percent > 100 { 100 } else { percent };
    let bar_width = 30;
    let filled = (pct as usize * bar_width) / 100;
    let empty = bar_width - filled;

    let color = if pct >= 90 {
        RED
    } else if pct >= 70 {
        YELLOW
    } else {
        GREEN
    };

    let bar: String = format!(
        "{}{}{}{}",
        color,
        "\u{2588}".repeat(filled),
        RESET,
        "\u{2591}".repeat(empty),
    );

    println!(
        "    {:<6} [{}] {:>3}%  ({} / {})",
        format!("{}:", label),
        bar,
        pct,
        used,
        total,
    );
}

fn format_bytes(bytes: i64) -> String {
    let b = bytes as f64;
    if b >= 1_073_741_824.0 {
        format!("{:.1} GB", b / 1_073_741_824.0)
    } else if b >= 1_048_576.0 {
        format!("{:.0} MB", b / 1_048_576.0)
    } else if b >= 1024.0 {
        format!("{:.0} KB", b / 1024.0)
    } else {
        format!("{} B", bytes)
    }
}
