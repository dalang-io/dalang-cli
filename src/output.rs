use std::sync::atomic::{AtomicBool, Ordering};

// ANSI color codes
pub const RESET: &str = "\x1b[0m";
pub const RED: &str = "\x1b[31m";
pub const GREEN: &str = "\x1b[32m";
pub const YELLOW: &str = "\x1b[33m";
pub const BLUE: &str = "\x1b[34m";
pub const CYAN: &str = "\x1b[36m";
pub const BOLD: &str = "\x1b[1m";

// Global flags
pub static JSON_OUTPUT: AtomicBool = AtomicBool::new(false);
pub static QUIET_OUTPUT: AtomicBool = AtomicBool::new(false);
pub static YES_FLAG: AtomicBool = AtomicBool::new(false);
pub static VERBOSE_OUTPUT: AtomicBool = AtomicBool::new(false);

pub fn json_output() -> bool {
    JSON_OUTPUT.load(Ordering::Relaxed)
}

pub fn quiet_output() -> bool {
    QUIET_OUTPUT.load(Ordering::Relaxed)
}

pub fn yes_flag() -> bool {
    YES_FLAG.load(Ordering::Relaxed)
}

pub fn verbose_output() -> bool {
    VERBOSE_OUTPUT.load(Ordering::Relaxed)
}

pub fn print_error(msg: &str) {
    eprintln!("{}Error: {}{}", RED, RESET, msg);
}

pub fn print_success(msg: &str) {
    if !quiet_output() {
        println!("{}\u{2713} {}{}", GREEN, RESET, msg);
    }
}

pub fn print_info(msg: &str) {
    if !quiet_output() {
        println!("{}\u{2192} {}{}", BLUE, RESET, msg);
    }
}

pub fn print_warn(msg: &str) {
    if !quiet_output() {
        println!("{}! {}{}", YELLOW, RESET, msg);
    }
}

pub fn print_debug(msg: &str) {
    if verbose_output() {
        eprintln!("{}[DEBUG] {}{}", CYAN, RESET, msg);
    }
}

pub fn confirm_prompt(message: &str) -> bool {
    use std::io::{self, Write};
    print!("{} [y/N]: ", message);
    io::stdout().flush().unwrap_or(());
    let mut response = String::new();
    if io::stdin().read_line(&mut response).is_err() {
        return false;
    }
    response.trim().eq_ignore_ascii_case("y")
}
