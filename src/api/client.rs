use crate::config;
use crate::output;

use super::error::ApiError;

pub struct Client {
    pub base_url: String,
    pub token: String,
    pub verbose: bool,
}

impl Client {
    pub fn new() -> Result<Client, String> {
        let base_url = config::get_api_url();

        let mut token = String::new();
        if let Ok(creds) = config::load_credentials() {
            if !creds.access_token.is_empty() {
                token = creds.access_token;
            }
        }

        Ok(Client {
            base_url,
            token,
            verbose: output::verbose_output(),
        })
    }

    pub fn new_authenticated() -> Result<Client, String> {
        let client = Client::new()?;
        if client.token.is_empty() {
            return Err("not authenticated. Run 'dalang auth' to login".to_string());
        }
        Ok(client)
    }

    pub fn request(
        &self,
        method: &str,
        path: &str,
        body: Option<&serde_json::Value>,
    ) -> Result<Vec<u8>, ApiError> {
        // Build URL - don't double-slash
        let mut url = self.base_url.clone();
        if !url.ends_with('/') && !path.starts_with('/') {
            url.push('/');
        } else if url.ends_with('/') && path.starts_with('/') {
            url.push_str(&path[1..]);
            // Skip the normal path append below
            if self.verbose {
                eprintln!("[DEBUG] {} {}", method, url);
            }
            return self.do_request(method, &url, body);
        }
        url.push_str(path);

        if self.verbose {
            eprintln!("[DEBUG] {} {}", method, url);
        }

        self.do_request(method, &url, body)
    }

    fn do_request(
        &self,
        method: &str,
        url: &str,
        body: Option<&serde_json::Value>,
    ) -> Result<Vec<u8>, ApiError> {
        if self.verbose {
            if let Some(b) = body {
                eprintln!("[DEBUG] Request body: {}", b);
            }
        }

        let tls_connector = native_tls::TlsConnector::new()
            .map_err(|e| ApiError::Network(format!("TLS init error: {}", e)))?;
        let agent = ureq::AgentBuilder::new()
            .timeout(std::time::Duration::from_secs(30))
            .tls_connector(std::sync::Arc::new(tls_connector))
            .build();

        let mut req = match method {
            "GET" => agent.get(url),
            "POST" => agent.post(url),
            "PUT" => agent.put(url),
            "DELETE" => agent.delete(url),
            _ => return Err(ApiError::Network(format!("unsupported method: {}", method))),
        };

        req = req
            .set("Content-Type", "application/json")
            .set("User-Agent", "dalang-cli/1.0");

        if !self.token.is_empty() {
            req = req.set("Authorization", &format!("Bearer {}", self.token));
            if self.verbose {
                let preview = if self.token.len() > 20 {
                    format!("{}...", &self.token[..20])
                } else {
                    self.token.clone()
                };
                eprintln!("[DEBUG] Auth: Bearer {}", preview);
            }
        }

        let response = if let Some(b) = body {
            let json_str = serde_json::to_string(b)
                .map_err(|e| ApiError::Parse(format!("failed to serialize body: {}", e)))?;
            req.send_string(&json_str)
        } else if method == "POST" || method == "PUT" {
            req.send_string("null")
        } else {
            req.call()
        };

        match response {
            Ok(resp) => {
                if self.verbose {
                    eprintln!("[DEBUG] Response status: {}", resp.status());
                }

                let mut body_bytes = Vec::new();
                resp.into_reader()
                    .read_to_end(&mut body_bytes)
                    .map_err(|e| ApiError::Network(format!("failed to read response: {}", e)))?;

                if self.verbose {
                    if let Ok(s) = std::str::from_utf8(&body_bytes) {
                        eprintln!("[DEBUG] Response body: {}", s);
                    }
                }

                Ok(body_bytes)
            }
            Err(ureq::Error::Status(status, resp)) => {
                if self.verbose {
                    eprintln!("[DEBUG] Response status: {}", status);
                }

                let mut body_bytes = Vec::new();
                let _ = resp.into_reader().read_to_end(&mut body_bytes);
                let body_str = String::from_utf8_lossy(&body_bytes).to_string();

                if self.verbose {
                    eprintln!("[DEBUG] Response body: {}", body_str);
                }

                if status == 401 {
                    return Err(ApiError::Auth(
                        "Authentication expired. Run 'dalang auth' to re-login".to_string(),
                    ));
                }

                if status == 429 {
                    return Err(ApiError::Http {
                        status,
                        message: "Rate limited. Please wait and try again".to_string(),
                        body: body_str,
                    });
                }

                // Try to parse error message from response
                if let Ok(err_resp) =
                    serde_json::from_str::<serde_json::Value>(&body_str)
                {
                    let msg = err_resp
                        .get("error")
                        .and_then(|v| v.as_str())
                        .or_else(|| err_resp.get("message").and_then(|v| v.as_str()))
                        .unwrap_or("")
                        .to_string();
                    return Err(ApiError::Http {
                        status,
                        message: msg,
                        body: body_str,
                    });
                }

                Err(ApiError::Http {
                    status,
                    message: String::new(),
                    body: body_str,
                })
            }
            Err(ureq::Error::Transport(e)) => {
                if self.verbose {
                    eprintln!("[DEBUG] HTTP error: {}", e);
                }
                Err(ApiError::Network(format!("{}", e)))
            }
        }
    }

    pub fn get(&self, path: &str) -> Result<Vec<u8>, ApiError> {
        self.request("GET", path, None)
    }

    pub fn post(&self, path: &str, body: Option<&serde_json::Value>) -> Result<Vec<u8>, ApiError> {
        self.request("POST", path, body)
    }

    pub fn put(&self, path: &str, body: Option<&serde_json::Value>) -> Result<Vec<u8>, ApiError> {
        self.request("PUT", path, body)
    }

    pub fn delete(&self, path: &str) -> Result<Vec<u8>, ApiError> {
        self.request("DELETE", path, None)
    }
}

use std::io::Read;
