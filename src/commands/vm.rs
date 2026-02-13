use crate::api::{self, Client};
use crate::output::*;

pub fn cmd_start(args: &[String]) -> Result<(), String> {
    if args.is_empty() || args[0] == "--help" || args[0] == "-h" {
        crate::commands::help::print_start_help();
        if args.is_empty() {
            return Err("missing service name".to_string());
        }
        return Ok(());
    }

    vm_action(&args[0], "start")
}

pub fn cmd_stop(args: &[String]) -> Result<(), String> {
    if args.is_empty() || args[0] == "--help" || args[0] == "-h" {
        crate::commands::help::print_stop_help();
        if args.is_empty() {
            return Err("missing service name".to_string());
        }
        return Ok(());
    }

    vm_action(&args[0], "stop")
}

pub fn cmd_delete(args: &[String]) -> Result<(), String> {
    if args.is_empty() || args[0] == "--help" || args[0] == "-h" {
        crate::commands::help::print_delete_help();
        if args.is_empty() {
            return Err("missing service name".to_string());
        }
        return Ok(());
    }

    let name = &args[0];

    if !yes_flag() {
        print_warn(&format!(
            "This will permanently delete '{}' and all its data!",
            name
        ));
        if !confirm_prompt("Are you sure you want to delete this VPS?") {
            print_info("Cancelled");
            return Ok(());
        }
    }

    vm_delete(name)
}

fn vm_action(name: &str, action: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;

    let (vps_id, display_name) = resolve_vps_name(&client, name)?;

    let action_label = capitalize(action);
    print_info(&format!("{}ing {}...", action_label, display_name));

    let body = serde_json::json!({
        "id": vps_id,
        "action": action,
    });

    let resp = client
        .post("/vps/action", Some(&body))
        .map_err(|e| format!("failed to {} VPS: {}", action, e))?;

    let result: api::GenericResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    if result.success {
        print_success(&format!("VPS {} command sent successfully", action));
    } else {
        print_error(&format!("Failed: {}", result.message));
    }

    Ok(())
}

fn vm_delete(name: &str) -> Result<(), String> {
    let client = Client::new_authenticated()?;

    let (vps_id, display_name) = resolve_vps_name(&client, name)?;

    print_info(&format!("Deleting {}...", display_name));

    let resp = client
        .delete(&format!("/vps/delete?id={}", vps_id))
        .map_err(|e| format!("failed to delete VPS: {}", e))?;

    let result: api::GenericResponse = serde_json::from_slice(&resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!("{}", String::from_utf8_lossy(&resp));
        return Ok(());
    }

    if result.success {
        print_success(&format!("VPS '{}' deleted successfully", display_name));
    } else {
        print_error(&format!("Failed: {}", result.message));
    }

    Ok(())
}

pub fn resolve_vps_name(client: &Client, name: &str) -> Result<(String, String), String> {
    let vps_resp = client
        .get("/vps/list")
        .map_err(|e| format!("failed to fetch services: {}", e))?;

    let vps_data: api::VPSListResponse = serde_json::from_slice(&vps_resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    for v in &vps_data.data {
        if v.name == name || v.display_name == name {
            let display = if !v.display_name.is_empty() {
                v.display_name.clone()
            } else {
                v.name.clone()
            };
            return Ok((v.id.clone(), display));
        }
    }

    Err(format!("VPS '{}' not found", name))
}

fn capitalize(s: &str) -> String {
    let mut c = s.chars();
    match c.next() {
        None => String::new(),
        Some(f) => f.to_uppercase().collect::<String>() + c.as_str(),
    }
}

