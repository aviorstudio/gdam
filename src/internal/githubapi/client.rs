use crate::{Result, lockfile::Release, network::Network};
use anyhow::ensure;
use serde::Deserialize;
use sha2::{Digest, Sha256};
use std::{
    fs,
    io::{Read, Write},
    path::{Path, PathBuf},
};
pub const MAX_DOWNLOAD: u64 = 128 << 20;
pub fn digest_hex(value: &str) -> Result<String> {
    let (kind, hex) = value
        .trim()
        .split_once(':')
        .ok_or_else(|| anyhow::anyhow!("unsupported release asset digest (expected sha256)"))?;
    ensure!(
        kind.eq_ignore_ascii_case("sha256")
            && hex.len() == 64
            && hex.bytes().all(|b| b.is_ascii_hexdigit()),
        "invalid sha256 release asset digest"
    );
    Ok(hex.to_ascii_lowercase())
}
fn copy_verified(
    mut input: impl Read,
    destination: &Path,
    digest: &str,
    length: Option<u64>,
) -> Result<()> {
    let want = digest_hex(digest)?;
    let mut out = tempfile::NamedTempFile::new_in(destination.parent().unwrap_or(Path::new(".")))?;
    let mut hash = Sha256::new();
    let mut total = 0_u64;
    let mut buffer = [0; 65536];
    loop {
        let n = input.read(&mut buffer)?;
        if n == 0 {
            break;
        }
        total += n as u64;
        ensure!(
            total <= MAX_DOWNLOAD,
            "release asset exceeds {MAX_DOWNLOAD}-byte download limit"
        );
        out.write_all(&buffer[..n])?;
        hash.update(&buffer[..n]);
    }
    ensure!(
        length.is_none_or(|n| n == total),
        "release asset truncated: received {total} bytes"
    );
    ensure!(
        format!("{:x}", hash.finalize()) == want,
        "release asset digest mismatch: expected {digest}"
    );
    out.as_file().sync_all()?;
    out.persist(destination)?;
    Ok(())
}
fn cache_path(cache: &Path, digest: &str) -> Result<PathBuf> {
    ensure!(
        !cache.as_os_str().is_empty(),
        "offline install requires an archive cache"
    );
    Ok(cache.join(format!("{}.zip", digest_hex(digest)?)))
}
pub fn copy_cached(cache: &Path, destination: &Path, digest: &str) -> Result<()> {
    let path = cache_path(cache, digest)?;
    let m = fs::symlink_metadata(&path)?;
    ensure!(
        m.is_file() && m.len() <= MAX_DOWNLOAD,
        "verified archive unavailable in cache"
    );
    copy_verified(fs::File::open(path)?, destination, digest, None)
}
#[derive(Deserialize)]
#[serde(deny_unknown_fields)]
struct Binding {
    repository: String,
    tag: String,
    publisher_commit: String,
    source_commit: String,
}
fn reviewed_source(
    owner: &str,
    repo: &str,
    tag: &str,
    publisher: &str,
    source: &str,
) -> Result<bool> {
    let rows: Vec<Binding> = serde_json::from_str(include_str!("reviewed_release_sources.json"))?;
    let repo_pattern = regex::Regex::new(r"^[a-z0-9][a-z0-9_.-]*/[a-z0-9][a-z0-9_.-]*$")?;
    let commit_pattern = regex::Regex::new(r"^[a-f0-9]{40}$")?;
    let mut seen = std::collections::BTreeSet::new();
    let mut matched = false;
    for r in rows {
        ensure!(
            repo_pattern.is_match(&r.repository)
                && !r.tag.is_empty()
                && r.tag.trim() == r.tag
                && commit_pattern.is_match(&r.publisher_commit)
                && commit_pattern.is_match(&r.source_commit)
                && r.publisher_commit != r.source_commit
                && seen.insert((
                    r.repository.clone(),
                    r.tag.clone(),
                    r.publisher_commit.clone()
                )),
            "invalid reviewed release source"
        );
        if r.repository == format!("{owner}/{repo}").to_ascii_lowercase()
            && r.tag == tag
            && r.publisher_commit.eq_ignore_ascii_case(publisher)
            && r.source_commit.eq_ignore_ascii_case(source)
        {
            matched = true;
        }
    }
    Ok(matched)
}
pub(crate) fn timestamp(s: &str) -> Result<chrono::DateTime<chrono::FixedOffset>> {
    let time = chrono::DateTime::parse_from_rfc3339(s)?;
    ensure!(
        time.timestamp() != -62135596800 || time.timestamp_subsec_nanos() != 0,
        "release publication time must not be zero"
    );
    Ok(time)
}
impl Network {
    pub fn verify_release(&self, r: &Release) -> Result<()> {
        let release_id = r.github_release_id.to_string();
        let asset_id = r.asset_id.to_string();
        let base = ["repos", &r.github_owner, &r.github_repo];
        let release = self.github_json(&[base.as_slice(), &["releases", &release_id]].concat())?;
        ensure!(
            release["draft"].as_bool() == Some(false)
                && release["id"].as_i64() == Some(r.github_release_id)
                && release["tag_name"].as_str() == Some(r.tag.as_str())
                && timestamp(release["published_at"].as_str().unwrap_or(""))?
                    == timestamp(&r.published_at)?
                && release["prerelease"].as_bool() == Some(r.prerelease),
            "registered release identity drift detected"
        );
        let commit = self.github_json(&[base.as_slice(), &["commits", &r.tag]].concat())?;
        let source = commit["sha"]
            .as_str()
            .ok_or_else(|| anyhow::anyhow!("missing tag commit"))?;
        ensure!(
            source.eq_ignore_ascii_case(&r.commit_sha)
                || reviewed_source(
                    &r.github_owner,
                    &r.github_repo,
                    &r.tag,
                    &r.commit_sha,
                    source
                )?,
            "registered release commit drift detected"
        );
        let asset =
            self.github_json(&[base.as_slice(), &["releases", "assets", &asset_id]].concat())?;
        ensure!(
            asset["id"].as_i64() == Some(r.asset_id)
                && asset["name"].as_str() == Some(r.asset_name.as_str())
                && asset["digest"].as_str() == Some(r.asset_digest.as_str())
                && asset["state"].as_str() == Some("uploaded"),
            "registered release asset drift detected"
        );
        Ok(())
    }
    pub fn download(
        &self,
        r: &Release,
        destination: &Path,
        cache: &Path,
        offline: bool,
    ) -> Result<()> {
        ensure!(
            r.github_release_id > 0
                && r.asset_id > 0
                && !r.tag.trim().is_empty()
                && !r.commit_sha.trim().is_empty()
                && !r.asset_name.trim().is_empty()
                && !r.asset_digest.trim().is_empty()
                && !r.published_at.is_empty(),
            "incomplete verified release identity"
        );
        digest_hex(&r.asset_digest)?;
        timestamp(&r.published_at)?;
        if offline {
            return copy_cached(cache, destination, &r.asset_digest);
        }
        self.verify_release(r)?;
        if !cache.as_os_str().is_empty() && copy_cached(cache, destination, &r.asset_digest).is_ok()
        {
            return Ok(());
        }
        let response = self.github_download(&[
            "repos",
            &r.github_owner,
            &r.github_repo,
            "releases",
            "assets",
            &r.asset_id.to_string(),
        ])?;
        let length = response.content_length();
        ensure!(
            length.is_none_or(|n| n <= MAX_DOWNLOAD),
            "release asset exceeds {MAX_DOWNLOAD}-byte download limit"
        );
        copy_verified(response, destination, &r.asset_digest, length)?;
        if !cache.as_os_str().is_empty() {
            let mut builder = fs::DirBuilder::new();
            builder.recursive(true);
            #[cfg(unix)]
            {
                use std::os::unix::fs::DirBuilderExt;
                builder.mode(0o700);
            }
            builder.create(cache)?;
            ensure!(
                fs::symlink_metadata(cache)?.is_dir(),
                "archive cache must be a real directory"
            );
            copy_verified(
                fs::File::open(destination)?,
                &cache_path(cache, &r.asset_digest)?,
                &r.asset_digest,
                None,
            )?;
        }
        Ok(())
    }
    pub fn publication_identity(
        &self,
        owner: &str,
        repo: &str,
        tag: &str,
        asset_name: &str,
    ) -> Result<(Release, u64)> {
        let release = self.github_json(&["repos", owner, repo, "releases", "tags", tag])?;
        let release_id = release["id"].as_i64().unwrap_or(0);
        let published = release["published_at"].as_str().unwrap_or("");
        timestamp(published)?;
        ensure!(
            release_id > 0
                && release["tag_name"].as_str() == Some(tag)
                && release["draft"].as_bool() == Some(false),
            "release is not published or has a different tag"
        );
        let assets = release["assets"]
            .as_array()
            .ok_or_else(|| anyhow::anyhow!("release has no assets"))?;
        let matches = assets
            .iter()
            .filter(|a| {
                a["state"] == "uploaded" && (asset_name.is_empty() || a["name"] == asset_name)
            })
            .collect::<Vec<_>>();
        ensure!(
            matches.len() == 1,
            "name exactly one uploaded release asset"
        );
        let a = matches[0];
        let asset_id = a["id"].as_i64().unwrap_or(0);
        let size = a["size"].as_u64().unwrap_or(0);
        ensure!(
            asset_id > 0 && size > 0,
            "name exactly one uploaded release asset"
        );
        let digest = a["digest"].as_str().unwrap_or("").to_ascii_lowercase();
        digest_hex(&digest)?;
        let commit = self.github_json(&["repos", owner, repo, "commits", tag])?;
        let sha = commit["sha"].as_str().unwrap_or("");
        ensure!(
            [40, 64].contains(&sha.len()) && sha.bytes().all(|b| b.is_ascii_hexdigit()),
            "tag does not resolve to a commit"
        );
        Ok((
            Release {
                name: format!("@{owner}/{repo}"),
                tag: tag.into(),
                github_owner: owner.into(),
                github_repo: repo.into(),
                github_release_id: release_id,
                commit_sha: sha.to_ascii_lowercase(),
                asset_id,
                asset_name: a["name"].as_str().unwrap_or("").into(),
                asset_digest: digest,
                published_at: published.into(),
                prerelease: release["prerelease"].as_bool().unwrap_or(false),
                ..Release::default()
            },
            size,
        ))
    }
}

