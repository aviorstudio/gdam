use crate::{
    Result, fsutil,
    lockfile::Lock,
    manifest::{LINK_FILENAME, Manifest},
};
use anyhow::{Context, ensure};
#[cfg(unix)]
use cap_std::fs::{OpenOptionsExt, PermissionsExt};
use cap_std::{
    ambient_authority,
    fs::{Dir, Metadata, OpenOptions, Permissions},
};
use serde::{Deserialize, Serialize};
use sha2::{Digest, Sha256};
use std::{
    collections::BTreeSet,
    fs,
    io::{Read, Write},
    path::{Component, Path, PathBuf},
};
const STATE: &str = ".gdam-state";
const TRANSACTION: &str = ".gdam-state/transaction";
#[derive(Default, Serialize, Deserialize)]
#[serde(default, deny_unknown_fields)]
struct Journal {
    version: u32,
    state: String,
    entries: Vec<Entry>,
}
#[derive(Default, Serialize, Deserialize)]
#[serde(default, deny_unknown_fields)]
struct Entry {
    path: String,
    present: bool,
    #[serde(skip_serializing_if = "String::is_empty")]
    digest: String,
}
pub struct Project {
    pub dir: PathBuf,
    pub root: Dir,
}
fn parent(path: &Path) -> &Path {
    path.parent()
        .filter(|p| !p.as_os_str().is_empty())
        .unwrap_or(Path::new("."))
}
fn not_found(e: &std::io::Error) -> bool {
    e.kind() == std::io::ErrorKind::NotFound
}
fn valid_relative(path: &str) -> bool {
    !path.is_empty()
        && !path.contains('\\')
        && path
            .split('/')
            .all(|p| !p.is_empty() && p != "." && p != "..")
        && !Path::new(path).is_absolute()
}
fn open_new(root: &Dir, path: &Path, mode: u32) -> Result<cap_std::fs::File> {
    let mut o = OpenOptions::new();
    o.write(true).create_new(true);
    #[cfg(unix)]
    o.mode(mode);
    #[cfg(not(unix))]
    let _ = mode;
    Ok(root.open_with(path, &o)?)
}
fn mode(m: &Metadata) -> u32 {
    #[cfg(unix)]
    let bits = {
        let p = m.permissions().mode();
        (p & 0o777)
            | if p & 0o4000 != 0 { 1 << 23 } else { 0 }
            | if p & 0o2000 != 0 { 1 << 22 } else { 0 }
            | if p & 0o1000 != 0 { 1 << 20 } else { 0 }
    };
    #[cfg(not(unix))]
    let bits = if m.permissions().readonly() {
        0o444
    } else {
        0o666
    } | if m.is_dir() { 0o111 } else { 0 };
    bits | if m.is_dir() {
        1 << 31
    } else if m.file_type().is_symlink() {
        1 << 27
    } else {
        0
    }
}
fn list(root: &Dir, path: &str) -> Result<Vec<String>> {
    let mut names = root
        .read_dir(path)?
        .map(|r| {
            r?.file_name()
                .into_string()
                .map_err(|_| anyhow::anyhow!("project filename is not UTF-8"))
        })
        .collect::<Result<Vec<_>>>()?;
    names.sort();
    Ok(names)
}
fn link(root: &Dir, target: &Path, destination: &str, directory: bool) -> Result<()> {
    #[cfg(not(windows))]
    {
        let _ = directory;
        root.symlink_contents(target, destination)?;
    }
    #[cfg(windows)]
    {
        if directory {
            cap_primitives::fs::symlink_dir_contents(
                target,
                &root.try_clone()?.into_std_file(),
                Path::new(destination),
            )?;
        } else {
            cap_primitives::fs::symlink_file_contents(
                target,
                &root.try_clone()?.into_std_file(),
                Path::new(destination),
            )?;
        }
    }
    Ok(())
}
fn copy_snapshot(root: &Dir, src: &str, dst: &str) -> Result<()> {
    let m = root.symlink_metadata(src)?;
    root.create_dir_all(parent(Path::new(dst)))?;
    if m.file_type().is_symlink() {
        return link(
            root,
            &root.read_link_contents(src)?,
            dst,
            is_directory_link(&m),
        );
    }
    if m.is_dir() {
        root.create_dir(dst)?;
        for name in list(root, src)? {
            copy_snapshot(root, &format!("{src}/{name}"), &format!("{dst}/{name}"))?;
        }
        root.set_permissions(dst, m.permissions())?;
        return Ok(());
    }
    ensure!(m.is_file(), "unsupported project file {src}");
    let mut input = root.open(src)?;
    let mut output = open_new(root, Path::new(dst), mode(&m) & 0o777)?;
    std::io::copy(&mut input, &mut output)?;
    output.set_permissions(m.permissions())?;
    output.sync_all()?;
    Ok(())
}
fn is_directory_link(m: &Metadata) -> bool {
    #[cfg(windows)]
    {
        use cap_std::fs::MetadataExt;
        m.file_attributes() & 0x10 != 0
    }
    #[cfg(not(windows))]
    {
        let _ = m;
        false
    }
}
fn tree_digest(root: &Dir, start: &str) -> Result<String> {
    fn walk(root: &Dir, path: &str, relative: &str, h: &mut Sha256) -> Result<()> {
        let m = root.symlink_metadata(path)?;
        h.update(format!("{relative}\0{}\0", mode(&m)));
        if m.file_type().is_symlink() {
            h.update(
                root.read_link_contents(path)?
                    .to_str()
                    .ok_or_else(|| anyhow::anyhow!("link target is not UTF-8"))?
                    .as_bytes(),
            );
            return Ok(());
        }
        if m.is_dir() {
            for name in list(root, path)? {
                walk(
                    root,
                    &format!("{path}/{name}"),
                    &format!("{relative}/{name}"),
                    h,
                )?;
            }
            return Ok(());
        }
        ensure!(m.is_file(), "unsupported snapshot file");
        let mut f = root.open(path)?;
        let mut buffer = [0; 65536];
        loop {
            let n = f.read(&mut buffer)?;
            if n == 0 {
                break;
            }
            h.update(&buffer[..n]);
        }
        Ok(())
    }
    let mut h = Sha256::new();
    walk(root, start, ".", &mut h)?;
    Ok(format!("{:x}", h.finalize()))
}
fn check_parents(root: &Dir, path: &str) -> Result<()> {
    ensure!(valid_relative(path), "invalid project destination {path:?}");
    let parts = path.split('/').collect::<Vec<_>>();
    for i in 1..parts.len() {
        let p = parts[..i].join("/");
        match root.symlink_metadata(&p) {
            Ok(m) => ensure!(
                m.is_dir() && !m.file_type().is_symlink(),
                "refusing non-directory or linked destination parent {p}"
            ),
            Err(e) if not_found(&e) => {}
            Err(e) => return Err(e.into()),
        }
    }
    Ok(())
}
fn remove(root: &Dir, path: &str) -> Result<()> {
    match root.symlink_metadata(path) {
        Ok(m) if m.is_dir() && !m.file_type().is_symlink() => root.remove_dir_all(path)?,
        Ok(m) if is_directory_link(&m) => root.remove_dir(path)?,
        Ok(_) => root.remove_file(path)?,
        Err(e) if not_found(&e) => {}
        Err(e) => return Err(e.into()),
    }
    Ok(())
}
fn save_journal(root: &Dir, j: &Journal) -> Result<()> {
    let tmp = format!("{TRANSACTION}/journal.tmp");
    remove(root, &tmp)?;
    let mut file = open_new(root, Path::new(&tmp), 0o600)?;
    file.write_all(&serde_json::to_vec(j)?)?;
    file.sync_all()?;
    drop(file);
    root.rename(&tmp, root, format!("{TRANSACTION}/journal.json"))?;
    #[cfg(unix)]
    root.open(TRANSACTION)?.sync_all()?;
    Ok(())
}
fn recover(root: &Dir) -> Result<()> {
    match root.symlink_metadata(TRANSACTION) {
        Err(e) if not_found(&e) => return Ok(()),
        Err(e) => return Err(e.into()),
        Ok(m) => ensure!(
            m.is_dir() && !m.file_type().is_symlink(),
            "invalid transaction directory"
        ),
    }
    let mut raw = Vec::new();
    match root.open(format!("{TRANSACTION}/journal.json")) {
        Err(e) if not_found(&e) => return remove(root, TRANSACTION),
        Err(e) => return Err(e.into()),
        Ok(f) => {
            f.take((1 << 20) + 1).read_to_end(&mut raw)?;
        }
    }
    ensure!(
        raw.len() <= 1 << 20,
        "invalid gdam transaction journal; preserved for inspection"
    );
    let j: Journal = serde_json::from_slice(&raw)
        .context("invalid gdam transaction journal; preserved for inspection")?;
    ensure!(
        j.version == 1,
        "invalid gdam transaction journal; preserved for inspection"
    );
    if j.state == "preparing" || j.state == "committed" {
        return remove(root, TRANSACTION);
    }
    ensure!(j.state == "prepared", "invalid gdam transaction state");
    let mut seen = BTreeSet::new();
    for e in &j.entries {
        let allowed = [
            "gdam.json",
            LINK_FILENAME,
            "gdam.lock",
            "project.godot",
            "addons",
        ]
        .contains(&e.path.as_str())
            || (valid_relative(&e.path)
                && e.path.ends_with("/.gdam")
                && e.path.split('/').next() != Some(STATE));
        ensure!(
            allowed && seen.insert(&e.path) && (!e.present || e.digest.len() == 64),
            "invalid recovery destination"
        );
        check_parents(root, &e.path)?;
    }
    for (i, e) in j.entries.iter().enumerate() {
        let backup = format!("{TRANSACTION}/backup/{i}");
        if e.present {
            match root.symlink_metadata(&backup) {
                Err(err) if not_found(&err) => {
                    ensure!(
                        tree_digest(root, &e.path).is_ok_and(|d| d == e.digest),
                        "recovery backup missing and destination differs; state preserved"
                    );
                    continue;
                }
                Err(err) => return Err(err.into()),
                Ok(_) => {}
            }
            ensure!(
                tree_digest(root, &backup).is_ok_and(|d| d == e.digest),
                "recovery backup differs; state preserved"
            );
        }
        remove(root, &e.path)?;
        if e.present {
            root.create_dir_all(parent(Path::new(&e.path)))?;
            root.rename(&backup, root, &e.path)?;
        }
    }
    remove(root, TRANSACTION)
}
fn private_dir(root: &Dir, path: &str) -> Result<()> {
    #[cfg(windows)]
    let builder = cap_std::fs::DirBuilder::new();
    #[cfg(unix)]
    let mut builder = cap_std::fs::DirBuilder::new();
    #[cfg(unix)]
    {
        use cap_std::fs::DirBuilderExt;
        builder.mode(0o700);
    }
    root.create_dir_with(path, &builder)?;
    Ok(())
}
pub fn find(start: &Path) -> Option<PathBuf> {
    start
        .ancestors()
        .find(|p| p.join("gdam.json").exists() || p.join(TRANSACTION).join("journal.json").exists())
        .map(Path::to_owned)
}
impl Project {
    pub fn with_mutation<T>(action: impl FnOnce(&Self) -> Result<T>) -> Result<T> {
        let dir = find(&std::env::current_dir()?)
            .ok_or_else(|| crate::input("no gdam.json found (run `gdam init`)"))?;
        let project = Self {
            root: Dir::open_ambient_dir(&dir, ambient_authority())?,
            dir,
        };
        let root = &project.root;
        match root.symlink_metadata(STATE) {
            Ok(m) => {
                ensure!(
                    m.is_dir() && !m.file_type().is_symlink(),
                    "gdam state directory must be a private regular directory"
                );
                #[cfg(unix)]
                ensure!(
                    m.permissions().mode() & 0o77 == 0,
                    "gdam state directory must be a private regular directory"
                );
            }
            Err(e) if not_found(&e) => private_dir(root, STATE)?,
            Err(e) => return Err(e.into()),
        }
        for name in [".gitignore", "operation.lock"] {
            let p = format!("{STATE}/{name}");
            match root.symlink_metadata(&p) {
                Ok(m) => ensure!(
                    m.is_file() && !m.file_type().is_symlink(),
                    "invalid gdam state file {name}"
                ),
                Err(e) if not_found(&e) => {}
                Err(e) => return Err(e.into()),
            }
        }
        root.write(format!("{STATE}/.gitignore"), b"*\n")?;
        #[cfg(unix)]
        root.set_permissions(format!("{STATE}/.gitignore"), Permissions::from_mode(0o600))?;
        let mut options = OpenOptions::new();
        options.create(true).read(true).write(true);
        #[cfg(unix)]
        options.mode(0o600);
        let guard = root
            .open_with(format!("{STATE}/operation.lock"), &options)?
            .into_std();
        guard
            .try_lock()
            .context("another gdam operation may be running")?;
        recover(root).context("recovering interrupted gdam operation")?;
        let mut paths = [
            "gdam.json",
            LINK_FILENAME,
            "gdam.lock",
            "project.godot",
            "addons",
        ]
        .into_iter()
        .map(str::to_owned)
        .collect::<Vec<_>>();
        for p in &paths {
            match root.symlink_metadata(p) {
                Ok(m) => ensure!(
                    !m.file_type().is_symlink()
                        && if p == "addons" {
                            m.is_dir()
                        } else {
                            m.is_file()
                        },
                    "refusing nonregular project path {p}"
                ),
                Err(e) if not_found(&e) => {}
                Err(e) => return Err(e.into()),
            }
        }
        let manifest = project.manifest()?;
        if !manifest.package.is_empty() {
            ensure!(
                valid_relative(&manifest.package)
                    && manifest.package.split('/').next() != Some(STATE),
                "package must name a directory inside the project"
            );
            let p = format!("{}/.gdam", manifest.package);
            check_parents(root, &format!("{p}/deps.gd"))?;
            if !p.starts_with("addons/") {
                paths.push(p);
            }
        }
        if root.try_exists("gdam.lock")? {
            let lock = Lock::load(&project.dir.join("gdam.lock"))?;
            for p in lock.paths() {
                check_parents(root, &p)?;
            }
        }
        private_dir(root, TRANSACTION)?;
        private_dir(root, &format!("{TRANSACTION}/backup"))?;
        let mut journal = Journal {
            version: 1,
            state: "preparing".into(),
            entries: Vec::new(),
        };
        save_journal(root, &journal)?;
        for (i, path) in paths.into_iter().enumerate() {
            let mut e = Entry {
                path,
                ..Entry::default()
            };
            match root.symlink_metadata(&e.path) {
                Ok(_) => {
                    e.present = true;
                    let backup = format!("{TRANSACTION}/backup/{i}");
                    copy_snapshot(root, &e.path, &backup)?;
                    e.digest = tree_digest(root, &backup)?;
                }
                Err(err) if not_found(&err) => {}
                Err(err) => return Err(err.into()),
            }
            journal.entries.push(e);
        }
        journal.state = "prepared".into();
        save_journal(root, &journal)?;
        match action(&project) {
            Err(error) => {
                if let Err(recovery) = recover(root) {
                    return Err(error.context(format!(
                        "previous state retained for recovery: {recovery:#}"
                    )));
                }
                Err(error)
            }
            Ok(value) => {
                journal.state = "committed".into();
                if let Err(e) = save_journal(root, &journal) {
                    recover(root)?;
                    return Err(e);
                }
                if remove(root, TRANSACTION).is_err() {
                    eprintln!(
                        "gdam: update committed; temporary backup cleanup will retry on the next operation"
                    );
                }
                Ok(value)
            }
        }
    }
    pub fn manifest(&self) -> Result<Manifest> {
        Manifest::load(&self.dir.join("gdam.json"))
    }
    pub fn relative<'a>(&self, path: &'a Path) -> Result<&'a Path> {
        let relative = path.strip_prefix(&self.dir)?;
        ensure!(
            relative
                .components()
                .all(|p| matches!(p, Component::Normal(_))),
            "path must stay inside project"
        );
        Ok(relative)
    }
    pub fn write(&self, path: &str, bytes: &[u8], mode: u32) -> Result<()> {
        check_parents(&self.root, path)?;
        let parent = parent(Path::new(path));
        self.root.create_dir_all(parent)?;
        let mut random = [0; 16];
        getrandom::fill(&mut random)?;
        let tmp = parent.join(format!(".gdam-write-{:x}", u128::from_ne_bytes(random)));
        let result = (|| -> Result<()> {
            let mut f = open_new(&self.root, &tmp, mode)?;
            f.write_all(bytes)?;
            f.sync_all()?;
            drop(f);
            self.root.rename(&tmp, &self.root, path)?;
            Ok(())
        })();
        let _ = self.root.remove_file(&tmp);
        result
    }
    pub fn remove(&self, path: &str) -> Result<()> {
        check_parents(&self.root, path)?;
        remove(&self.root, path)
    }
    pub fn link(&self, target: &Path, dst: &str) -> Result<()> {
        check_parents(&self.root, dst)?;
        link(&self.root, target, dst, true)
    }
    pub fn copy(&self, src: &Path, dst: &str) -> Result<()> {
        check_parents(&self.root, dst)?;
        let m = fs::symlink_metadata(src)?;
        ensure!(
            !m.file_type().is_symlink(),
            "refusing package symlink {}",
            src.display()
        );
        if m.is_dir() {
            self.root.create_dir_all(dst)?;
            let mut children = fs::read_dir(src)?.collect::<std::io::Result<Vec<_>>>()?;
            children.sort_by_key(|e| e.file_name());
            for e in children {
                self.copy(
                    &e.path(),
                    &format!(
                        "{dst}/{}",
                        e.file_name()
                            .to_str()
                            .ok_or_else(|| anyhow::anyhow!("package path is not UTF-8"))?
                    ),
                )?;
            }
            return Ok(());
        }
        ensure!(m.is_file(), "package contains a nonregular file");
        self.root.create_dir_all(parent(Path::new(dst)))?;
        let mut input = fs::File::open(src)?;
        let mut output = open_new(&self.root, Path::new(dst), 0o644)?;
        std::io::copy(&mut input, &mut output)?;
        output.set_permissions(Permissions::from_std(m.permissions()))?;
        Ok(())
    }
    pub fn save_manifest(&self, m: &Manifest) -> Result<()> {
        let links = crate::manifest::LinkManifest {
            addons: m
                .addons
                .iter()
                .filter_map(|(n, a)| a.link.as_ref().map(|l| (n.clone(), l.clone())))
                .collect(),
        };
        if links.addons.is_empty() {
            self.remove(LINK_FILENAME)?;
        } else {
            self.write(LINK_FILENAME, &fsutil::json_bytes(&links)?, 0o644)?;
        }
        self.write("gdam.json", &fsutil::json_bytes(m)?, 0o644)
    }
}
