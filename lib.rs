#![forbid(unsafe_code)]
pub type Result<T> = anyhow::Result<T>;
#[path = "internal/fsutil/zip.rs"]
pub mod archive;
#[path = "internal/fsutil/fsutil.rs"]
pub mod fsutil;
#[path = "internal/githubapi/client.rs"]
pub mod github;
#[path = "internal/lockfile/lockfile.rs"]
pub mod lockfile;
#[path = "internal/manifest/manifest.rs"]
pub mod manifest;
#[path = "internal/gdamdb/client.rs"]
pub mod network;
#[path = "internal/project/project.rs"]
pub mod project;
#[path = "internal/resolve/resolve.rs"]
pub mod resolve;
#[path = "internal/spec/spec.rs"]
pub mod spec;
#[path = "internal/commands/transaction.rs"]
pub mod transaction;
#[path = "internal/godotuid/uid.rs"]
pub mod uid;
#[derive(Debug)]
pub struct UserInput(pub String);
impl std::fmt::Display for UserInput {
    fn fmt(&self, f: &mut std::fmt::Formatter<'_>) -> std::fmt::Result {
        f.write_str(&self.0)
    }
}
impl std::error::Error for UserInput {}
pub fn input(error: impl std::fmt::Display) -> anyhow::Error {
    anyhow::Error::new(UserInput(error.to_string()))
}
pub fn require(condition: bool, message: impl Into<String>) -> Result<()> {
    if condition {
        Ok(())
    } else {
        Err(input(message.into()))
    }
}
#[cfg(test)]
#[path = "internal/archive_tests.rs"]
mod archive_tests;
#[path = "internal/commands/commands.rs"]
pub mod commands;
#[path = "internal/commands/depsfile.rs"]
pub mod depsfile;
#[path = "internal/commands/install.rs"]
pub mod install;
#[cfg(test)]
#[path = "internal/network_tests.rs"]
mod network_tests;
#[cfg(test)]
#[path = "internal/reference_tests.rs"]
mod reference_tests;
