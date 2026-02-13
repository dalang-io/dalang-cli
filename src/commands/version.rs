use crate::output::*;

const VERSION: &str = env!("VERSION");
const BUILD_DATE: &str = env!("BUILD_DATE");
const COMMIT: &str = env!("COMMIT");

pub fn get_version() -> &'static str {
    VERSION
}

pub fn cmd_version() -> Result<(), String> {
    if json_output() {
        let output = serde_json::json!({
            "version": VERSION,
            "build_date": BUILD_DATE,
            "commit": COMMIT,
        });
        println!("{}", serde_json::to_string_pretty(&output).unwrap());
        return Ok(());
    }

    print!(
        "{}Dalang CLI{} version {}{}{}",
        BOLD, RESET, CYAN, VERSION, RESET
    );
    if BUILD_DATE != "unknown" || COMMIT != "unknown" {
        print!(" (build {}, commit {})", BUILD_DATE, COMMIT);
    }
    println!();
    Ok(())
}
