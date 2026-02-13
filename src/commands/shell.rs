use crate::api::{self, Client};
use crate::config;
use crate::output::*;
use crate::terminal;

pub fn cmd_shell(args: &[String]) -> Result<(), String> {
    if args.is_empty() || args[0] == "--help" || args[0] == "-h" {
        crate::commands::help::print_shell_help();
        if args.is_empty() {
            return Err("missing service name".to_string());
        }
        return Ok(());
    }

    connect_terminal(&args[0], "shell")
}

pub fn cmd_console(args: &[String]) -> Result<(), String> {
    if args.is_empty() || args[0] == "--help" || args[0] == "-h" {
        crate::commands::help::print_console_help();
        if args.is_empty() {
            return Err("missing service name".to_string());
        }
        return Ok(());
    }

    connect_terminal(&args[0], "console")
}

fn connect_terminal(name: &str, mode: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;

    // Find VPS by name
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
        Some(v) => v,
        None => {
            print_error(&format!("VPS '{}' not found", name));
            return Err(format!("VPS not found: {}", name));
        }
    };

    if found_vps.status.to_uppercase() != "RUNNING" {
        print_error(&format!(
            "VPS '{}' is not running (status: {})",
            name, found_vps.status
        ));
        return Err("VPS not running".to_string());
    }

    // Load credentials for token
    let creds = config::load_credentials()
        .map_err(|e| format!("failed to load credentials: {}", e))?;

    // Build WebSocket URL
    let api_url = config::get_api_url();
    let ws_url = api_url
        .replace("https://", "wss://")
        .replace("http://", "ws://");

    let ws_url = format!(
        "{}/vps/terminal?uuid={}&token={}&mode={}&force=true",
        ws_url,
        found_vps.id,
        url_encode(&creds.access_token),
        mode
    );

    print_debug(&format!(
        "WebSocket URL: {}",
        ws_url.replace(&creds.access_token, "[TOKEN]")
    ));

    let display_name = if !found_vps.display_name.is_empty() {
        &found_vps.display_name
    } else {
        &found_vps.name
    };

    // Show resource usage before connecting
    let usage_path = format!("/vps/usage?vps_id={}", found_vps.id);
    if let Ok(resp) = client.get(&usage_path) {
        if let Ok(usage_resp) = serde_json::from_slice::<api::VPSUsageResponse>(&resp) {
            let u = &usage_resp.data;
            if u.memory_total > 0 || u.disk_total > 0 {
                println!();
                println!("  {}Resource Usage:{}", BOLD, RESET);
                let mut warnings: Vec<String> = Vec::new();
                if u.memory_total > 0 {
                    let mem_pct = (u.memory_used as f64 / u.memory_total as f64 * 100.0) as u8;
                    print_compact_bar("RAM", mem_pct);
                    if mem_pct >= 90 {
                        warnings.push(format!("RAM usage critical ({}%) - VM may be unresponsive", mem_pct));
                    } else if mem_pct >= 70 {
                        warnings.push(format!("RAM usage high ({}%) - performance may be degraded", mem_pct));
                    }
                }
                if u.disk_total > 0 {
                    let disk_pct = (u.disk_used as f64 / u.disk_total as f64 * 100.0) as u8;
                    print_compact_bar("Disk", disk_pct);
                    if disk_pct >= 90 {
                        warnings.push(format!("Disk usage critical ({}%) - VM may fail to write", disk_pct));
                    } else if disk_pct >= 70 {
                        warnings.push(format!("Disk usage high ({}%) - consider freeing space", disk_pct));
                    }
                }
                for w in &warnings {
                    println!("  {}! {}{}", YELLOW, w, RESET);
                }
                println!();
            }
        }
    }

    print_info(&format!("Connecting to {} ({} mode)...", display_name, mode));

    // Connect and run terminal
    let mut term = terminal::websocket::Terminal::new(&ws_url)
        .map_err(|e| format!("failed to connect: {}", e))?;

    print_success("Connected! Type ~. (tilde dot) after Enter to disconnect.\n");

    match term.run() {
        Ok(()) => Ok(()),
        Err(e) => {
            if e.contains("close 1000") || e.contains("normal") {
                Ok(())
            } else {
                Err(format!("terminal error: {}", e))
            }
        }
    }
}

fn print_compact_bar(label: &str, percent: u8) {
    let pct = if percent > 100 { 100 } else { percent };
    let bar_width = 20;
    let filled = (pct as usize * bar_width) / 100;
    let empty = bar_width - filled;

    let color = if pct >= 90 {
        RED
    } else if pct >= 70 {
        YELLOW
    } else {
        GREEN
    };

    println!(
        "  {:<6} [{}{}{}{}] {:>3}%",
        format!("{}:", label),
        color,
        "\u{2588}".repeat(filled),
        RESET,
        "\u{2591}".repeat(empty),
        pct,
    );
}

fn url_encode(s: &str) -> String {
    let mut result = String::new();
    for b in s.bytes() {
        match b {
            b'A'..=b'Z' | b'a'..=b'z' | b'0'..=b'9' | b'-' | b'_' | b'.' | b'~' => {
                result.push(b as char);
            }
            _ => {
                result.push_str(&format!("%{:02X}", b));
            }
        }
    }
    result
}
