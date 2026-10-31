use crate::{
    Result, input,
    install::{self, Options, absolute, linked},
    manifest::{Addon, Link, Manifest},
    network::Network,
    require,
    spec::{PackageSpec, dir_name},
    transaction::Project,
};
use anyhow::Context;
use std::{
    collections::BTreeSet,
    fs,
    path::{Path, PathBuf},
};
fn package(s: &str) -> Result<PackageSpec> {
    let s = s.trim();
    PackageSpec::parse(&if s.starts_with('@') {
        s.to_owned()
    } else {
        format!("@{s}")
    })
    .map_err(input)
}
fn collision(m: &Manifest, name: &str) -> Result<String> {
    let dir = dir_name(name).map_err(input)?;
    for other in m.addons.keys() {
        if other != name {
            require(
                dir_name(other).map_err(input)?.to_lowercase() != dir.to_lowercase(),
                format!("path addons/{dir} is already managed by {other}"),
            )?;
        }
    }
    Ok(dir)
}
pub fn add(p: &Project, network: &Network, s: &str) -> Result<()> {
    let pkg = package(s)?;
    let name = pkg.name();
    let mut m = p.manifest()?;
    let r = network.resolve(&name, &pkg.tag).map_err(input)?;
    let old = m.addons.get(&name).cloned().unwrap_or_default();
    if linked(&old) {
        let mut old = old;
        old.tag = r.identity.tag;
        m.addons.insert(name.clone(), old.clone());
        p.save_manifest(&m)?;
        println!("updated {name}@{} (linked)", old.tag);
        return Ok(());
    }
    collision(&m, &name)?;
    m.addons.insert(
        name,
        Addon {
            tag: r.identity.tag,
            link: old.link,
        },
    );
    install::install(p, network, &Options::default(), &m)?;
    p.save_manifest(&m)
}
pub fn remove(p: &Project, network: &Network, s: &str) -> Result<()> {
    let pkg = package(s)?;
    require(
        pkg.tag.is_empty(),
        "remove does not take a tag (use @username/addon)",
    )?;
    let name = pkg.name();
    let mut m = p.manifest()?;
    require(
        m.addons.contains_key(&name),
        format!("addon not found in gdam.json: {name}"),
    )?;
    let dir = dir_name(&name).map_err(input)?;
    let cfg = format!("res://addons/{dir}/plugin.cfg");
    if p.set_plugin(&cfg, false)? {
        println!("disabled {cfg}");
    }
    p.remove(&format!("addons/{dir}"))?;
    m.addons.remove(&name);
    p.save_manifest(&m)?;
    println!("removed {name}");
    install::install(p, network, &Options::default(), &m)
}
fn resolved(path: &Path) -> PathBuf {
    fs::canonicalize(path).unwrap_or_else(|_| path.to_owned())
}
fn aliases(p: &Project, m: &Manifest, name: &str, dir: &str, target: &Path) -> Result<()> {
    let target = resolved(target);
    let mut old = BTreeSet::new();
    if let Some(base) = target.file_name().and_then(|s| s.to_str()) {
        let legacy = format!("@{}", base.trim().trim_start_matches('@').replace(' ', "_"));
        if let Ok(v) = dir_name(&legacy) {
            old.insert(v);
        }
    }
    for (other, a) in &m.addons {
        if other != name
            && linked(a)
            && resolved(&absolute(&p.dir, &a.link.as_ref().unwrap().path)?) == target
            && let Ok(v) = dir_name(other)
        {
            old.insert(v);
        }
    }
    if p.dir.join("addons").is_dir() {
        for entry in fs::read_dir(p.dir.join("addons"))? {
            let e = entry?;
            if resolved(&e.path()) == target {
                old.insert(
                    e.file_name()
                        .into_string()
                        .map_err(|_| input("addon filename is not UTF-8"))?,
                );
            }
        }
    }
    for other in old {
        if other != dir {
            p.set_plugin(&format!("res://addons/{other}/plugin.cfg"), false)?;
            p.replace_autoload_dir(&other, dir)?;
        }
    }
    Ok(())
}
pub fn link(p: &Project, network: &Network, s: &str, path: Option<&str>) -> Result<()> {
    let pkg = package(s)?;
    require(
        pkg.tag.is_empty(),
        "link does not take a tag (use @username/addon)",
    )?;
    let name = pkg.name();
    let mut m = p.manifest()?;
    let existing = m.addons.get(&name).cloned();
    let editor = existing.as_ref().is_some_and(|a| {
        network
            .resolve(&name, &a.tag)
            .is_ok_and(|r| r.identity.editor_plugin)
    });
    let mut addon = existing.unwrap_or_default();
    let supplied = path.filter(|s| !s.trim().is_empty());
    let stored = addon.link.as_ref().map(|l| l.path.as_str()).unwrap_or("");
    let chosen = supplied.unwrap_or(stored);
    require(
        !chosen.trim().is_empty(),
        format!("missing local path (run `gdam link {name} <local_path>` )"),
    )?;
    let cwd = std::env::current_dir()?;
    let target = absolute(if supplied.is_some() { &cwd } else { &p.dir }, chosen)?;
    require(
        target.is_dir(),
        format!("local path is not a directory: {}", target.display()),
    )?;
    require(
        target.join("plugin.cfg").is_file(),
        format!(
            "plugin.cfg not found at {} (pass the addon directory that contains plugin.cfg)",
            target.join("plugin.cfg").display()
        ),
    )?;
    let dir = collision(&m, &name)?;
    let dst = format!("addons/{dir}");
    require(
        resolved(&target) != resolved(&p.dir.join(&dst)),
        "local link target is the managed addon destination",
    )?;
    p.root.create_dir_all("addons")?;
    p.remove(&dst)?;
    p.link(&target, &dst)?;
    let path = if supplied.is_some() {
        let home = install::home().ok();
        match home.as_deref().and_then(|h| target.strip_prefix(h).ok()) {
            Some(rest) if rest.as_os_str().is_empty() => "~".into(),
            Some(rest) => format!("~{}{}", std::path::MAIN_SEPARATOR, rest.display()),
            None => target.to_string_lossy().into_owned(),
        }
    } else {
        chosen.into()
    };
    addon.link = Some(Link {
        enabled: true,
        path: path.clone(),
    });
    m.addons.insert(name.clone(), addon);
    p.save_manifest(&m)?;
    if editor && p.root.try_exists("project.godot")? {
        aliases(p, &m, &name, &dir, &target)?;
        let cfg = format!("res://addons/{dir}/plugin.cfg");
        if p.set_plugin(&cfg, true)? {
            println!("enabled {cfg}");
        }
    }
    println!("linked {name} -> {path}");
    Ok(())
}
pub fn unlink(p: &Project, network: &Network, s: &str) -> Result<()> {
    let pkg = package(s)?;
    require(
        pkg.tag.is_empty(),
        "unlink does not take a tag (use @username/addon)",
    )?;
    let name = pkg.name();
    let mut m = p.manifest()?;
    let mut a = m
        .addons
        .get(&name)
        .cloned()
        .ok_or_else(|| input(format!("addon not found in gdam.json: {name}")))?;
    require(linked(&a), format!("addon is not linked: {name}"))?;
    let target = absolute(&p.dir, &a.link.as_ref().unwrap().path)?;
    a.link.as_mut().unwrap().enabled = false;
    let dir = dir_name(&name).map_err(input)?;
    if p.root.try_exists("project.godot")? {
        aliases(p, &m, &name, &dir, &target)?;
    }
    let cfg = format!("res://addons/{dir}/plugin.cfg");
    if p.set_plugin(&cfg, false)? {
        println!("disabled {cfg}");
    }
    let dst = format!("addons/{dir}");
    if a.tag.trim().is_empty() {
        p.remove(&dst)?;
        m.addons.insert(name.clone(), a);
        p.save_manifest(&m)?;
        println!("unlinked {name}");
        return Ok(());
    }
    let r = network.resolve(&name, &a.tag).map_err(input)?;
    let tmp = tempfile::Builder::new().prefix("gdam-unlink-").tempdir()?;
    let root =
        install::prepare(network, &r.identity, tmp.path(), Path::new(""), false).map_err(input)?;
    p.root.create_dir_all("addons")?;
    p.remove(&dst)?;
    p.copy(&root, &dst)?;
    m.addons.insert(name.clone(), a);
    p.save_manifest(&m)?;
    if r.identity.editor_plugin && p.set_plugin(&cfg, true)? {
        println!("enabled {cfg}");
    }
    println!("unlinked {name}");
    Ok(())
}
pub fn unlink_all(p: &Project, network: &Network) -> Result<()> {
    let m = p.manifest()?;
    for (name, a) in m.addons {
        if linked(&a) {
            unlink(p, network, &name)?;
        }
    }
    Ok(())
}
pub fn publish(network: &Network, s: &str, tag: &str, asset: Option<&str>) -> Result<()> {
    let pkg = PackageSpec::parse(s).map_err(input)?;
    require(
        pkg.tag.is_empty(),
        "publish tag must be a separate argument",
    )?;
    let tag = tag.trim();
    require(!tag.is_empty(), "release tag is required")?;
    let key = std::env::var("GDAM_API_KEY").unwrap_or_default();
    require(
        key.trim().starts_with("ak_"),
        "GDAM_API_KEY must be a Clerk publishing key",
    )?;
    let default = std::env::var("GITHUB_REPOSITORY").ok().and_then(|s| {
        s.split_once('/')
            .map(|(o, r)| format!("@{}_{}.zip", o.trim(), r.trim()))
    });
    let asset = asset
        .filter(|s| !s.trim().is_empty())
        .or(default.as_deref())
        .unwrap_or("");
    let (owner, repo) = network.repository(&pkg.name())?;
    let (r, size) = network.publication_identity(&owner, &repo, tag, asset.trim())?;
    network
        .publish(key.trim(), &pkg.owner, &pkg.repo, &r, size)
        .context("publishing release")?;
    println!("published {}@{tag}", pkg.name());
    Ok(())
}
