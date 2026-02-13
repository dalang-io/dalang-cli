use crate::api::{self, Client};
use crate::config;
use crate::output::*;

pub fn cmd_auth(args: &[String]) -> Result<(), String> {
    if args.is_empty() {
        return do_auth();
    }

    match args[0].as_str() {
        "status" => auth_status(),
        "logout" => auth_logout(),
        "--help" | "-h" => {
            crate::commands::help::print_auth_help();
            Ok(())
        }
        other => {
            print_error(&format!("Unknown auth subcommand: {}", other));
            Err(format!("unknown subcommand: {}", other))
        }
    }
}

fn do_auth() -> Result<(), String> {
    let client = Client::new()?;

    print_info("Initiating authentication...");

    let resp = client
        .post("/cli/auth/init", None)
        .map_err(|e| format!("failed to initiate auth: {}", e))?;

    let init_resp: api::CLIAuthInitResponse =
        serde_json::from_slice(&resp).map_err(|e| format!("failed to parse response: {}", e))?;

    if !init_resp.success {
        return Err("auth init failed".to_string());
    }

    // Display instructions
    println!();
    println!(
        "  {}Visit:{} {}{}{}",
        BOLD, RESET, CYAN, init_resp.data.verification_url, RESET
    );
    println!(
        "  {}Enter code:{} {}{}{}{}",
        BOLD, RESET, YELLOW, BOLD, init_resp.data.user_code, RESET
    );
    println!();

    let device_code = &init_resp.data.device_code;
    let max_retries = 3;
    let poll_interval = 15;

    for retry in 1..=max_retries {
        // Countdown timer
        for remaining in (1..=poll_interval).rev() {
            eprint!(
                "\r{}\u{2192}{} Attempt {}/{}: Checking in {:2}s...  ",
                BLUE, RESET, retry, max_retries, remaining
            );
            std::thread::sleep(std::time::Duration::from_secs(1));
        }

        eprint!(
            "\r{}\u{2192}{} Attempt {}/{}: Polling...            ",
            BLUE, RESET, retry, max_retries
        );

        let encoded_code =
            url_encode(device_code);
        let poll_result = client.get(&format!(
            "/cli/auth/poll?device_code={}",
            encoded_code
        ));

        let poll_resp = match poll_result {
            Ok(data) => data,
            Err(_) => {
                eprintln!(
                    "\r{}\u{2717}{} Attempt {}/{}: Network error        ",
                    RED, RESET, retry, max_retries
                );
                continue;
            }
        };

        let auth_resp: api::CLIAuthPollResponse = match serde_json::from_slice(&poll_resp) {
            Ok(r) => r,
            Err(_) => {
                eprintln!(
                    "\r{}\u{2717}{} Attempt {}/{}: Parse error          ",
                    RED, RESET, retry, max_retries
                );
                continue;
            }
        };

        // Check both Error and Message fields
        let err_msg = if !auth_resp.error.is_empty() {
            &auth_resp.error
        } else {
            &auth_resp.message
        };

        if err_msg == "authorization_pending" {
            eprintln!(
                "\r{}\u{2026}{} Attempt {}/{}: Not yet authorized   ",
                YELLOW, RESET, retry, max_retries
            );
            continue;
        }

        if err_msg == "expired_token" {
            eprintln!(
                "\r{}\u{2717}{} Attempt {}/{}: Code expired         ",
                RED, RESET, retry, max_retries
            );
            return Err("authorization expired. Please try again".to_string());
        }

        if err_msg == "access_denied" {
            eprintln!(
                "\r{}\u{2717}{} Attempt {}/{}: Access denied        ",
                RED, RESET, retry, max_retries
            );
            return Err("authorization denied".to_string());
        }

        if auth_resp.success && !auth_resp.data.access_token.is_empty() {
            eprintln!(
                "\r{}\u{2713}{} Attempt {}/{}: Authorized!          ",
                GREEN, RESET, retry, max_retries
            );

            let now = std::time::SystemTime::now()
                .duration_since(std::time::UNIX_EPOCH)
                .unwrap_or_default()
                .as_secs() as i64;

            let creds = config::Credentials {
                access_token: auth_resp.data.access_token,
                refresh_token: if auth_resp.data.refresh_token.is_empty() {
                    None
                } else {
                    Some(auth_resp.data.refresh_token)
                },
                email: if auth_resp.data.email.is_empty() {
                    None
                } else {
                    Some(auth_resp.data.email.clone())
                },
                expires_at: Some(now + auth_resp.data.expires_in),
            };

            config::save_credentials(&creds)
                .map_err(|e| format!("failed to save credentials: {}", e))?;

            println!();
            print_success(&format!(
                "Successfully authenticated as {}{}{}",
                CYAN, auth_resp.data.email, RESET
            ));
            return Ok(());
        }

        // Unknown response
        eprintln!(
            "\r{}?{} Attempt {}/{}: Unexpected response  ",
            YELLOW, RESET, retry, max_retries
        );
    }

    Err(format!(
        "authorization failed after {} attempts. Please try again",
        max_retries
    ))
}

fn auth_status() -> Result<(), String> {
    let creds = config::load_credentials();
    let creds = match creds {
        Ok(c) if !c.access_token.is_empty() => c,
        _ => {
            if json_output() {
                let output = serde_json::json!({"authenticated": false});
                println!("{}", serde_json::to_string_pretty(&output).unwrap());
                return Ok(());
            }
            print_warn("Not authenticated. Run 'dalang auth' to login.");
            return Ok(());
        }
    };

    // Verify token
    let client = match Client::new_authenticated() {
        Ok(c) => c,
        Err(e) => {
            if json_output() {
                let output = serde_json::json!({"authenticated": false, "error": e.to_string()});
                println!("{}", serde_json::to_string_pretty(&output).unwrap());
                return Ok(());
            }
            print_warn(&format!("Credentials found but may be invalid: {}", e));
            return Ok(());
        }
    };

    let resp = match client.get("/auth/me") {
        Ok(r) => r,
        Err(_) => {
            if json_output() {
                let output =
                    serde_json::json!({"authenticated": false, "error": "Token expired or invalid"});
                println!("{}", serde_json::to_string_pretty(&output).unwrap());
                return Ok(());
            }
            print_warn("Token expired or invalid. Run 'dalang auth' to re-login.");
            return Ok(());
        }
    };

    let me_resp: api::MeResponse =
        serde_json::from_slice(&resp).map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        let output = serde_json::json!({
            "authenticated": true,
            "email": me_resp.data.email,
            "role": me_resp.data.role,
        });
        println!("{}", serde_json::to_string_pretty(&output).unwrap());
        return Ok(());
    }

    print_success(&format!(
        "Authenticated as {}{}{} (role: {})",
        CYAN, me_resp.data.email, RESET, me_resp.data.role
    ));

    let _ = creds; // suppress unused warning
    Ok(())
}

fn auth_logout() -> Result<(), String> {
    config::delete_credentials().map_err(|e| format!("failed to remove credentials: {}", e))?;
    print_success("Logged out successfully");
    Ok(())
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
