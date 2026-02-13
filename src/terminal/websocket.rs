use std::io::{self, Read, Write};
use std::net::TcpStream;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{mpsc, Arc};
use std::thread;
use std::time::Duration;

use crossterm::terminal;
use tungstenite::protocol::Message;
use tungstenite::stream::MaybeTlsStream;
use tungstenite::WebSocket;

pub struct Terminal {
    ws: WebSocket<MaybeTlsStream<TcpStream>>,
    raw_mode: bool,
    done: Arc<AtomicBool>,
}

#[derive(serde::Serialize)]
struct ResizeMessage {
    #[serde(rename = "type")]
    msg_type: String,
    cols: u16,
    rows: u16,
}

impl Terminal {
    pub fn new(ws_url: &str) -> Result<Terminal, String> {
        use tungstenite::client::IntoClientRequest;

        let mut request = ws_url
            .into_client_request()
            .map_err(|e| format!("invalid WebSocket URL: {}", e))?;

        // Derive frontend Origin from the WebSocket URL
        // wss://api.dalang.io -> https://dalang.io
        // wss://test-api.dalang.io -> https://test.dalang.io
        let origin = {
            let host = ws_url
                .split('/')
                .nth(2)
                .unwrap_or("dalang.io");
            let scheme = if ws_url.starts_with("wss://") { "https" } else { "http" };
            let frontend_host = if host.starts_with("api.") {
                host.strip_prefix("api.").unwrap().to_string()
            } else if host.starts_with("test-api.") {
                format!("test.{}", host.strip_prefix("test-api.").unwrap())
            } else {
                host.to_string()
            };
            format!("{}://{}", scheme, frontend_host)
        };

        request
            .headers_mut()
            .insert("Origin", origin.parse().unwrap());
        request.headers_mut().insert(
            "User-Agent",
            "dalang-cli/1.0".parse().unwrap(),
        );

        let (ws, _response) = tungstenite::connect(request)
            .map_err(|e| format!("failed to connect to WebSocket: {}", e))?;

        Ok(Terminal {
            ws,
            raw_mode: false,
            done: Arc::new(AtomicBool::new(false)),
        })
    }

