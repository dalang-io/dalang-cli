use crate::api::{self, Client};
use crate::commands::credit::{format_date, format_idr};
use crate::output::*;

pub fn cmd_domain(args: &[String]) -> Result<(), String> {
    if args.is_empty() || args[0] == "--help" || args[0] == "-h" {
        crate::commands::help::print_domain_help();
        return Ok(());
    }

    match args[0].as_str() {
        "enable" => {
            if args.len() < 2 {
                print_error("Usage: dalang domain enable <vps-name>");
                return Err("missing VPS name".to_string());
            }
            domain_enable(&args[1])
        }
        "list" | "ls" => {
            if args.len() < 2 {
                print_error("Usage: dalang domain list <vps-name>");
                return Err("missing VPS name".to_string());
            }
            domain_list(&args[1])
        }
        "add" => {
            if args.len() < 3 {
                print_error("Usage: dalang domain add <vps-name> <domain>");
                return Err("missing VPS name or domain".to_string());
            }
            domain_add(&args[1], &args[2])
        }
        "verify" => {
            if args.len() < 2 {
                print_error("Usage: dalang domain verify <domain>");
                return Err("missing domain".to_string());
            }
            domain_verify(&args[1])
        }
        "remove" | "rm" => {
            if args.len() < 2 {
                print_error("Usage: dalang domain remove <domain>");
                return Err("missing domain".to_string());
            }
            domain_remove(&args[1])
        }
        other => {
            print_error(&format!("Unknown domain subcommand: {}", other));
            Err(format!("unknown subcommand: {}", other))
        }
    }
}

fn find_vps_by_name(client: &Client, name: &str) -> Result<api::VPS, String> {
    let resp = client
        .get("/vps/list")
        .map_err(|e| format!("failed to fetch VPS list: {}", e))?;

    let vps_data: api::VPSListResponse =
        serde_json::from_slice(&resp).map_err(|e| format!("failed to parse response: {}", e))?;

    for v in vps_data.data {
        if v.name == name || v.display_name == name {
            return Ok(v);
        }
    }

    Err(format!("VPS '{}' not found", name))
}

fn domain_enable(vps_name: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;
    let vps = find_vps_by_name(&client, vps_name)?;

    if vps.custom_domain_enabled == 1 {
        print_success(&format!(
            "Custom domain is already enabled for {}",
            vps_name
        ));
        return Ok(());
    }

    if !yes_flag() {
        println!(
            "\nEnable custom domain for {}{}{}?",
            CYAN, vps_name, RESET
        );
        println!(
            "Price: {}{}/month{}\n",
            YELLOW,
            format_idr(vps.custom_domain_price as i64),
            RESET
        );
        if !confirm_prompt("Proceed with payment?") {
            print_info("Cancelled");
            return Ok(());
        }
    }

    print_info("Creating invoice...");

    let body = serde_json::json!({"vps_id": vps.id});
    let resp = client
        .post("/vps/addon/custom-domain", Some(&body))
        .map_err(|e| format!("failed to enable custom domain: {}", e))?;

    let result: api::DomainEnableResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if !result.success {
        print_error(&result.message);
        return Err(result.message.clone());
    }

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    print_success("Invoice created!");
    println!(
        "\n  Amount: {}",
        format_idr(result.data.amount)
    );
    println!(
        "  Pay at: {}{}{}\n",
        CYAN, result.data.invoice_url, RESET
    );
    print_info("After payment, custom domain will be automatically enabled");

    Ok(())
}

fn domain_list(vps_name: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;
    let vps = find_vps_by_name(&client, vps_name)?;

    if vps.custom_domain_enabled != 1 {
        print_warn(&format!(
            "Custom domain is not enabled for {}",
            vps_name
        ));
        println!(
            "\nRun '{}dalang domain enable {}{}' to activate",
            CYAN, vps_name, RESET
        );
        return Ok(());
    }

    let resp = client
        .get(&format!("/vps/domains/list?vps_id={}", vps.id))
        .map_err(|e| format!("failed to list domains: {}", e))?;

    let result: api::DomainListResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    println!(
        "\n{}Custom Domains for {}{}",
        BOLD, vps_name, RESET
    );
    println!("{}", "\u{2500}".repeat(60));

    if result.data.is_empty() {
        print_info("No custom domains configured");
        println!(
            "\nAdd a domain with: {}dalang domain add {} <your-domain.com>{}",
            CYAN, vps_name, RESET
        );
        return Ok(());
    }

    println!("{:<30} {:<12} {}", "DOMAIN", "STATUS", "CREATED");
    println!("{}", "\u{2500}".repeat(60));

    for d in &result.data {
        let status_color = match d.status.as_str() {
            "active" => GREEN,
            "failed" => RED,
            _ => YELLOW,
        };

        println!(
            "{:<30} {}{:<12}{} {}",
            d.domain,
            status_color,
            d.status,
            RESET,
            format_date(&d.created_at),
        );
    }
    println!();

    Ok(())
}

