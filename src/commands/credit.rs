use crate::api::{self, Client};
use crate::output::*;

pub fn cmd_credit(args: &[String]) -> Result<(), String> {
    if args.is_empty() {
        return credit_balance();
    }

    match args[0].as_str() {
        "history" => credit_history(),
        "add" => {
            if args.len() < 2 {
                print_error("Usage: dalang credit add <amount>");
                return Err("missing amount".to_string());
            }
            credit_add(&args[1])
        }
        "--help" | "-h" => {
            crate::commands::help::print_credit_help();
            Ok(())
        }
        other => {
            // Check if it's a number (shorthand for add)
            if other.parse::<i64>().is_ok() {
                return credit_add(other);
            }
            print_error(&format!("Unknown credit subcommand: {}", other));
            Err(format!("unknown subcommand: {}", other))
        }
    }
}

fn credit_balance() -> Result<(), String> {
    let client = Client::new_authenticated()?;

    let resp = client
        .get("/credits/balance")
        .map_err(|e| format!("failed to fetch balance: {}", e))?;

    let balance_resp: api::BalanceResponse =
        serde_json::from_slice(&resp).map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!(
            "{}",
            serde_json::to_string_pretty(&balance_resp.data).unwrap()
        );
        return Ok(());
    }

    println!();
    println!("{}Credit Balance{}", BOLD, RESET);
    println!("{}", "\u{2500}".repeat(40));
    println!(
        "  Balance:     {}{}{}{}",
        GREEN,
        BOLD,
        format_idr(balance_resp.data.balance),
        RESET
    );
    println!(
        "  Total Topup: {}",
        format_idr(balance_resp.data.total_topup)
    );
    println!(
        "  Total Spent: {}",
        format_idr(balance_resp.data.total_spent)
    );
    if balance_resp.data.total_commission > 0 {
        println!(
            "  Commission:  {}{}{}",
            CYAN,
            format_idr(balance_resp.data.total_commission),
            RESET
        );
    }
    println!();

    Ok(())
}

fn credit_history() -> Result<(), String> {
    let client = Client::new_authenticated()?;

    let resp = client
        .get("/credits/transactions?page=1&limit=25")
        .map_err(|e| format!("failed to fetch transactions: {}", e))?;

    let tx_resp: api::TransactionsResponse =
        serde_json::from_slice(&resp).map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!(
            "{}",
            serde_json::to_string_pretty(&tx_resp.data).unwrap()
        );
        return Ok(());
    }

    if tx_resp.data.transactions.is_empty() {
        print_info("No transactions found");
        return Ok(());
    }

    println!(
        "\n{}Recent Transactions{} (showing {} of {})",
        BOLD,
        RESET,
        tx_resp.data.transactions.len(),
        tx_resp.data.total
    );
    println!("{}", "\u{2500}".repeat(70));
    println!(
        "{:<12} {:<10} {:>15} {:>15}  {}",
        "DATE", "TYPE", "AMOUNT", "BALANCE", "DESCRIPTION"
    );
    println!("{}", "\u{2500}".repeat(70));

    for tx in &tx_resp.data.transactions {
        let date = format_date(&tx.created_at);
        let (type_color, amount_str) = match tx.tx_type.as_str() {
            "topup" => (GREEN, format!("+{}", format_idr(tx.amount))),
            "spend" => {
                let abs_amount = if tx.amount < 0 { -tx.amount } else { tx.amount };
                (RED, format!("-{}", format_idr(abs_amount)))
            }
            "commission" => (CYAN, format!("+{}", format_idr(tx.amount))),
            "refund" => (YELLOW, format!("+{}", format_idr(tx.amount))),
            _ => (RESET, format_idr(tx.amount)),
        };

        let desc = if tx.description.len() > 25 {
            format!("{}...", &tx.description[..22])
        } else {
            tx.description.clone()
        };

        println!(
            "{:<12} {}{:<10}{} {:>15} {:>15}  {}",
            date,
            type_color,
            tx.tx_type,
            RESET,
            amount_str,
            format_idr(tx.balance_after),
            desc,
        );
    }
    println!();

    Ok(())
}

