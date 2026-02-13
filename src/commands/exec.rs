use crate::api::{self, Client};
use crate::output::*;

pub fn cmd_exec(args: &[String]) -> Result<(), String> {
    if args.len() < 2 || args[0] == "--help" || args[0] == "-h" {
        crate::commands::help::print_exec_help();
        if args.len() < 2 {
            return Err("missing arguments".to_string());
        }
        return Ok(());
    }

    let vm_name = &args[0];
    let command = if args.len() > 2 {
        args[1..].join(" ")
    } else {
        args[1].clone()
    };

    execute_command(vm_name, &command)
}

fn execute_command(name: &str, command: &str) -> Result<(), String> {
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

    print_debug(&format!(
        "Executing command on VPS {}: {}",
        found_vps.id, command
    ));

    let body = serde_json::json!({
        "uuid": found_vps.id,
        "command": command,
    });

    let exec_resp = client
        .post("/vps/session/exec", Some(&body))
        .map_err(|e| format!("failed to execute command: {}", e))?;

    let result: api::ExecResponse = serde_json::from_slice(&exec_resp)
        .map_err(|e| format!("failed to parse response: {}", e))?;

    if !result.success {
        print_error(&format!("Command failed: {}", result.message));
        return Err("command failed".to_string());
    }

    if json_output() {
        let json_result = serde_json::json!({
            "output": result.data.output,
            "exit_code": result.data.exit_code,
        });
        println!(
            "{}",
            serde_json::to_string_pretty(&json_result).unwrap()
        );
    } else {
        if !result.data.output.is_empty() {
            print!("{}", result.data.output);
            if !result.data.output.ends_with('\n') {
                println!();
            }
        }
    }

    if result.data.exit_code != 0 {
        return Err(format!("command exited with code {}", result.data.exit_code));
    }

    Ok(())
}