    pub fn run(&mut self) -> Result<(), String> {
        // Put terminal in raw mode
        if crossterm::tty::IsTty::is_tty(&io::stdin()) {
            terminal::enable_raw_mode()
                .map_err(|e| format!("failed to set raw mode: {}", e))?;
            self.raw_mode = true;
        }

        // Send initial resize
        self.send_resize();

        // Set a short read timeout on the underlying stream so ws.read()
        // doesn't block forever — allows us to interleave reads and writes.
        self.set_read_timeout(Some(Duration::from_millis(50)));

        // Channel: stdin thread sends keystrokes to main loop
        let (tx, rx) = mpsc::channel::<Vec<u8>>();
        let done_stdin = Arc::clone(&self.done);
        thread::spawn(move || {
            let stdin = io::stdin();
            let mut buf = [0u8; 1024];
            loop {
                if done_stdin.load(Ordering::Relaxed) {
                    return;
                }
                match stdin.lock().read(&mut buf) {
                    Ok(0) => return,
                    Ok(n) => {
                        if tx.send(buf[..n].to_vec()).is_err() {
                            return;
                        }
                    }
                    Err(e) if e.kind() == io::ErrorKind::Interrupted => continue,
                    Err(_) => return,
                }
            }
        });

        // Track terminal size for resize detection
        let mut last_size = terminal::size().unwrap_or((80, 24));
        let mut escape_state: u8 = 1; // start after "newline" to allow ~. at beginning

        // Main loop: owns the WebSocket, no Mutex needed
        loop {
            if self.done.load(Ordering::Relaxed) {
                break;
            }

            // 1. Try reading from WebSocket (non-blocking due to timeout)
            match self.ws.read() {
                Ok(Message::Text(text)) => {
                    // Check if it's an error message from server
                    if let Ok(val) = serde_json::from_str::<serde_json::Value>(&text) {
                        if let Some(error) = val.get("error").and_then(|v| v.as_str()) {
                            if !error.is_empty() {
                                eprint!("\r\nError: {}\r\n", error);
                                continue;
                            }
                        }
                    }
                    let _ = io::stdout().write_all(text.as_bytes());
                    let _ = io::stdout().flush();
                }
                Ok(Message::Binary(data)) => {
                    let _ = io::stdout().write_all(&data);
                    let _ = io::stdout().flush();
                }
                Ok(Message::Close(_)) => {
                    break;
                }
                Ok(Message::Ping(data)) => {
                    let _ = self.ws.send(Message::Pong(data));
                }
                Ok(_) => {}
                Err(tungstenite::Error::Io(ref e))
                    if e.kind() == io::ErrorKind::WouldBlock
                        || e.kind() == io::ErrorKind::TimedOut =>
                {
                    // Timeout — no data from server, continue to process stdin
                }
                Err(tungstenite::Error::ConnectionClosed) => {
                    break;
                }
                Err(_) => {
                    break;
                }
            }

            // 2. Drain stdin channel and send to WebSocket
            while let Ok(data) = rx.try_recv() {
                // Escape sequence detection: ~. after newline
                let mut should_disconnect = false;
                for &b in &data {
                    match escape_state {
                        1 => {
                            if b == b'~' {
                                escape_state = 2;
                                continue;
                            } else if b == b'\r' || b == b'\n' {
                                escape_state = 1;
                            } else {
                                escape_state = 0;
                            }
                        }
                        2 => {
                            if b == b'.' {
                                should_disconnect = true;
                                break;
                            } else if b == b'\r' || b == b'\n' {
                                escape_state = 1;
                            } else {
                                escape_state = 0;
                            }
                        }
                        _ => {
                            if b == b'\r' || b == b'\n' {
                                escape_state = 1;
                            }
                        }
                    }
                }

                if should_disconnect {
                    self.done.store(true, Ordering::Relaxed);
                    break;
                }

                let text = String::from_utf8_lossy(&data).to_string();
                if self.ws.send(Message::Text(text)).is_err() {
                    self.done.store(true, Ordering::Relaxed);
                    break;
                }
            }

            // 3. Check for terminal resize
            if let Ok(current_size) = terminal::size() {
                if current_size != last_size {
                    last_size = current_size;
                    let msg = ResizeMessage {
                        msg_type: "resize".to_string(),
                        cols: current_size.0,
                        rows: current_size.1,
                    };
                    if let Ok(json) = serde_json::to_string(&msg) {
                        let _ = self.ws.send(Message::Text(json));
                    }
                }
            }
        }

        self.restore();
        self.done.store(true, Ordering::Relaxed);
        println!("\r\nConnection closed.");
        Ok(())
    }

    pub fn send_resize(&mut self) {
        if let Ok((cols, rows)) = terminal::size() {
            let msg = ResizeMessage {
                msg_type: "resize".to_string(),
                cols,
                rows,
            };
            if let Ok(data) = serde_json::to_string(&msg) {
                let _ = self.ws.send(Message::Text(data));
            }
        }
    }

    fn set_read_timeout(&self, timeout: Option<Duration>) {
        match self.ws.get_ref() {
            MaybeTlsStream::Plain(stream) => {
                let _ = stream.set_read_timeout(timeout);
            }
            MaybeTlsStream::NativeTls(tls_stream) => {
                let _ = tls_stream.get_ref().set_read_timeout(timeout);
            }
            _ => {}
        }
    }

    fn restore(&self) {
        if self.raw_mode {
            let _ = terminal::disable_raw_mode();
        }
    }
}

impl Drop for Terminal {
    fn drop(&mut self) {
        self.done.store(true, Ordering::Relaxed);
        self.restore();
        let _ = self.ws.close(None);
    }
}
