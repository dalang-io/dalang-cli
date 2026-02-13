use std::fmt;

#[derive(Debug)]
pub enum ApiError {
    Http { status: u16, message: String, body: String },
    Network(String),
    Parse(String),
    Auth(String),
}

impl fmt::Display for ApiError {
    fn fmt(&self, f: &mut fmt::Formatter<'_>) -> fmt::Result {
        match self {
            ApiError::Http { message, status, .. } => {
                if !message.is_empty() {
                    write!(f, "{}", message)
                } else {
                    write!(f, "API error: {}", status)
                }
            }
            ApiError::Network(msg) => write!(f, "{}", msg),
            ApiError::Parse(msg) => write!(f, "{}", msg),
            ApiError::Auth(msg) => write!(f, "{}", msg),
        }
    }
}