fn domain_add(vps_name: &str, domain: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;
    let vps = find_vps_by_name(&client, vps_name)?;

    if vps.custom_domain_enabled != 1 {
        print_error(&format!(
            "Custom domain is not enabled for {}",
            vps_name
        ));
        println!(
            "\nRun '{}dalang domain enable {}{}' first",
            CYAN, vps_name, RESET
        );
        return Err("custom domain not enabled".to_string());
    }

    print_info(&format!("Adding domain {}...", domain));

    let body = serde_json::json!({
        "vps_id": vps.id,
        "domain": domain,
    });

    let resp = client
        .post("/vps/domains/add", Some(&body))
        .map_err(|e| format!("failed to add domain: {}", e))?;

    let result: api::DomainAddResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if !result.success {
        print_error(&result.message);
        return Err(result.message.clone());
    }

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    print_success("Domain added!");
    println!(
        "\n{}Configure these DNS records:{}\n",
        BOLD, RESET
    );

    for record in &result.data.dns_records {
        let rtype = record.get("type").map(|s| s.as_str()).unwrap_or("");
        let rname = record.get("name").map(|s| s.as_str()).unwrap_or("");
        let rvalue = record.get("value").map(|s| s.as_str()).unwrap_or("");
        let rdesc = record.get("description").map(|s| s.as_str()).unwrap_or("");

        println!("  {}{:<6}{}  {}", CYAN, rtype, RESET, rname);
        println!("          \u{2192} {}", rvalue);
        println!("          {}{}{}\n", YELLOW, rdesc, RESET);
    }

    println!(
        "After configuring DNS, run: {}dalang domain verify {}{}\n",
        CYAN, domain, RESET
    );

    Ok(())
}

fn domain_verify(domain: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;

    print_info(&format!("Verifying domain {}...", domain));

    let body = serde_json::json!({"domain": domain});
    let resp = client
        .post("/vps/domains/verify", Some(&body))
        .map_err(|e| format!("failed to verify domain: {}", e))?;

    let result: api::DomainVerifyResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    if result.success && result.data.status == "active" {
        print_success(&format!("Domain {} is verified and active!", domain));
        return Ok(());
    }

    print_warn("Domain verification in progress");
    println!("\n  Domain: {}", result.data.domain);
    println!("  Status: {}", result.data.status);
    println!("  SSL:    {}\n", result.data.ssl_status);

    if !result.data.dns_records.is_empty() {
        println!(
            "{}Ensure these DNS records are configured:{}\n",
            BOLD, RESET
        );
        for record in &result.data.dns_records {
            let rtype = record.get("type").map(|s| s.as_str()).unwrap_or("");
            let rname = record.get("name").map(|s| s.as_str()).unwrap_or("");
            let rvalue = record.get("value").map(|s| s.as_str()).unwrap_or("");

            println!("  {}{:<6}{}  {}", CYAN, rtype, RESET, rname);
            println!("          \u{2192} {}\n", rvalue);
        }
    }

    print_info("DNS propagation can take up to 24 hours. Try again later.");

    Ok(())
}

fn domain_remove(domain: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;

    if !yes_flag() {
        println!("\nRemove domain {}{}{}?\n", CYAN, domain, RESET);
        if !confirm_prompt("Are you sure?") {
            print_info("Cancelled");
            return Ok(());
        }
    }

    print_info(&format!("Removing domain {}...", domain));

    let body = serde_json::json!({"domain": domain});
    let resp = client
        .post("/vps/domains/remove", Some(&body))
        .map_err(|e| format!("failed to remove domain: {}", e))?;

    let result: api::GenericResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if !result.success {
        print_error(&result.message);
        return Err(result.message.clone());
    }

    print_success("Domain removed successfully");
    Ok(())
}
