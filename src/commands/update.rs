use std::env;
use std::fs;
use std::io::Read;

use crate::api;
use crate::output::*;

const UPDATE_BASE_URL: &str = "https://dalang.io/cli";
const VERSION_INFO_URL: &str = "https://dalang.io/cli/version.json";

pub fn cmd_update(args: &[String]) -> Result<(), String> {
    if !args.is_empty() && (args[0] == "--help" || args[0] == "-h") {
        crate::commands::help::print_update_help();
        return Ok(());
    }

    print_info("Checking for updates...");

    let current_version = crate::commands::version::get_version();

    let latest_version = match get_latest_version() {
        Ok(info) => {
            if info.version == current_version {
                print_success(&format!("Already up to date (version {})", current_version));
                return Ok(());
            }
            print_info(&format!(
                "New version available: {} (current: {})",
                info.version, current_version
            ));
            Some(info)
        }
        Err(e) => {
            print_warn(&format!("Could not check for updates: {}", e));
            print_info("Proceeding with update anyway...");
            None
        }
    };

    let binary_name = get_binary_name();
    let download_url = format!("{}/{}", UPDATE_BASE_URL, binary_name);

    print_info(&format!("Downloading {}...", binary_name));

    // Get current executable path
    let exec_path = env::current_exe()
        .map_err(|e| format!("failed to get executable path: {}", e))?;
    let exec_path = fs::canonicalize(&exec_path)
        .map_err(|e| format!("failed to resolve executable path: {}", e))?;

    // Download to temp file
    let tmp_dir = env::temp_dir();
    let tmp_path = tmp_dir.join(format!("dalang-update-{}", std::process::id()));

    let tls = native_tls::TlsConnector::new()
        .map_err(|e| format!("TLS init error: {}", e))?;
    let agent = ureq::AgentBuilder::new()
        .timeout(std::time::Duration::from_secs(120))
        .tls_connector(std::sync::Arc::new(tls))
        .resolver(crate::api::dns::CloudflareDns)
        .build();

    let resp = agent
        .get(&download_url)
        .call()
        .map_err(|e| format!("failed to download: {}", e))?;

    if resp.status() != 200 {
        return Err(format!("download failed: HTTP {}", resp.status()));
    }

    let mut body = Vec::new();
    resp.into_reader()
        .read_to_end(&mut body)
        .map_err(|e| format!("failed to save download: {}", e))?;

    fs::write(&tmp_path, &body).map_err(|e| format!("failed to write temp file: {}", e))?;

    // Make executable (Unix)
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        fs::set_permissions(&tmp_path, fs::Permissions::from_mode(0o755))
            .map_err(|e| format!("failed to set permissions: {}", e))?;
    }

    // Replace current binary
    if cfg!(windows) {
        let old_path = exec_path.with_extension("old");
        let _ = fs::remove_file(&old_path);
        fs::rename(&exec_path, &old_path)
            .map_err(|e| format!("failed to backup old binary: {}", e))?;
        if let Err(e) = fs::rename(&tmp_path, &exec_path) {
            let _ = fs::rename(&old_path, &exec_path);
            return Err(format!("failed to install new binary: {}", e));
        }
        let _ = fs::remove_file(&old_path);
    } else {
        if let Err(e) = fs::rename(&tmp_path, &exec_path) {
            if e.kind() == std::io::ErrorKind::PermissionDenied {
                let _ = fs::remove_file(&tmp_path);
                print_error("Permission denied. Try: sudo dalang update");
                return Err("permission denied".to_string());
            }
            let _ = fs::remove_file(&tmp_path);
            return Err(format!("failed to install new binary: {}", e));
        }
    }

    if let Some(info) = latest_version {
        print_success(&format!("Updated to version {}", info.version));
    } else {
        print_success("Updated to latest version");
    }

    Ok(())
}

fn get_latest_version() -> Result<api::VersionInfo, String> {
    let tls = native_tls::TlsConnector::new()
        .map_err(|e| format!("TLS init error: {}", e))?;
    let agent = ureq::AgentBuilder::new()
        .timeout(std::time::Duration::from_secs(10))
        .tls_connector(std::sync::Arc::new(tls))
        .resolver(crate::api::dns::CloudflareDns)
        .build();

    let resp = agent
        .get(VERSION_INFO_URL)
        .call()
        .map_err(|e| format!("{}", e))?;

    if resp.status() != 200 {
        return Err(format!("HTTP {}", resp.status()));
    }

    let mut body = Vec::new();
    resp.into_reader()
        .read_to_end(&mut body)
        .map_err(|e| format!("{}", e))?;

    serde_json::from_slice(&body).map_err(|e| format!("{}", e))
}

fn get_binary_name() -> String {
    let os = if cfg!(target_os = "windows") {
        "windows"
    } else if cfg!(target_os = "macos") {
        "darwin"
    } else if cfg!(target_os = "android") {
        "android"
    } else {
        "linux"
    };

    let arch = if cfg!(target_arch = "x86_64") {
        "amd64"
    } else if cfg!(target_arch = "aarch64") {
        "arm64"
    } else {
        "amd64"
    };

    let ext = if cfg!(target_os = "windows") {
        ".exe"
    } else {
        ""
    };

    format!("dalang-{}-{}{}", os, arch, ext)
}
