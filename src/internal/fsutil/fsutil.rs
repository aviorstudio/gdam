use crate::Result;
use anyhow::ensure;
use serde::{Serialize, de::DeserializeOwned};
use std::{
    fs,
    io::{Read, Write},
    path::Path,
};
pub const MAX_JSON: u64 = 4 << 20;
pub fn read_json<T: DeserializeOwned>(path: &Path) -> Result<T> {
    let bytes = read_limited(path, MAX_JSON)?;
    Ok(serde_json::from_slice(&bytes)?)
}
pub fn read_limited(path: &Path, limit: u64) -> Result<Vec<u8>> {
    let mut out = Vec::new();
    fs::File::open(path)?
        .take(limit + 1)
        .read_to_end(&mut out)?;
    ensure!(
        out.len() as u64 <= limit,
        "file exceeds {limit}-byte limit: {}",
        path.display()
    );
    Ok(out)
}
pub fn atomic(path: &Path, bytes: &[u8], mode: u32) -> Result<()> {
    let mut temp = tempfile::NamedTempFile::new_in(path.parent().unwrap_or(Path::new(".")))?;
    #[cfg(unix)]
    {
        use std::os::unix::fs::PermissionsExt;
        temp.as_file()
            .set_permissions(fs::Permissions::from_mode(mode))?;
    }
    #[cfg(not(unix))]
    let _ = mode;
    temp.write_all(bytes)?;
    temp.as_file().sync_all()?;
    temp.persist(path)?;
    Ok(())
}
pub fn json_bytes(value: &impl Serialize) -> Result<Vec<u8>> {
    let text = serde_json::to_string_pretty(value)?
        .replace('&', "\\u0026")
        .replace('<', "\\u003c")
        .replace('>', "\\u003e")
        .replace('\u{2028}', "\\u2028")
        .replace('\u{2029}', "\\u2029");
    Ok(format!("{text}\n").into_bytes())
}
pub fn save_json(path: &Path, value: &impl Serialize) -> Result<()> {
    atomic(path, &json_bytes(value)?, 0o644)
}
pub fn remove(path: &Path) -> Result<()> {
    match fs::symlink_metadata(path) {
        Ok(m) if m.is_dir() && !m.file_type().is_symlink() => fs::remove_dir_all(path)?,
        Ok(_) => fs::remove_file(path)?,
        Err(e) if e.kind() == std::io::ErrorKind::NotFound => {}
        Err(e) => return Err(e.into()),
    }
    Ok(())
}
