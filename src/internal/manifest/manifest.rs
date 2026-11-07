use crate::{Result, fsutil};
use anyhow::{bail, ensure};
use serde::{Deserialize, Serialize, de::Error as _};
use serde_json::Value;
use std::{collections::BTreeMap, fs, path::Path};
pub const FILENAME: &str = "gdam.toml";
pub const LINK_FILENAME: &str = "gdam.link.toml";
/// Recognized legacy sources never become missing-config defaults or implicit imports.
pub fn require_current_source(directory: &Path) -> Result<()> {
    for (legacy, canonical) in [("gdam.json", FILENAME), ("gdam.link.json", LINK_FILENAME)] {
        if directory.join(canonical).try_exists()? {
            continue;
        }
        match fs::symlink_metadata(directory.join(legacy)) {
            Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
            Err(e) => return Err(e.into()),
            Ok(_) => bail!(
                "legacy JSON v1 {legacy} requires explicit conversion: manually create {canonical}; no automatic import"
            ),
        }
    }
    Ok(())
}
#[derive(Clone, Default, Debug, Serialize)]
pub struct Manifest {
    pub addons: BTreeMap<String, Addon>,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub package: String,
}
#[derive(Clone, Default, Debug, Serialize)]
pub struct Addon {
    #[serde(skip_serializing_if = "String::is_empty")]
    pub tag: String,
    #[serde(skip)]
    pub link: Option<Link>,
}
#[derive(Clone, Default, Debug, Serialize)]
pub struct Link {
    pub enabled: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    pub path: String,
}
#[derive(Clone, Default, Debug, Serialize, Deserialize)]
#[serde(deny_unknown_fields)]
pub struct LinkManifest {
    #[serde(default, deserialize_with = "nil_default")]
    pub addons: BTreeMap<String, Link>,
}
pub fn nil_default<'de, D, T>(d: D) -> std::result::Result<T, D::Error>
where
    D: serde::Deserializer<'de>,
    T: Deserialize<'de> + Default,
{
    Ok(Option::<T>::deserialize(d)?.unwrap_or_default())
}
fn string(value: Option<&Value>) -> Result<String> {
    match value {
        None | Some(Value::Null) => Ok(String::new()),
        Some(Value::String(s)) => Ok(s.clone()),
        _ => bail!("expected string"),
    }
}
impl<'de> Deserialize<'de> for Addon {
    fn deserialize<D: serde::Deserializer<'de>>(d: D) -> std::result::Result<Self, D::Error> {
        let value = Value::deserialize(d)?;
        let parse = || -> Result<Self> {
            if value.is_null() {
                return Ok(Self::default());
            }
            let m = value
                .as_object()
                .ok_or_else(|| anyhow::anyhow!("addon must be an object"))?;
            for key in m.keys() {
                match key.as_str() {
                    "tag" => {}
                    "version" => bail!(
                        "legacy gdam.json version field is unsupported; run `gdam add @owner/addon@<exact-tag>` to create a new manifest"
                    ),
                    "link" => bail!(
                        "gdam.json no longer supports link configuration (move it to {LINK_FILENAME})"
                    ),
                    _ => bail!("unknown field {key:?}"),
                }
            }
            Ok(Self {
                tag: string(m.get("tag"))?,
                link: None,
            })
        };
        parse().map_err(D::Error::custom)
    }
}
impl<'de> Deserialize<'de> for Link {
    fn deserialize<D: serde::Deserializer<'de>>(d: D) -> std::result::Result<Self, D::Error> {
        let value = Value::deserialize(d)?;
        let parse = || -> Result<Self> {
            if value.is_null() {
                return Ok(Self::default());
            }
            let m = value
                .as_object()
                .ok_or_else(|| anyhow::anyhow!("link must be an object"))?;
            for key in m.keys() {
                ensure!(
                    key == "enabled" || key == "path",
                    "unknown link field {key:?}"
                );
            }
            let enabled = match m.get("enabled") {
                None => bail!("missing link.enabled"),
                Some(Value::Null) => false,
                Some(Value::Bool(v)) => *v,
                _ => bail!("link.enabled must be boolean"),
            };
            let path = string(m.get("path"))?.trim().to_owned();
            ensure!(
                !enabled || !path.is_empty(),
                "link is enabled but path is empty"
            );
            Ok(Self { enabled, path })
        };
        parse().map_err(D::Error::custom)
    }
}
#[derive(Deserialize, Default)]
#[serde(default, deny_unknown_fields)]
struct Raw {
    #[serde(deserialize_with = "nil_default")]
    addons: BTreeMap<String, Addon>,
    #[serde(deserialize_with = "nil_default")]
    package: String,
}
impl Manifest {
    pub fn parse(bytes: &[u8]) -> Result<Self> {
        let raw: Option<Raw> = serde_json::from_slice(bytes)?;
        let raw = raw.unwrap_or_default();
        Ok(Self {
            addons: raw.addons,
            package: raw.package,
        })
    }
    /// Canonical source decoder. `parse` remains the explicit package/legacy-v1 JSON codec.
    pub fn parse_source(bytes: &[u8]) -> Result<Self> {
        let value =
            crate::source_toml::parse(std::str::from_utf8(bytes)?).map_err(anyhow::Error::msg)?;
        let raw: Raw = serde_json::from_value(value)?;
        Ok(Self {
            addons: raw.addons,
            package: raw.package,
        })
    }
    pub fn source_bytes<T: Serialize>(value: &T) -> Result<Vec<u8>> {
        Ok(toml::to_string_pretty(value)?.into_bytes())
    }
    pub fn load(path: &Path) -> Result<Self> {
        require_current_source(path.parent().unwrap_or(Path::new(".")))?;
        let mut m = Self::parse_source(&fsutil::read_limited(path, fsutil::MAX_JSON)?)?;
        let directory = path.parent().unwrap_or(Path::new("."));
        let links = directory.join(LINK_FILENAME);
        if links.exists() {
            let value = crate::source_toml::parse(std::str::from_utf8(&fsutil::read_limited(
                &links,
                fsutil::MAX_JSON,
            )?)?)
            .map_err(anyhow::Error::msg)?;
            let lm: LinkManifest = serde_json::from_value(value)?;
            for (name, link) in lm.addons {
                if let Some(addon) = m.addons.get_mut(&name) {
                    addon.link = Some(link);
                }
            }
        }
        Ok(m)
    }
    pub fn save(&self, path: &Path) -> Result<()> {
        require_current_source(path.parent().unwrap_or(Path::new(".")))?;
        let links = LinkManifest {
            addons: self
                .addons
                .iter()
                .filter_map(|(k, a)| a.link.as_ref().map(|l| (k.clone(), l.clone())))
                .collect(),
        };
        let lp = path.parent().unwrap_or(Path::new(".")).join(LINK_FILENAME);
        if links.addons.is_empty() {
            match fs::remove_file(lp) {
                Ok(()) => {}
                Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
                Err(e) => return Err(e.into()),
            }
        } else {
            fsutil::atomic(&lp, &Self::source_bytes(&links)?, 0o644)?;
        }
        fsutil::atomic(path, &Self::source_bytes(self)?, 0o644)
    }
    pub fn pins(&self) -> BTreeMap<String, String> {
        self.addons
            .iter()
            .filter(|(_, a)| {
                !a.link
                    .as_ref()
                    .is_some_and(|l| l.enabled && !l.path.trim().is_empty())
            })
            .map(|(n, a)| (n.clone(), a.tag.clone()))
            .collect()
    }
}

