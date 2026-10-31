use std::{env, process::Command};
fn main() {
    for name in ["VERSION", "COMMIT", "DATE"] {
        println!("cargo:rerun-if-env-changed={name}");
    }
    let git = |args: &[&str]| {
        Command::new("git")
            .args(args)
            .output()
            .ok()
            .filter(|o| o.status.success())
            .map(|o| String::from_utf8_lossy(&o.stdout).trim().to_owned())
            .unwrap_or_default()
    };
    println!(
        "cargo:rustc-env=GDAM_TARGET={}",
        env::var("TARGET").unwrap()
    );
    let version = env::var("VERSION").unwrap_or_else(|_| format!("v{}", env!("CARGO_PKG_VERSION")));
    let commit = env::var("COMMIT").unwrap_or_else(|_| git(&["rev-parse", "HEAD"]));
    for name in ["HEAD".to_owned(), git(&["symbolic-ref", "-q", "HEAD"])] {
        if !name.is_empty() {
            println!(
                "cargo:rerun-if-changed={}",
                git(&["rev-parse", "--git-path", &name])
            );
        }
    }
    let date = env::var("DATE").unwrap_or_else(|_| git(&["show", "-s", "--format=%cI", "HEAD"]));
    for (name, value) in [
        ("GDAM_VERSION", version),
        ("GDAM_COMMIT", commit),
        ("GDAM_DATE", date),
    ] {
        println!("cargo:rustc-env={name}={value}");
    }
}
