//! Command simulate plays automated campaigns against the shipped content
//! and prints the ending distribution, for balance work after content
//! edits. Exits 1 when any run soft-locked.

use std::env;
use std::process::ExitCode;

use alexander_core::content;
use alexander_core::sim;

fn main() -> ExitCode {
    let mut runs: usize = 1000;
    let mut seed: u64 = 1;
    let mut args = env::args().skip(1);
    while let Some(arg) = args.next() {
        match arg.as_str() {
            "--runs" => match args.next().and_then(|v| v.parse().ok()) {
                Some(n) => runs = n,
                None => {
                    eprintln!("usage: simulate [--runs N] [--seed S]");
                    return ExitCode::FAILURE;
                }
            },
            "--seed" => match args.next().and_then(|v| v.parse().ok()) {
                Some(s) => seed = s,
                None => {
                    eprintln!("usage: simulate [--runs N] [--seed S]");
                    return ExitCode::FAILURE;
                }
            },
            other => {
                eprintln!("unknown flag {other:?}; usage: simulate [--runs N] [--seed S]");
                return ExitCode::FAILURE;
            }
        }
    }

    let lib = content::load_embedded();
    let report = sim::aggregate(&sim::simulate(&lib, runs, seed));
    print!("{report}");
    if report.stuck > 0 {
        ExitCode::FAILURE
    } else {
        ExitCode::SUCCESS
    }
}
