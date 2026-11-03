use std::process::{Command, Output};
fn run(root: &std::path::Path, args: &[&str]) -> Output {
    Command::new(env!("CARGO_BIN_EXE_gdam"))
        .args(args)
        .current_dir(root)
        .env("XDG_CACHE_HOME", root.join("cache"))
        .env("LocalAppData", root.join("cache"))
        .env_remove("GDAM_API_KEY")
        .env_remove("GITHUB_TOKEN")
        .output()
        .unwrap()
}
#[test]
fn machine_output_reports_committed_project_state_without_mixing_progress() {
    let root = tempfile::tempdir().unwrap();
    std::fs::write(
        root.path().join("project.godot"),
        "[application]\nconfig/name=\"fixture\"\n",
    )
    .unwrap();
    let initialized = run(root.path(), &["--json", "init"]);
    assert!(initialized.status.success());
    let value: serde_json::Value = serde_json::from_slice(&initialized.stdout).unwrap();
    assert_eq!(value["result"]["command"], "init");
    assert!(String::from_utf8_lossy(&initialized.stderr).contains("created"));
    let resolved = run(root.path(), &["--json", "install"]);
    assert!(
        resolved.status.success(),
        "{}",
        String::from_utf8_lossy(&resolved.stderr)
    );
    let installed = run(root.path(), &["--json", "install", "--offline"]);
    assert!(
        installed.status.success(),
        "{}",
        String::from_utf8_lossy(&installed.stderr)
    );
    let value: serde_json::Value = serde_json::from_slice(&installed.stdout).unwrap();
    assert_eq!(value["result"]["command"], "install");
    assert_eq!(value["result"]["manifest"]["addons"], serde_json::json!({}));
    let invalid = run(root.path(), &["--json", "unknown"]);
    assert_eq!(invalid.status.code(), Some(2));
    let value: serde_json::Value = serde_json::from_slice(&invalid.stdout).unwrap();
    assert_eq!(value["error"]["code"], "invalid_usage");
    let helped = run(root.path(), &["--json", "install", "--help"]);
    assert!(helped.status.success());
    let _: serde_json::Value = serde_json::from_slice(&helped.stdout).unwrap();
}
