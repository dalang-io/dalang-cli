use serde::{Deserialize, Serialize};
use std::fs;

use super::config::{ensure_config_dir, get_config_dir};

const CREDS_FILE_NAME: &str = "credentials";

#[derive(Serialize, Deserialize, Default)]
pub struct Credentials {
    pub access_token: String,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub refresh_token: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub email: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub expires_at: Option<i64>,
}

pub fn load_credentials() -> Result<Credentials, String> {
    let dir = get_config_dir()?;
    let path = dir.join(CREDS_FILE_NAME);
    let data =
        fs::read_to_string(&path).map_err(|e| format!("failed to read credentials: {}", e))?;
    serde_json::from_str(&data).map_err(|e| format!("failed to parse credentials: {}", e))
}

pub fn save_credentials(creds: &Credentials) -> Result<(), String> {
    ensure_config_dir()?;
    let dir = get_config_dir()?;
    let path = dir.join(CREDS_FILE_NAME);
    let data =
        serde_json::to_string_pretty(creds).map_err(|e| format!("failed to serialize: {}", e))?;

    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        let mut file = fs::OpenOptions::new()
            .write(true)
            .create(true)
            .truncate(true)
            .mode(0o600)
            .open(&path)
            .map_err(|e| format!("failed to write credentials: {}", e))?;
        use std::io::Write;
        file.write_all(data.as_bytes())
            .map_err(|e| format!("failed to write credentials: {}", e))?;
        Ok(())
    }

    #[cfg(not(unix))]
    {
        fs::write(&path, data).map_err(|e| format!("failed to write credentials: {}", e))
    }
}

pub fn delete_credentials() -> Result<(), String> {
    let dir = get_config_dir()?;
    let path = dir.join(CREDS_FILE_NAME);
    match fs::remove_file(&path) {
        Ok(()) => Ok(()),
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(()),
        Err(e) => Err(format!("failed to remove credentials: {}", e)),
    }
}

pub fn has_credentials() -> bool {
    load_credentials()
        .map(|c| !c.access_token.is_empty())
        .unwrap_or(false)
}
