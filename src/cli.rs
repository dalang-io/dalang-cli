use std::env;
use std::sync::atomic::Ordering;

use crate::commands;
use crate::output::*;

pub fn execute() -> Result<(), String> {
    let mut args: Vec<String> = env::args().skip(1).collect();

    // Android's linker adds executable path as extra argument - skip it
    if cfg!(target_os = "android") {
        if let Some(first) = args.first() {
            if first.starts_with('/') && first.ends_with("dalang") {
                args.remove(0);
            }
        }
    }

    if args.is_empty() {
        commands::help::print_help();
        return Ok(());
    }

    // Parse global flags first
    args = parse_global_flags(args);

    if args.is_empty() {
        commands::help::print_help();
        return Ok(());
    }

    let command = args[0].as_str();
    let cmd_args: Vec<String> = args[1..].to_vec();

    match command {
        "version" | "-v" | "--version" => commands::version::cmd_version(),
        "help" | "-h" | "--help" => {
            if let Some(subcmd) = cmd_args.first() {
                commands::help::cmd_help_for(subcmd)
            } else {
                commands::help::print_help();
                Ok(())
            }
        }
        "auth" => commands::auth::cmd_auth(&cmd_args),
        "credit" | "credits" => commands::credit::cmd_credit(&cmd_args),
        "service" | "services" => commands::service::cmd_service(&cmd_args),
        "info" => {
            if cmd_args.is_empty() {
                print_error("Usage: dalang info <name>");
                Err("missing service name".to_string())
            } else {
                commands::service::cmd_info(&cmd_args[0])
            }
        }
        "shell" => commands::shell::cmd_shell(&cmd_args),
        "exec" => commands::exec::cmd_exec(&cmd_args),
        "console" => commands::shell::cmd_console(&cmd_args),
        "start" => commands::vm::cmd_start(&cmd_args),
        "stop" => commands::vm::cmd_stop(&cmd_args),
        "delete" => commands::vm::cmd_delete(&cmd_args),
        "domain" | "domains" => commands::domain::cmd_domain(&cmd_args),
        "price" | "pricing" => commands::price::cmd_price(&cmd_args),
        "update" => commands::update::cmd_update(&cmd_args),
        _ => {
            print_error(&format!("Unknown command: {}", command));
            println!("\nRun 'dalang help' for usage.");
            Err(format!("unknown command: {}", command))
        }
    }
}

fn parse_global_flags(args: Vec<String>) -> Vec<String> {
    let mut remaining = Vec::new();
    for arg in args {
        match arg.as_str() {
            "--json" => JSON_OUTPUT.store(true, Ordering::Relaxed),
            "--quiet" | "-q" => QUIET_OUTPUT.store(true, Ordering::Relaxed),
            "--yes" | "-y" => YES_FLAG.store(true, Ordering::Relaxed),
            "--verbose" => VERBOSE_OUTPUT.store(true, Ordering::Relaxed),
            _ => remaining.push(arg),
        }
    }
    remaining
}