#[cfg(test)]
mod source_binding_tests {
    use super::*;
    #[test]
    fn reviewed_legacy_bindings_are_exact_repository_tag_and_both_commits() {
        let rows: Vec<Binding> =
            serde_json::from_str(include_str!("reviewed_release_sources.json")).unwrap();
        for row in rows {
            let (owner, repo) = row.repository.split_once('/').unwrap();
            assert!(
                reviewed_source(
                    owner,
                    repo,
                    &row.tag,
                    &row.publisher_commit,
                    &row.source_commit
                )
                .unwrap()
            );
            for (o, r, t, p, s) in [
                (
                    "other",
                    repo,
                    row.tag.as_str(),
                    row.publisher_commit.as_str(),
                    row.source_commit.as_str(),
                ),
                (
                    owner,
                    "different",
                    row.tag.as_str(),
                    row.publisher_commit.as_str(),
                    row.source_commit.as_str(),
                ),
                (
                    owner,
                    repo,
                    "other-tag",
                    row.publisher_commit.as_str(),
                    row.source_commit.as_str(),
                ),
                (
                    owner,
                    repo,
                    row.tag.as_str(),
                    "0000000000000000000000000000000000000000",
                    row.source_commit.as_str(),
                ),
                (
                    owner,
                    repo,
                    row.tag.as_str(),
                    row.publisher_commit.as_str(),
                    "0000000000000000000000000000000000000000",
                ),
            ] {
                assert!(!reviewed_source(o, r, t, p, s).unwrap());
            }
        }
    }
}
