use std::io::{self, Read, Write};
use std::net::TcpStream;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::{Arc, Mutex};
use std::thread;

use crossterm::terminal;
use tungstenite::protocol::Message;
use tungstenite::stream::MaybeTlsStream;
use tungstenite::WebSocket;

pub struct Terminal {
    ws: Arc<Mutex<WebSocket<MaybeTlsStream<TcpStream>>>>,
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

        request
            .headers_mut()
            .insert("Origin", "https://dalang.io".parse().unwrap());
        request.headers_mut().insert(
            "User-Agent",
            "dalang-cli/1.0".parse().unwrap(),
        );

        let (ws, _response) = tungstenite::connect(request)
            .map_err(|e| format!("failed to connect to WebSocket: {}", e))?;

        Ok(Terminal {
            ws: Arc::new(Mutex::new(ws)),
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

        // Set up SIGWINCH handler on Unix
        #[cfg(unix)]
        let resize_ws = {
            let ws_clone = Arc::clone(&self.ws);
            let done_clone = Arc::clone(&self.done);
            thread::spawn(move || {
                // Use a simple polling approach for resize detection
                let mut last_size = terminal::size().unwrap_or((80, 24));
                while !done_clone.load(Ordering::Relaxed) {
                    thread::sleep(std::time::Duration::from_millis(500));
                    if let Ok(current_size) = terminal::size() {
                        if current_size != last_size {
                            last_size = current_size;
                            let msg = ResizeMessage {
                                msg_type: "resize".to_string(),
                                cols: current_size.0,
                                rows: current_size.1,
                            };
                            if let Ok(data) = serde_json::to_string(&msg) {
                                if let Ok(mut ws) = ws_clone.lock() {
                                    let _ = ws.send(Message::Text(data));
                                }
                            }
                        }
                    }
                }
            });
            Some(())
        };

        // Start writeLoop in background (stdin -> WebSocket)
        let ws_write = Arc::clone(&self.ws);
        let done_write = Arc::clone(&self.done);
        let write_handle = thread::spawn(move || {
            write_loop(ws_write, done_write);
        });

        // readLoop: WebSocket -> stdout (main thread)
        let result = self.read_loop();

        // Signal done
        self.done.store(true, Ordering::Relaxed);

        // Restore terminal
        self.restore();

        // Wait for write thread (but don't block forever since stdin may be blocking)
        let _ = write_handle;

        #[cfg(unix)]
        let _ = resize_ws;

        result
    }

    fn read_loop(&self) -> Result<(), String> {
        loop {
            if self.done.load(Ordering::Relaxed) {
                return Ok(());
            }

            let message = {
                let mut ws = self.ws.lock().map_err(|e| format!("lock error: {}", e))?;
                ws.read()
            };

            match message {
                Ok(Message::Text(text)) => {
                    // Check if it's an error message
                    if let Ok(err_msg) =
                        serde_json::from_str::<serde_json::Value>(&text)
                    {
                        if let Some(error) = err_msg.get("error").and_then(|v| v.as_str()) {
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
                    self.restore();
                    self.done.store(true, Ordering::Relaxed);
                    println!("\r\nConnection closed.");
                    std::process::exit(0);
                }
                Ok(Message::Ping(data)) => {
                    if let Ok(mut ws) = self.ws.lock() {
                        let _ = ws.send(Message::Pong(data));
                    }
                }
                Ok(_) => {}
                Err(tungstenite::Error::ConnectionClosed) => {
                    self.restore();
                    self.done.store(true, Ordering::Relaxed);
                    println!("\r\nConnection closed.");
                    std::process::exit(0);
                }
                Err(_e) => {
                    self.restore();
                    self.done.store(true, Ordering::Relaxed);
                    println!("\r\nConnection closed.");
                    std::process::exit(0);
                }
            }
        }
    }

    pub fn send_resize(&self) {
        if let Ok((cols, rows)) = terminal::size() {
            let msg = ResizeMessage {
                msg_type: "resize".to_string(),
                cols,
                rows,
            };
            if let Ok(data) = serde_json::to_string(&msg) {
                if let Ok(mut ws) = self.ws.lock() {
                    let _ = ws.send(Message::Text(data));
                }
            }
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

        if let Ok(mut ws) = self.ws.lock() {
            let _ = ws.close(None);
        }
    }
}

fn write_loop(
    ws: Arc<Mutex<WebSocket<MaybeTlsStream<TcpStream>>>>,
    done: Arc<AtomicBool>,
) {
    let mut buf = [0u8; 1024];
    let mut escape_state: u8 = 1; // Start after "newline" to allow ~. at very beginning

    let stdin = io::stdin();

    loop {
        if done.load(Ordering::Relaxed) {
            return;
        }

        let n = match stdin.lock().read(&mut buf) {
            Ok(0) => return, // EOF
            Ok(n) => n,
            Err(e) if e.kind() == io::ErrorKind::Interrupted => continue,
            Err(_) => return,
        };

        // Check for SSH-style escape sequence: ~. after newline
        for i in 0..n {
            match escape_state {
                1 => {
                    // After newline
                    if buf[i] == b'~' {
                        escape_state = 2;
                        continue;
                    } else if buf[i] == b'\r' || buf[i] == b'\n' {
                        escape_state = 1;
                    } else {
                        escape_state = 0;
                    }
                }
                2 => {
                    // After tilde
                    if buf[i] == b'.' {
                        // Escape sequence detected - disconnect
                        done.store(true, Ordering::Relaxed);
                        let _ = terminal::disable_raw_mode();
                        println!("\r\nConnection closed.");
                        std::process::exit(0);
                    } else if buf[i] == b'\r' || buf[i] == b'\n' {
                        escape_state = 1;
                    } else {
                        escape_state = 0;
                    }
                }
                _ => {
                    // Normal
                    if buf[i] == b'\r' || buf[i] == b'\n' {
                        escape_state = 1;
                    }
                }
            }
        }

        if let Ok(mut ws) = ws.lock() {
            if ws
                .send(Message::Text(
                    String::from_utf8_lossy(&buf[..n]).to_string(),
                ))
                .is_err()
            {
                return;
            }
        }
    }
}
