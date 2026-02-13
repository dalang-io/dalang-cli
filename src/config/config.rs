use serde::{Deserialize, Serialize};
use std::env;
use std::fs;
use std::path::PathBuf;

const CONFIG_DIR_NAME: &str = ".dalang";
const CONFIG_FILE_NAME: &str = "config.json";
const DEFAULT_API_URL: &str = "https://api.dalang.io";

#[derive(Serialize, Deserialize, Default)]
pub struct Config {
    #[serde(skip_serializing_if = "Option::is_none")]
    pub api_url: Option<String>,
}

pub fn get_config_dir() -> Result<PathBuf, String> {
    let home = if cfg!(windows) {
        env::var("USERPROFILE").map_err(|_| "could not determine home directory".to_string())?
    } else {
        env::var("HOME").map_err(|_| "could not determine home directory".to_string())?
    };
    Ok(PathBuf::from(home).join(CONFIG_DIR_NAME))
}

pub fn ensure_config_dir() -> Result<(), String> {
    let dir = get_config_dir()?;
    fs::create_dir_all(&dir).map_err(|e| format!("failed to create config directory: {}", e))
}

pub fn get_api_url() -> String {
    // Environment variable takes precedence
    if let Ok(url) = env::var("DALANG_API_URL") {
        if !url.is_empty() {
            return url;
        }
    }

    // Try to load from config
    if let Ok(config) = load_config() {
        if let Some(url) = config.api_url {
            if !url.is_empty() {
                return url;
            }
        }
    }

    DEFAULT_API_URL.to_string()
}

pub fn load_config() -> Result<Config, String> {
    let dir = get_config_dir()?;
    let path = dir.join(CONFIG_FILE_NAME);

    match fs::read_to_string(&path) {
        Ok(data) => {
            serde_json::from_str(&data).map_err(|e| format!("failed to parse config: {}", e))
        }
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => Ok(Config::default()),
        Err(e) => Err(format!("failed to read config: {}", e)),
    }
}

#[allow(dead_code)]
pub fn save_config(config: &Config) -> Result<(), String> {
    ensure_config_dir()?;
    let dir = get_config_dir()?;
    let path = dir.join(CONFIG_FILE_NAME);
    let data =
        serde_json::to_string_pretty(config).map_err(|e| format!("failed to serialize: {}", e))?;

    #[cfg(unix)]
    {
        use std::os::unix::fs::OpenOptionsExt;
        let mut file = fs::OpenOptions::new()
            .write(true)
            .create(true)
            .truncate(true)
            .mode(0o600)
            .open(&path)
            .map_err(|e| format!("failed to write config: {}", e))?;
        use std::io::Write;
        file.write_all(data.as_bytes())
            .map_err(|e| format!("failed to write config: {}", e))?;
        Ok(())
    }

    #[cfg(not(unix))]
    {
        fs::write(&path, data).map_err(|e| format!("failed to write config: {}", e))
    }
}
