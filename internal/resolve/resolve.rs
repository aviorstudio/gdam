use crate::{Result, lockfile, spec};
use anyhow::{Context, ensure};
use std::collections::{BTreeMap, VecDeque};
#[derive(Clone, Debug, Default)]
pub struct Release {
    pub identity: lockfile::Release,
    pub global_classes: Vec<String>,
    pub local: bool,
}
#[derive(Clone, Debug, Default)]
pub struct Copy {
    pub release: Release,
    pub path: String,
    pub nested: bool,
    pub required_by: Vec<String>,
    pub dependencies: BTreeMap<String, String>,
}
#[derive(Clone, Debug, Default)]
pub struct Plan {
    pub copies: Vec<Copy>,
    pub hoisted: BTreeMap<String, String>,
}
fn published(value: &str) -> Option<chrono::DateTime<chrono::FixedOffset>> {
    chrono::DateTime::parse_from_rfc3339(value).ok()
}
pub fn resolve(
    pins: &BTreeMap<String, String>,
    mut fetch: impl FnMut(&str, &str) -> Result<Release>,
) -> Result<Plan> {
    let mut releases = BTreeMap::new();
    let mut requesters: BTreeMap<String, BTreeMap<String, Vec<String>>> = BTreeMap::new();
    let mut queue = pins
        .iter()
        .map(|(n, t)| (n.clone(), t.clone(), "gdam.json".to_owned()))
        .collect::<VecDeque<_>>();
    while let Some((name, tag, by)) = queue.pop_front() {
        requesters
            .entry(name.clone())
            .or_default()
            .entry(tag.clone())
            .or_default()
            .push(by.clone());
        let key = lockfile::key(&name, &tag);
        if releases.contains_key(&key) {
            continue;
        }
        ensure!(
            releases.len() < 4096,
            "dependency graph exceeds 4096 releases"
        );
        let mut r =
            fetch(&name, &tag).with_context(|| format!("resolving {key} (required by {by})"))?;
        if r.identity.name.is_empty() {
            r.identity.name = name.clone();
        }
        if r.identity.tag.is_empty() {
            r.identity.tag = tag.clone();
        }
        for (dep, t) in &r.identity.dependencies {
            ensure!(*dep != name, "{key} depends on itself");
            spec::PackageSpec::parse(dep)?;
            ensure!(!t.is_empty(), "dependency {dep} needs an exact tag");
            queue.push_back((dep.clone(), t.clone(), key.clone()));
        }
        releases.insert(key, r);
    }
    let mut hoisted = BTreeMap::new();
    for (name, tags) in &requesters {
        let best = if let Some(t) = pins.get(name) {
            t.clone()
        } else {
            let mut best = String::new();
            for (tag, by) in tags {
                if best.is_empty()
                    || by.len() > tags[&best].len()
                    || (by.len() == tags[&best].len()
                        && published(&releases[&lockfile::key(name, tag)].identity.published_at)
                            > published(
                                &releases[&lockfile::key(name, &best)].identity.published_at,
                            ))
                {
                    best = tag.clone();
                }
            }
            best
        };
        hoisted.insert(name.clone(), best);
    }
    let mut copies = Vec::new();
    for (name, tag) in &hoisted {
        let mut by = requesters[name][tag].clone();
        by.sort();
        copies.push(Copy {
            release: releases[&lockfile::key(name, tag)].clone(),
            path: format!("addons/{}", spec::dir_name(name)?),
            required_by: by,
            ..Copy::default()
        });
    }
    let mut i = 0;
    while i < copies.len() {
        let consumer = copies[i].clone();
        for (name, tag) in &consumer.release.identity.dependencies {
            let path = if hoisted[name] == *tag {
                format!("addons/{}", spec::dir_name(name)?)
            } else {
                ensure!(
                    !consumer.release.local,
                    "linked addon {} pins {name}@{tag} but the project installs {name}@{}; pin the same tag in gdam.json",
                    consumer.release.identity.name,
                    hoisted[name]
                );
                ensure!(
                    copies.len() < 4096 && consumer.path.split('/').count() < 128,
                    "dependency placement contains an unbounded cycle"
                );
                let path = format!("{}/.gdam/{}", consumer.path, spec::dir_name(name)?);
                copies.push(Copy {
                    release: releases[&lockfile::key(name, tag)].clone(),
                    path: path.clone(),
                    nested: true,
                    required_by: vec![lockfile::key(
                        &consumer.release.identity.name,
                        &consumer.release.identity.tag,
                    )],
                    ..Copy::default()
                });
                path
            };
            copies[i].dependencies.insert(name.clone(), path);
        }
        i += 1;
    }
    Ok(Plan { copies, hoisted })
}
