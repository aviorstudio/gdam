use crate::{Result, transaction::Project};
use anyhow::ensure;
use regex::Regex;
use std::collections::BTreeMap;
pub fn text(mut id: i64) -> String {
    if id <= 0 {
        return "uid://<invalid>".into();
    }
    let chars = b"abcdefghijklmnopqrstuvwxy012345678";
    let mut out = Vec::new();
    while id > 0 {
        out.push(chars[(id % 34) as usize] as char);
        id /= 34;
    }
    out.reverse();
    format!("uid://{}", out.into_iter().collect::<String>())
}
fn fresh() -> Result<String> {
    let mut bytes = [0; 8];
    getrandom::fill(&mut bytes)?;
    Ok(text(
        ((u64::from_be_bytes(bytes) & i64::MAX as u64).max(1)) as i64,
    ))
}
fn walk(project: &Project, path: &str, files: &mut Vec<String>) -> Result<()> {
    let m = project.root.symlink_metadata(path)?;
    ensure!(!m.file_type().is_symlink(), "refusing addon symlink {path}");
    if m.is_dir() {
        let mut names = project
            .root
            .read_dir(path)?
            .map(|r| {
                r?.file_name()
                    .into_string()
                    .map_err(|_| anyhow::anyhow!("non-UTF-8 addon filename"))
            })
            .collect::<Result<Vec<_>>>()?;
        names.sort();
        for name in names {
            walk(project, &format!("{path}/{name}"), files)?;
        }
    } else {
        ensure!(m.is_file(), "addon contains a nonregular file");
        files.push(path.into());
    }
    Ok(())
}
pub fn regenerate(project: &Project, path: &str) -> Result<usize> {
    let mut files = Vec::new();
    walk(project, path, &mut files)?;
    let mut replacements = BTreeMap::new();
    let pattern = Regex::new(r"uid://[a-y0-8]+")?;
    for p in files.iter().filter(|p| p.ends_with(".uid")) {
        let value = project.root.read_to_string(p)?.trim().to_owned();
        ensure!(
            pattern.find(&value).is_some_and(|m| m.as_str() == value),
            "{p} does not hold a uid"
        );
        if let std::collections::btree_map::Entry::Vacant(e) = replacements.entry(value) {
            e.insert(fresh()?);
        }
    }
    if replacements.is_empty() {
        return Ok(0);
    }
    for p in files {
        if ![
            ".uid", ".tscn", ".tres", ".gd", ".import", ".cfg", ".godot", ".tool",
        ]
        .iter()
        .any(|ext| p.ends_with(ext))
        {
            continue;
        }
        let data = project.root.read_to_string(&p)?;
        let rewritten = pattern.replace_all(&data, |c: &regex::Captures<'_>| {
            replacements
                .get(&c[0])
                .cloned()
                .unwrap_or_else(|| c[0].to_owned())
        });
        if rewritten != data {
            let m = project.root.metadata(&p)?;
            #[cfg(unix)]
            let mode = {
                use cap_std::fs::PermissionsExt;
                m.permissions().mode() & 0o777
            };
            #[cfg(not(unix))]
            let mode = 0o644;
            project.write(&p, rewritten.as_bytes(), mode)?;
            #[cfg(not(unix))]
            let _ = m;
        }
    }
    Ok(replacements.len())
}
