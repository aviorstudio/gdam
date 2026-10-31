use crate::{
    Result, fsutil,
    manifest::nil_default,
    spec::{PackageSpec, dir_name},
};
use anyhow::ensure;
use serde::{Deserialize, Serialize};
use std::{
    collections::{BTreeMap, BTreeSet},
    path::Path,
};
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub struct Lock {
    pub version: u32,
    #[serde(deserialize_with = "nil_default")]
    pub requirements: BTreeMap<String, String>,
    #[serde(deserialize_with = "nil_default")]
    pub addons: BTreeMap<String, Release>,
}
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub struct Release {
    pub name: String,
    pub tag: String,
    pub github_owner: String,
    pub github_repo: String,
    pub github_release_id: i64,
    pub commit_sha: String,
    pub asset_id: i64,
    pub asset_name: String,
    pub asset_digest: String,
    pub published_at: String,
    pub prerelease: bool,
    pub editor_plugin: bool,
    #[serde(
        skip_serializing_if = "BTreeMap::is_empty",
        deserialize_with = "nil_default"
    )]
    pub dependencies: BTreeMap<String, String>,
    pub installs: Vec<Install>,
}
#[derive(Clone, Debug, Default, Serialize, Deserialize)]
#[serde(default, deny_unknown_fields)]
pub struct Install {
    pub path: String,
    pub required_by: Vec<String>,
}
pub fn key(name: &str, tag: &str) -> String {
    format!("{name}@{tag}")
}
pub fn valid_install_path(path: &str, name: &str) -> bool {
    let parts = path.split('/').collect::<Vec<_>>();
    parts.len() >= 2
        && parts.len().is_multiple_of(2)
        && parts[0] == "addons"
        && parts
            .last()
            .is_some_and(|last| *last == name.replace('/', "_"))
        && parts.iter().enumerate().skip(1).all(|(i, p)| {
            if i % 2 == 1 {
                dir_name(p).is_ok_and(|n| n == *p) && p.starts_with('@') && !p.contains('\\')
            } else {
                *p == ".gdam"
            }
        })
}
impl Lock {
    pub fn new() -> Self {
        Self {
            version: 1,
            ..Self::default()
        }
    }
    pub fn load(path: &Path) -> Result<Self> {
        let lock: Self = fsutil::read_json(path)?;
        lock.validate()?;
        ensure!(
            lock.version == 1,
            "gdam.lock: unsupported version {} (this gdam writes 1)",
            lock.version
        );
        Ok(lock)
    }
    pub fn validate(&self) -> Result<()> {
        let mut seen = BTreeSet::new();
        for (k, r) in &self.addons {
            let p = PackageSpec::parse(&r.name)?;
            ensure!(
                p.tag.is_empty() && p.name() == r.name,
                "gdam.lock: invalid addon name {:?}",
                r.name
            );
            ensure!(
                !r.name.is_empty() && !r.tag.is_empty() && key(&r.name, &r.tag) == *k,
                "gdam.lock: invalid release entry {k:?}"
            );
            ensure!(
                r.github_release_id > 0
                    && r.asset_id > 0
                    && !r.asset_digest.is_empty()
                    && !r.asset_name.is_empty()
                    && !r.commit_sha.is_empty(),
                "gdam.lock: entry {k:?} lacks a verified release identity"
            );
            ensure!(
                !r.installs.is_empty(),
                "gdam.lock: entry {k:?} is installed nowhere"
            );
            for i in &r.installs {
                ensure!(
                    valid_install_path(&i.path, &r.name) && seen.insert(i.path.to_lowercase()),
                    "gdam.lock: invalid or duplicate install path {:?}",
                    i.path
                );
            }
        }
        Ok(())
    }
    pub fn json_bytes(&self) -> Result<Vec<u8>> {
        let mut out = self.clone();
        out.version = 1;
        for r in out.addons.values_mut() {
            r.installs.sort_by(|a, b| a.path.cmp(&b.path));
            for i in &mut r.installs {
                i.required_by.sort();
            }
        }
        fsutil::json_bytes(&out)
    }
    pub fn save(&self, path: &Path) -> Result<()> {
        fsutil::atomic(path, &self.json_bytes()?, 0o644)
    }
    pub fn matches(&self, pins: &BTreeMap<String, String>) -> bool {
        self.requirements == *pins
    }
    pub fn paths(&self) -> Vec<String> {
        let mut paths = self
            .addons
            .values()
            .flat_map(|r| r.installs.iter().map(|i| i.path.clone()))
            .collect::<Vec<_>>();
        paths.sort();
        paths
    }
}