#[cfg(test)]
mod source_tests {
    use super::*;
    #[test]
    fn source_roundtrip_keeps_tags_links_and_separate_json_wire() {
        let root = tempfile::tempdir().unwrap();
        let source = root.path().join(FILENAME);
        let mut manifest =
            Manifest::parse(br#"{"addons":{"@owner/addon":{"tag":"v1"}},"package":"addon"}"#)
                .unwrap();
        manifest.addons.get_mut("@owner/addon").unwrap().link = Some(Link {
            enabled: false,
            path: "../fixture 回転".into(),
        });
        manifest.save(&source).unwrap();
        let loaded = Manifest::load(&source).unwrap();
        assert_eq!(loaded.package, "addon");
        assert_eq!(loaded.addons["@owner/addon"].tag, "v1");
        assert_eq!(
            loaded.addons["@owner/addon"].link.as_ref().unwrap().path,
            "../fixture 回転"
        );
        assert!(!loaded.addons["@owner/addon"].link.as_ref().unwrap().enabled);
        assert!(!root.path().join("gdam.json").exists());
        assert!(Manifest::parse_source(br#"{"addons":{}}"#).is_err());
        assert!(
            Manifest::parse_source(b"[addons.'@owner/addon']\ntag = 'v1'\nversion = 'v2'").is_err()
        );
        assert!(
            Manifest::parse_source(b"[addons.'@owner/addon']\ntag = 'v1'\ntag = 'v2'").is_err()
        );
        // Legacy null defaults remain confined to the explicit v1 JSON/package codec.
        let legacy = Manifest::parse(br#"{"addons":null,"package":null}"#).unwrap();
        assert!(legacy.addons.is_empty());
        assert!(legacy.package.is_empty());
    }
}
