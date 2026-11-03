use crate::{Result, transaction::Project};
use anyhow::ensure;
use regex::Regex;
use std::path::{Path, PathBuf};
pub fn find_up(start: &Path, name: &str) -> Option<PathBuf> {
    start
        .ancestors()
        .find(|p| p.join(name).exists())
        .map(Path::to_owned)
}
fn kv(line: &str) -> Option<(&str, &str)> {
    let (k, v) = line.trim().split_once('=')?;
    let k = k.trim();
    (!k.is_empty()).then_some((k, v.trim()))
}
fn section(lines: &[String], name: &str) -> Option<(usize, usize)> {
    let needle = format!("[{name}]");
    let mut start = None;
    for (i, line) in lines.iter().enumerate() {
        let t = line.trim();
        if t.starts_with('[') && t.ends_with(']') {
            if t == needle {
                start = Some(i);
            } else if let Some(start) = start {
                return Some((start, i));
            }
        }
    }
    start.map(|s| (s, lines.len()))
}
fn array_type(lines: &[String]) -> &'static str {
    for line in lines {
        let t = line.trim();
        if t.starts_with('[') {
            break;
        }
        if let Some(("config_version", v)) = kv(t) {
            return if v.parse::<u32>().is_ok_and(|n| n <= 4) {
                "PoolStringArray"
            } else {
                "PackedStringArray"
            };
        }
    }
    "PackedStringArray"
}
pub fn quote(value: &str) -> String {
    serde_json::to_string(value).expect("string serialization")
}
fn unquote(value: &str) -> Result<String> {
    Ok(serde_json::from_str(value)?)
}
fn parse_array(value: &str) -> Result<(String, Vec<String>)> {
    let t = value.trim();
    let (mut ty, mut inner) = (String::new(), t);
    if let Some(open) = t.find('(')
        && let Some(close) = t.rfind(')').filter(|close| *close > open)
    {
        let kind = t[..open].trim();
        if ["PackedStringArray", "PoolStringArray"].contains(&kind) {
            ty = kind.into();
        }
        inner = &t[open + 1..close];
    }
    let pattern = Regex::new(r#""(?:[^"\\]|\\.)*""#)?;
    let values = pattern
        .find_iter(inner)
        .map(|m| unquote(m.as_str()).unwrap_or_else(|_| m.as_str().trim_matches('"').into()))
        .collect();
    Ok((ty, values))
}
fn format_array(kind: &str, values: &[String]) -> String {
    format!(
        "{kind}({})",
        values
            .iter()
            .map(|v| quote(v))
            .collect::<Vec<_>>()
            .join(", ")
    )
}
fn rewrite(
    input: &str,
    action: impl FnOnce(&mut Vec<String>) -> Result<bool>,
) -> Result<Option<String>> {
    let windows = input.contains("\r\n");
    let normalized = input.replace("\r\n", "\n");
    let trailing = normalized.ends_with('\n');
    let body = normalized.strip_suffix('\n').unwrap_or(&normalized);
    let mut lines = if body.is_empty() {
        Vec::new()
    } else {
        body.split('\n').map(str::to_owned).collect()
    };
    if !action(&mut lines)? {
        return Ok(None);
    }
    let mut out = lines.join("\n");
    if trailing {
        out.push('\n');
    }
    if windows {
        out = out.replace('\n', "\r\n");
    }
    Ok(Some(out))
}
pub fn editor_plugins(input: &str, plugin: &str, enable: bool) -> Result<Option<String>> {
    let plugin = plugin.trim();
    ensure!(
        !plugin.is_empty() && plugin.starts_with("res://"),
        "plugin cfg path must start with res://"
    );
    rewrite(input, |lines| {
        let default_kind = array_type(lines).to_owned();
        let Some((start, end)) = section(lines, "editor_plugins") else {
            if !enable {
                return Ok(false);
            }
            if lines.last().is_some_and(|s| !s.trim().is_empty()) {
                lines.push(String::new());
            }
            lines.push("[editor_plugins]".into());
            lines.push(format!(
                "enabled={}",
                format_array(&default_kind, &[plugin.into()])
            ));
            return Ok(true);
        };
        let index = (start + 1..end).find(|&i| kv(&lines[i]).is_some_and(|(k, _)| k == "enabled"));
        let Some(index) = index else {
            if !enable {
                return Ok(false);
            }
            lines.insert(
                start + 1,
                format!("enabled={}", format_array(&default_kind, &[plugin.into()])),
            );
            return Ok(true);
        };
        let (mut kind, mut values) = parse_array(kv(&lines[index]).unwrap().1)?;
        if kind.is_empty() {
            kind = default_kind;
        }
        if enable {
            if values.iter().any(|s| s == plugin) {
                return Ok(false);
            }
            values.push(plugin.into());
        } else {
            let before = values.len();
            values.retain(|s| s != plugin);
            if before == values.len() {
                return Ok(false);
            }
        }
        lines[index] = format!("enabled={}", format_array(&kind, &values));
        Ok(true)
    })
}
pub fn autoloads(input: &str, from: &str, to: &str) -> Result<Option<String>> {
    let (from, to) = (from.trim(), to.trim());
    ensure!(
        !from.is_empty() && !to.is_empty(),
        "empty addon directory name"
    );
    if from == to {
        return Ok(None);
    }
    rewrite(input, |lines| {
        let Some((start, end)) = section(lines, "autoload") else {
            return Ok(false);
        };
        let old = format!("res://addons/{from}/");
        let new = format!("res://addons/{to}/");
        let mut changed = false;
        for line in &mut lines[start + 1..end] {
            if let Some((key, value)) = kv(line)
                && value.starts_with('"')
            {
                let raw = unquote(value)?;
                if raw.contains(&old) {
                    *line = format!("{key}={}", quote(&raw.replace(&old, &new)));
                    changed = true;
                }
            }
        }
        Ok(changed)
    })
}
impl Project {
    fn edit_config(&self, update: impl FnOnce(&str) -> Result<Option<String>>) -> Result<bool> {
        if !self.root.try_exists("project.godot")? {
            return Ok(false);
        }
        let input = self.root.read_to_string("project.godot")?;
        if let Some(out) = update(&input)? {
            let m = self.root.metadata("project.godot")?;
            #[cfg(unix)]
            let mode = {
                use cap_std::fs::PermissionsExt;
                m.permissions().mode() & 0o777
            };
            #[cfg(not(unix))]
            let mode = 0o644;
            self.write("project.godot", out.as_bytes(), mode)?;
            #[cfg(not(unix))]
            let _ = m;
            return Ok(true);
        }
        Ok(false)
    }
    pub fn set_plugin(&self, plugin: &str, enabled: bool) -> Result<bool> {
        self.edit_config(|s| editor_plugins(s, plugin, enabled))
    }
    pub fn replace_autoload_dir(&self, from: &str, to: &str) -> Result<bool> {
        self.edit_config(|s| autoloads(s, from, to))
    }
}
