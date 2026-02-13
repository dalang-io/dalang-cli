#![allow(dead_code)]

mod cli;
mod output;
mod commands;
mod api;
mod config;
mod terminal;

use std::process;

fn main() {
    if let Err(e) = cli::execute() {
        eprintln!("{}", e);
        process::exit(1);
    }
}
