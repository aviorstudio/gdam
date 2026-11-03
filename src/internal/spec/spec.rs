use crate::Result;
use anyhow::{bail, ensure};
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct PackageSpec {
    pub owner: String,
    pub repo: String,
    pub tag: String,
}
impl PackageSpec {
    pub fn name(&self) -> String {
        format!("@{}/{}", self.owner, self.repo)
    }
    pub fn repo_path(&self) -> String {
        format!("{}/{}", self.owner, self.repo)
    }
    pub fn parse(input: &str) -> Result<Self> {
        let s = input.trim();
        ensure!(!s.is_empty(), "empty spec");
        let Some(rest) = s.strip_prefix('@') else {
            bail!("spec must start with @ (got {s:?})")
        };
        let parts = rest.split('@').collect::<Vec<_>>();
        ensure!(
            parts.len() <= 2,
            "invalid spec {s:?} (expected @owner/repo[@tag])"
        );
        let bits = parts[0].split('/').collect::<Vec<_>>();
        ensure!(
            bits.len() == 2 && !bits[0].is_empty() && !bits[1].is_empty(),
            "invalid repo {:?} (expected owner/repo)",
            parts[0]
        );
        let tag = parts.get(1).copied().unwrap_or("");
        ensure!(
            parts.len() == 1 || !tag.is_empty(),
            "invalid spec {s:?} (tag is empty)"
        );
        Ok(Self {
            owner: bits[0].to_lowercase(),
            repo: bits[1].into(),
            tag: tag.into(),
        })
    }
}
pub fn dir_name(name: &str) -> Result<String> {
    let value = name.trim().replace('/', "_");
    let plain = value.strip_prefix('@').unwrap_or("");
    ensure!(
        plain
            .as_bytes()
            .first()
            .is_some_and(u8::is_ascii_alphanumeric)
            && plain
                .bytes()
                .all(|b| b.is_ascii_alphanumeric() || b"._-".contains(&b)),
        "invalid addon dir name: {value:?}"
    );
    Ok(value)
}