fn credit_add(amount_str: &str) -> Result<(), String> {
    let amount: i64 = amount_str
        .parse()
        .map_err(|_| format!("invalid amount: {}", amount_str))?;

    let actual_amount = amount * 1000;

    if actual_amount < 50000 {
        print_error("Minimum topup is 50K IDR (enter 50 or higher)");
        return Err("minimum topup is 50K".to_string());
    }

    let client = Client::new_authenticated()?;

    print_info(&format!("Creating topup for {}...", format_idr(actual_amount)));

    let body = serde_json::json!({"amount": actual_amount});
    let resp = client
        .post("/credits/topup", Some(&body))
        .map_err(|e| format!("failed to create topup: {}", e))?;

    let topup_resp: api::TopupResponse =
        serde_json::from_slice(&resp).map_err(|e| format!("failed to parse response: {}", e))?;

    if json_output() {
        println!(
            "{}",
            serde_json::to_string_pretty(&topup_resp.data).unwrap()
        );
        return Ok(());
    }

    println!();
    print_success("Topup invoice created!");
    println!(
        "\n  {}Amount:{} {}",
        BOLD,
        RESET,
        format_idr(topup_resp.data.amount)
    );
    println!("  {}Payment URL:{}", BOLD, RESET);
    println!("  {}{}{}\n", CYAN, topup_resp.data.invoice_url, RESET);
    print_info("Open the URL above to complete payment");

    Ok(())
}

pub fn format_idr(amount: i64) -> String {
    let s = format!("{}", amount);
    let n = s.len();
    if n <= 3 {
        return format!("Rp {}", s);
    }

    let mut result = String::from("Rp ");
    let remainder = n % 3;
    if remainder > 0 {
        result.push_str(&s[..remainder]);
        if n > remainder {
            result.push('.');
        }
    }

    let mut i = remainder;
    while i < n {
        result.push_str(&s[i..i + 3]);
        if i + 3 < n {
            result.push('.');
        }
        i += 3;
    }

    result
}

pub fn format_date(iso_date: &str) -> String {
    if iso_date.len() >= 10 {
        iso_date[..10].to_string()
    } else {
        iso_date.to_string()
    }
}

pub fn format_expiry_with_days(iso_date: &str) -> String {
    if iso_date.is_empty() {
        return "N/A".to_string();
    }

    let date_str = format_date(iso_date);

    // Try to parse date and calculate days remaining
    // Parse YYYY-MM-DD
    let parts: Vec<&str> = date_str.split('-').collect();
    if parts.len() != 3 {
        return date_str;
    }

    let (year, month, day) = match (
        parts[0].parse::<i64>(),
        parts[1].parse::<i64>(),
        parts[2].parse::<i64>(),
    ) {
        (Ok(y), Ok(m), Ok(d)) => (y, m, d),
        _ => return date_str,
    };

    // Get current date
    let now = std::time::SystemTime::now()
        .duration_since(std::time::UNIX_EPOCH)
        .unwrap_or_default()
        .as_secs() as i64;

    // Approximate: convert expiry to epoch seconds
    let expiry_epoch = date_to_epoch(year, month, day);
    let now_days = now / 86400;
    let expiry_days = expiry_epoch / 86400;
    let days = expiry_days - now_days;

    if days < 0 {
        format!("{} ({}expired{})", date_str, RED, RESET)
    } else if days == 0 {
        format!("{} ({}today{})", date_str, YELLOW, RESET)
    } else if days == 1 {
        format!("{} ({}1 day left{})", date_str, YELLOW, RESET)
    } else if days <= 7 {
        format!("{} ({}{} days left{})", date_str, YELLOW, days, RESET)
    } else {
        format!("{} ({} days left)", date_str, days)
    }
}

fn date_to_epoch(year: i64, month: i64, day: i64) -> i64 {
    // Simple epoch calculation (not accounting for all edge cases, but sufficient)
    let mut y = year;
    let mut m = month;
    if m <= 2 {
        y -= 1;
        m += 12;
    }
    let days = 365 * y + y / 4 - y / 100 + y / 400 + (153 * (m - 3) + 2) / 5 + day - 719469;
    days * 86400
}
