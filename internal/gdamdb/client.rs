use crate::{Result, lockfile, resolve::Release, spec::PackageSpec};
use anyhow::{bail, ensure};
use reqwest::{
    Url,
    blocking::{Client, Response},
    header,
};
use serde_json::{Value, json};
use std::{
    io::Read,
    time::{Duration, Instant},
};
pub struct Network {
    client: Client,
    github_token: String,
    pub api: Url,
    pub github: Url,
    deadline: Instant,
}
fn origin(a: &Url, b: &Url) -> bool {
    a.scheme() == b.scheme()
        && a.host_str() == b.host_str()
        && a.port_or_known_default() == b.port_or_known_default()
}
fn response_bytes(response: Response, limit: u64) -> Result<Vec<u8>> {
    ensure!(
        response.content_length().is_none_or(|n| n <= limit),
        "response exceeds {limit}-byte limit"
    );
    let mut bytes = Vec::new();
    response.take(limit + 1).read_to_end(&mut bytes)?;
    ensure!(
        bytes.len() as u64 <= limit,
        "response exceeds {limit}-byte limit"
    );
    Ok(bytes)
}
impl Network {
    pub fn new(timeout: Duration) -> Result<Self> {
        let base = std::env::var("GDAM_API_URL")
            .ok()
            .filter(|s| !s.trim().is_empty())
            .unwrap_or_else(|| "https://api.gdam.dev".into());
        Self::with_bases(
            &base,
            "https://api.github.com",
            std::env::var("GITHUB_TOKEN").unwrap_or_default(),
            timeout,
        )
    }
    pub fn with_bases(api: &str, github: &str, token: String, timeout: Duration) -> Result<Self> {
        let _ = rustls::crypto::ring::default_provider().install_default();
        let client = Client::builder()
            .redirect(reqwest::redirect::Policy::none())
            .connect_timeout(Duration::from_secs(15))
            .timeout(Duration::from_secs(120))
            .build()?;
        let parse = |s: &str| -> Result<Url> {
            let u = Url::parse(&format!("{}/", s.trim().trim_end_matches('/')))?;
            ensure!(
                ["https", "http"].contains(&u.scheme())
                    && u.username().is_empty()
                    && u.password().is_none()
                    && u.query().is_none()
                    && u.fragment().is_none(),
                "invalid API base URL"
            );
            Ok(u)
        };
        let token = token.trim();
        let token = if token.is_empty()
            || token.to_ascii_lowercase().starts_with("bearer ")
            || token.to_ascii_lowercase().starts_with("token ")
        {
            token.into()
        } else {
            format!("Bearer {token}")
        };
        Ok(Self {
            client,
            github_token: token,
            api: parse(api)?,
            github: parse(github)?,
            deadline: Instant::now() + timeout,
        })
    }
    pub fn check_deadline(&self) -> Result<()> {
        ensure!(Instant::now() < self.deadline, "gdam operation timed out");
        Ok(())
    }
    fn url(base: &Url, parts: &[&str]) -> Result<Url> {
        let mut u = base.clone();
        u.path_segments_mut()
            .map_err(|_| anyhow::anyhow!("API URL cannot hold path segments"))?
            .pop_if_empty()
            .extend(parts);
        Ok(u)
    }
    fn request(
        &self,
        mut url: Url,
        github: bool,
        body: Option<&Value>,
        key: &str,
        accept: &str,
    ) -> Result<Response> {
        let original = url.clone();
        for redirects in 0..=5 {
            self.check_deadline()?;
            let remaining =
                self.deadline
                    .saturating_duration_since(Instant::now())
                    .min(if github {
                        Duration::from_secs(120)
                    } else {
                        Duration::from_secs(30)
                    });
            let mut req = if body.is_some() {
                self.client.post(url.clone())
            } else {
                self.client.get(url.clone())
            }
            .header(header::ACCEPT, accept)
            .header(header::USER_AGENT, "gdam")
            .timeout(remaining);
            if github {
                req = req.header("X-GitHub-Api-Version", "2022-11-28");
                if !self.github_token.is_empty() && origin(&url, &original) {
                    req = req.header(header::AUTHORIZATION, &self.github_token);
                }
            } else if !key.is_empty() {
                req = req.bearer_auth(key);
            }
            if let Some(body) = body {
                req = req.json(body);
            }
            let response = req.send()?;
            if response.status().is_redirection() {
                ensure!(
                    github && body.is_none() && redirects < 5,
                    "unexpected or excessive API redirect"
                );
                let location = response
                    .headers()
                    .get(header::LOCATION)
                    .ok_or_else(|| anyhow::anyhow!("redirect has no location"))?
                    .to_str()?;
                url = url.join(location)?;
                ensure!(
                    ["http", "https"].contains(&url.scheme()),
                    "unsupported download redirect scheme"
                );
                continue;
            }
            if !response.status().is_success() {
                let status = response.status().as_u16();
                let raw = response_bytes(response, 32 << 10)?;
                let mut text = String::from_utf8_lossy(&raw).trim().to_owned();
                if !key.is_empty() {
                    text = text.replace(key, "[redacted]");
                }
                if !self.github_token.is_empty() {
                    text = text.replace(&self.github_token, "[redacted]");
                }
                if !github
                    && let Ok(v) = serde_json::from_str::<Value>(&text)
                    && let Some(msg) = v["message"].as_str().filter(|s| !s.trim().is_empty())
                {
                    bail!("{msg}");
                }
                bail!(
                    "{} request failed ({status}): {text}",
                    if github { "github" } else { "gdam api" }
                );
            }
            return Ok(response);
        }
        bail!("github download exceeded 5 redirects")
    }
    pub fn registry(&self, parts: &[&str], query: Option<(&str, &str)>) -> Result<Value> {
        let mut url = Self::url(&self.api, parts)?;
        if let Some((k, v)) = query {
            url.query_pairs_mut().append_pair(k, v);
        }
        Ok(serde_json::from_slice(&response_bytes(
            self.request(url, false, None, "", "application/json")?,
            4 << 20,
        )?)?)
    }
    pub fn github_json(&self, parts: &[&str]) -> Result<Value> {
        Ok(serde_json::from_slice(&response_bytes(
            self.request(
                Self::url(&self.github, parts)?,
                true,
                None,
                "",
                "application/vnd.github+json",
            )?,
            4 << 20,
        )?)?)
    }
    pub fn github_download(&self, parts: &[&str]) -> Result<Response> {
        self.request(
            Self::url(&self.github, parts)?,
            true,
            None,
            "",
            "application/octet-stream",
        )
    }
    pub fn publish(
        &self,
        key: &str,
        owner: &str,
        addon: &str,
        release: &lockfile::Release,
        size: u64,
    ) -> Result<()> {
        ensure!(!key.trim().is_empty(), "missing GDAM_API_KEY");
        let r = release;
        let payload = json!({"owner":owner,"addon":addon,"tag_name":r.tag,"asset_name":r.asset_name,"github_release_id":r.github_release_id,"commit_sha":r.commit_sha,"asset_id":r.asset_id,"asset_digest":r.asset_digest,"asset_size":size,"published_at":r.published_at,"prerelease":r.prerelease});
        response_bytes(
            self.request(
                Self::url(&self.api, &["api", "v1", "publish"])?,
                false,
                Some(&payload),
                key.trim(),
                "application/json",
            )?,
            4 << 20,
        )?;
        Ok(())
    }
    pub fn resolve(&self, name: &str, tag: &str) -> Result<Release> {
        let p = PackageSpec::parse(name)?;
        ensure!(
            p.tag.is_empty(),
            "invalid addon key {name}: tags belong in the tag field"
        );
        let mut v = self.registry(
            &["api", "v1", "resolve", &p.owner, &p.repo],
            (!tag.trim().is_empty()).then_some(("tag", tag.trim())),
        )?;
        let list = self.registry(
            &[
                "api", "v1", "owners", &p.owner, "addons", &p.repo, "releases",
            ],
            None,
        )?;
        let rows = list
            .as_array()
            .ok_or_else(|| anyhow::anyhow!("registry releases must be an array"))?;
        let r = rows
            .iter()
            .find(|r| {
                r["github_release_id"].as_i64() == v["github_release_id"].as_i64()
                    && r["github_release_id"].as_i64().is_some_and(|n| n > 0)
            })
            .ok_or_else(|| {
                anyhow::anyhow!("resolved release is missing from registry release list")
            })?;
        for k in [
            "tag_name",
            "commit_sha",
            "asset_id",
            "asset_name",
            "asset_digest",
        ] {
            ensure!(r[k] == v[k], "registry release identity drift detected");
        }
        v["published_at"] = r["published_at"].clone();
        v["prerelease"] = r["prerelease"].clone();
        let globals = v
            .get("global_classes")
            .filter(|v| !v.is_null())
            .map(|v| serde_json::from_value(v.clone()))
            .transpose()?
            .unwrap_or_default();
        let mut map = v.as_object().unwrap().clone();
        let tag = map.remove("tag_name").unwrap_or(Value::Null);
        map.insert("tag".into(), tag);
        map.retain(|k, _| {
            [
                "name",
                "tag",
                "github_owner",
                "github_repo",
                "github_release_id",
                "commit_sha",
                "asset_id",
                "asset_name",
                "asset_digest",
                "published_at",
                "prerelease",
                "editor_plugin",
                "dependencies",
            ]
            .contains(&k.as_str())
        });
        let identity: lockfile::Release = serde_json::from_value(Value::Object(map))?;
        ensure!(
            !identity.name.is_empty()
                && !identity.github_owner.is_empty()
                && !identity.github_repo.is_empty()
                && !identity.tag.is_empty()
                && !identity.commit_sha.is_empty()
                && !identity.asset_name.is_empty()
                && !identity.asset_digest.is_empty()
                && identity.github_release_id > 0
                && identity.asset_id > 0,
            "invalid registry response: missing verified release identity fields"
        );
        crate::github::timestamp(&identity.published_at)?;
        Ok(Release {
            identity,
            global_classes: globals,
            local: false,
        })
    }
    pub fn repository(&self, name: &str) -> Result<(String, String)> {
        let p = PackageSpec::parse(name)?;
        let v = self.registry(&["api", "v1", "owners", &p.owner, "addons", &p.repo], None)?;
        let url = Url::parse(
            v["repo"]
                .as_str()
                .ok_or_else(|| anyhow::anyhow!("registered addon has no repository"))?,
        )?;
        ensure!(
            url.scheme() == "https"
                && url.host_str() == Some("github.com")
                && url.port().is_none()
                && url.username().is_empty()
                && url.password().is_none()
                && url.query().is_none()
                && url.fragment().is_none(),
            "registered addon has an invalid GitHub repository"
        );
        let parts = url.path().trim_matches('/').split('/').collect::<Vec<_>>();
        ensure!(
            parts.len() == 2 && !parts[0].is_empty() && !parts[1].is_empty(),
            "registered addon has an invalid GitHub repository"
        );
        Ok((parts[0].into(), parts[1].trim_end_matches(".git").into()))
    }
}
