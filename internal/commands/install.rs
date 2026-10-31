use crate::{
    Result, archive, fsutil, input,
    lockfile::{self, Install, Lock},
    manifest::Manifest,
    network::Network,
    require,
    resolve::{self, Copy, Plan, Release},
    spec::dir_name,
    transaction::Project,
    uid,
};
use anyhow::{Context, ensure};
use std::{
    collections::{BTreeMap, BTreeSet},
    fs,
    path::{Path, PathBuf},
};
#[derive(Default)]
pub struct Options {
    pub frozen: bool,
    pub offline: bool,
    pub cache: PathBuf,
}
pub fn linked(addon: &crate::manifest::Addon) -> bool {
    addon
        .link
        .as_ref()
        .is_some_and(|l| l.enabled && !l.path.trim().is_empty())
}
pub fn home() -> Result<PathBuf> {
    let key = if cfg!(windows) { "USERPROFILE" } else { "HOME" };
    std::env::var_os(key)
        .filter(|s| !s.is_empty())
        .map(PathBuf::from)
        .ok_or_else(|| anyhow::anyhow!("home directory is unavailable"))
}
pub fn absolute(base: &Path, value: &str) -> Result<PathBuf> {
    let value = value.trim();
    let p = if value == "~" {
        home()?
    } else if let Some(rest) = value
        .strip_prefix("~/")
        .or_else(|| value.strip_prefix("~\\"))
    {
        home()?.join(rest)
    } else {
        PathBuf::from(value)
    };
    let p = if p.is_absolute() { p } else { base.join(p) };
    let mut out = PathBuf::new();
    for c in p.components() {
        match c {
            std::path::Component::ParentDir => {
                out.pop();
            }
            std::path::Component::CurDir => {}
            _ => out.push(c.as_os_str()),
        }
    }
    Ok(out)
}
pub fn cache_dir() -> Result<PathBuf> {
    #[cfg(windows)]
    let base = std::env::var_os("LocalAppData")
        .map(PathBuf::from)
        .ok_or_else(|| anyhow::anyhow!("LocalAppData is unavailable"))?;
    #[cfg(target_os = "macos")]
    let base = home()?.join("Library/Caches");
    #[cfg(all(unix, not(target_os = "macos")))]
    let base = match std::env::var_os("XDG_CACHE_HOME").filter(|s| !s.is_empty()) {
        Some(v) => {
            let p = PathBuf::from(v);
            ensure!(p.is_absolute(), "XDG_CACHE_HOME must be absolute");
            p
        }
        None => home()?.join(".cache"),
    };
    Ok(base.join("gdam/archives"))
}
pub fn package_dependencies(path: &Path) -> Result<BTreeMap<String, String>> {
    let p = path.join("gdam.json");
    if !p.exists() {
        return Ok(BTreeMap::new());
    }
    let m = Manifest::parse(&fsutil::read_limited(&p, fsutil::MAX_JSON)?)?;
    let mut deps = BTreeMap::new();
    for (name, a) in m.addons {
        ensure!(
            !a.tag.trim().is_empty(),
            "{}: dependency {name} needs an exact tag",
            p.display()
        );
        deps.insert(name, a.tag);
    }
    Ok(deps)
}
pub fn lock_from_plan(pins: &BTreeMap<String, String>, plan: &Plan) -> Lock {
    let mut lock = Lock::new();
    lock.requirements = pins.clone();
    for c in &plan.copies {
        if c.release.local {
            continue;
        }
        let r = &c.release.identity;
        let key = lockfile::key(&r.name, &r.tag);
        let entry = lock.addons.entry(key).or_insert_with(|| {
            let mut r = r.clone();
            r.installs.clear();
            r
        });
        entry.installs.push(Install {
            path: c.path.clone(),
            required_by: c.required_by.clone(),
        });
    }
    lock
}
pub fn plan_from_lock(lock: &Lock) -> Result<Plan> {
    let mut plan = Plan::default();
    let mut paths = BTreeMap::new();
    for r in lock.addons.values() {
        for install in &r.installs {
            let nested = install.path.contains("/.gdam/");
            if !nested {
                plan.hoisted.insert(r.name.clone(), r.tag.clone());
            }
            paths.insert(install.path.clone(), (r.name.clone(), r.tag.clone()));
            plan.copies.push(Copy {
                release: Release {
                    identity: r.clone(),
                    ..Release::default()
                },
                path: install.path.clone(),
                nested,
                required_by: install.required_by.clone(),
                dependencies: BTreeMap::new(),
            });
        }
    }
    plan.copies.sort_by(|a, b| {
        a.path
            .matches('/')
            .count()
            .cmp(&b.path.matches('/').count())
            .then(a.path.cmp(&b.path))
    });
    for c in &mut plan.copies {
        for (name, tag) in &c.release.identity.dependencies {
            let path = if plan.hoisted.get(name) == Some(tag) {
                format!("addons/{}", dir_name(name)?)
            } else {
                format!("{}/.gdam/{}", c.path, dir_name(name)?)
            };
            ensure!(
                paths.get(&path) == Some(&(name.clone(), tag.clone())),
                "gdam.lock records {} depending on {name}@{tag} but installs it nowhere; delete the lock and run `gdam install`",
                c.path
            );
            c.dependencies.insert(name.clone(), path);
        }
    }
    Ok(plan)
}
pub fn prepare(
    network: &Network,
    release: &lockfile::Release,
    temp: &Path,
    cache: &Path,
    offline: bool,
) -> Result<PathBuf> {
    fs::create_dir_all(temp)?;
    let package = temp.join("release-asset.zip");
    network.download(release, &package, cache, offline)?;
    let root = archive::extract(&package, &temp.join("release-asset"), false)?;
    ensure!(
        root.join("plugin.cfg").is_file(),
        "release asset {} is missing plugin.cfg at archive root",
        release.asset_name
    );
    Ok(root)
}
pub fn prune(
    project: &Project,
    previous: &Lock,
    current: &Lock,
    protected: &BTreeSet<String>,
) -> Result<()> {
    let keep = current.paths().into_iter().collect::<BTreeSet<_>>();
    for p in previous.paths() {
        if !keep.contains(&p)
            && p.starts_with("addons/")
            && !protected
                .iter()
                .any(|root| p == *root || p.starts_with(&format!("{root}/")))
        {
            project.remove(&p)?;
            println!("removed {p}");
        }
    }
    Ok(())
}
pub fn install(project: &Project, network: &Network, opts: &Options, m: &Manifest) -> Result<()> {
    let frozen = opts.frozen || opts.offline;
    let mut destinations = BTreeSet::new();
    for name in m.addons.keys() {
        let dir = dir_name(name).map_err(input)?;
        require(
            destinations.insert(dir.to_lowercase()),
            format!("addon directory collision in gdam.json: {name}"),
        )?;
    }
    let mut links = BTreeMap::new();
    for (name, a) in &m.addons {
        if linked(a) {
            links.insert(
                name.clone(),
                absolute(&project.dir, &a.link.as_ref().unwrap().path).map_err(input)?,
            );
        }
    }
    let mut pins = m.pins();
    let previous = if project.root.try_exists("gdam.lock")? {
        Some(Lock::load(&project.dir.join("gdam.lock"))?)
    } else {
        None
    };
    let (plan, lock, write_lock) = if let Some(old) = previous
        .as_ref()
        .filter(|old| links.is_empty() && old.matches(&pins))
    {
        (plan_from_lock(old).map_err(input)?, old.clone(), false)
    } else {
        if frozen {
            return Err(input(if previous.is_none() {
                "--frozen-lockfile requires gdam.lock; run `gdam install` once to create it"
            } else {
                "gdam.lock does not answer gdam.json; run `gdam install` without --frozen-lockfile to re-resolve"
            }));
        }
        for (name, tag) in &mut pins {
            if tag.is_empty() {
                *tag = network.resolve(name, "").map_err(input)?.identity.tag;
            }
        }
        let mut roots = pins.clone();
        for name in links.keys() {
            roots.insert(name.clone(), "linked".into());
        }
        let plan = resolve::resolve(&roots, |name, tag| {
            if let Some(path) = links.get(name) {
                return Ok(Release {
                    identity: lockfile::Release {
                        name: name.into(),
                        tag: tag.into(),
                        dependencies: package_dependencies(path)?,
                        ..lockfile::Release::default()
                    },
                    local: true,
                    ..Release::default()
                });
            }
            let r = network.resolve(name, tag)?;
            ensure!(
                r.identity.tag == tag,
                "registry answered {} for tag {tag}",
                r.identity.tag
            );
            Ok(r)
        })
        .map_err(input)?;
        let lock = lock_from_plan(&pins, &plan);
        (plan, lock, true)
    };
    lock.validate()?;
    let protected = plan
        .copies
        .iter()
        .filter(|c| c.release.local)
        .map(|c| c.path.clone())
        .collect::<BTreeSet<_>>();
    if plan.copies.is_empty() && m.package.is_empty() {
        if let Some(old) = &previous {
            prune(project, old, &lock, &protected)?;
        }
        if write_lock {
            project.write("gdam.lock", &lock.json_bytes()?, 0o644)?;
        }
        return Ok(());
    }
    project.root.create_dir_all("addons")?;
    let has_godot = project.root.try_exists("project.godot")?;
    let temp = tempfile::Builder::new().prefix("gdam-install-").tempdir()?;
    let cache = if opts.cache.as_os_str().is_empty() {
        cache_dir()?
    } else {
        opts.cache.clone()
    };
    let mut prepared = BTreeMap::new();
    for (i, c) in plan.copies.iter().enumerate() {
        network.check_deadline()?;
        if c.release.local {
            continue;
        }
        let root = prepare(
            network,
            &c.release.identity,
            &temp.path().join(format!("pkg-{i}")),
            &cache,
            opts.offline,
        )
        .map_err(input)?;
        prepared.insert(i, root);
    }
    for (i, c) in plan.copies.iter().enumerate() {
        network.check_deadline()?;
        if c.release.local {
            continue;
        }
        project.remove(&c.path)?;
        project.copy(&prepared[&i], &c.path)?;
        require(
            project
                .root
                .metadata(format!("{}/plugin.cfg", c.path))?
                .is_file(),
            format!("installed addon is missing plugin.cfg at {}", c.path),
        )?;
        if c.nested {
            uid::regenerate(project, &c.path)
                .with_context(|| format!("regenerating uids in {}", c.path))?;
        }
        if has_godot && c.release.identity.editor_plugin && !c.nested {
            let cfg = format!("res://{}/plugin.cfg", c.path);
            if project.set_plugin(&cfg, true)? {
                println!("enabled {cfg}");
            }
        }
        let r = &c.release.identity;
        if c.nested {
            println!(
                "installed {}@{} nested at {} ({} pins it)",
                r.name,
                r.tag,
                c.path,
                c.required_by.join(", ")
            );
        } else {
            println!("installed {}@{}", r.name, r.tag);
        }
    }
    for c in &plan.copies {
        if !c.release.local {
            crate::depsfile::write(
                project,
                &c.path,
                &lockfile::key(&c.release.identity.name, &c.release.identity.tag),
                &c.dependencies,
            )?;
        }
    }
    if !m.package.is_empty() {
        let path = project.dir.join(&m.package);
        require(
            path.is_dir(),
            format!(
                "gdam.json names package directory {:?}, which does not exist",
                m.package
            ),
        )?;
        let deps = package_dependencies(&path).map_err(input)?;
        let mut paths = BTreeMap::new();
        for (name, tag) in deps {
            let have = plan.hoisted.get(&name);
            require(
                have == Some(&tag),
                format!(
                    "{}/gdam.json declares {name}@{tag} but the project installs {}; pin the same tag in gdam.json",
                    m.package,
                    have.map(String::as_str).unwrap_or("no copy")
                ),
            )?;
            paths.insert(name.clone(), format!("addons/{}", dir_name(&name)?));
        }
        crate::depsfile::write(project, &m.package, &m.package, &paths)?;
    }
    if let Some(old) = &previous {
        prune(project, old, &lock, &protected)?;
    }
    if write_lock {
        project.write("gdam.lock", &lock.json_bytes()?, 0o644)?;
    }
    Ok(())
}
