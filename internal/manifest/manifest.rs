use crate::{Result, fsutil};
use anyhow::{bail, ensure};
use serde::{Deserialize, Serialize, de::Error as _};
use serde_json::Value;
use std::{collections::BTreeMap, fs, path::Path};
pub const LINK_FILENAME: &str = "gdam.link.json";
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
    pub fn load(path: &Path) -> Result<Self> {
        let mut m = Self::parse(&fsutil::read_limited(path, fsutil::MAX_JSON)?)?;
        let links = path.parent().unwrap_or(Path::new(".")).join(LINK_FILENAME);
        if links.exists() {
            let lm: Option<LinkManifest> = fsutil::read_json(&links)?;
            let lm = lm.unwrap_or_default();
            for (name, link) in lm.addons {
                if let Some(addon) = m.addons.get_mut(&name) {
                    addon.link = Some(link);
                }
            }
        }
        Ok(m)
    }
    pub fn save(&self, path: &Path) -> Result<()> {
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
            fsutil::save_json(&lp, &links)?;
        }
        fsutil::save_json(path, self)
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
