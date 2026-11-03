use crate::Result;
use anyhow::ensure;
use cap_std::{
    ambient_authority,
    fs::{Dir, OpenOptions},
};
use std::{
    collections::BTreeSet,
    fs,
    io::Read,
    path::{Component, Path, PathBuf},
};
const MAX_BYTES: u64 = 512 << 20;
pub fn extract(path: &Path, destination: &Path, require_single_root: bool) -> Result<PathBuf> {
    let mut archive = zip::ZipArchive::new(fs::File::open(path)?)?;
    ensure!(archive.len() <= 10000, "archive exceeds 10000-entry limit");
    let mut total = 0_u64;
    for i in 0..archive.len() {
        total = total
            .checked_add(archive.by_index(i)?.size())
            .ok_or_else(|| anyhow::anyhow!("archive byte size overflow"))?;
        ensure!(
            total <= MAX_BYTES,
            "archive exceeds {MAX_BYTES}-byte uncompressed limit"
        );
    }
    fs::create_dir_all(destination)?;
    let root = Dir::open_ambient_dir(destination, ambient_authority())?;
    let mut roots = BTreeSet::new();
    for i in 0..archive.len() {
        let mut file = archive.by_index(i)?;
        let name = file.name()?.into_owned();
        ensure!(
            !name.starts_with('/') && !name.contains('\\'),
            "invalid zip entry path: {name}"
        );
        let entry = Path::new(&name);
        ensure!(
            !entry.components().any(|c| matches!(
                c,
                Component::ParentDir | Component::RootDir | Component::Prefix(_)
            )),
            "invalid zip entry path: {name}"
        );
        if entry.components().all(|c| c == Component::CurDir) {
            continue;
        }
        let enclosed = file
            .enclosed_name()
            .ok_or_else(|| anyhow::anyhow!("invalid zip entry path: {name}"))?;
        let first = enclosed
            .components()
            .next()
            .ok_or_else(|| anyhow::anyhow!("empty zip entry"))?;
        roots.insert(first.as_os_str().to_owned());
        let mode = file.unix_mode().unwrap_or(0o644);
        ensure!(
            mode & 0o170000 != 0o120000,
            "refusing to extract symlink: {name}"
        );
        ensure!(
            mode & 0o170000 == 0 || mode & 0o170000 == 0o100000 || mode & 0o170000 == 0o40000,
            "refusing nonregular archive entry: {name}"
        );
        if file.is_dir() {
            root.create_dir_all(&enclosed)?;
            continue;
        }
        root.create_dir_all(enclosed.parent().unwrap_or(Path::new(".")))?;
        let mut options = OpenOptions::new();
        options.create_new(true).write(true);
        #[cfg(unix)]
        {
            use cap_std::fs::OpenOptionsExt;
            options.mode(mode & 0o777);
        }
        let mut output = root.open_with(&enclosed, &options)?;
        let size = file.size();
        let n = std::io::copy(&mut (&mut file).take(MAX_BYTES + 1), &mut output)?;
        ensure!(
            n == size && n <= MAX_BYTES,
            "archive entry size mismatch: {name}"
        );
        output.sync_all()?;
    }
    let result = if roots.len() == 1 {
        destination.join(roots.first().unwrap())
    } else {
        destination.to_owned()
    };
    if require_single_root {
        ensure!(
            roots.len() == 1 && result.is_dir(),
            "unexpected zip layout (expected single root dir)"
        );
    }
    Ok(if result.is_dir() {
        result
    } else {
        destination.to_owned()
    })
}
