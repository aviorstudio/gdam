use crate::{Result, github, lockfile::Release, network::Network};
use serde_json::{Value, json};
use sha2::{Digest, Sha256};
use std::{
    collections::BTreeMap,
    fs,
    io::{Read, Write},
    net::{TcpListener, TcpStream},
    sync::{
        Arc, Mutex,
        atomic::{AtomicBool, Ordering},
    },
    thread,
    time::Duration,
};
#[derive(Clone, Debug)]
struct Request {
    target: String,
    headers: BTreeMap<String, String>,
    body: Vec<u8>,
}
struct Reply {
    status: u16,
    headers: Vec<(String, String)>,
    body: Vec<u8>,
}
fn body(value: Value) -> Reply {
    Reply {
        status: 200,
        headers: vec![("Content-Type".into(), "application/json".into())],
        body: serde_json::to_vec(&value).unwrap(),
    }
}
struct Server {
    url: String,
    seen: Arc<Mutex<Vec<Request>>>,
    done: Arc<AtomicBool>,
    thread: Option<thread::JoinHandle<()>>,
}
impl Server {
    fn new(handler: impl Fn(&Request) -> Reply + Send + 'static) -> Self {
        let listener = TcpListener::bind("127.0.0.1:0").unwrap();
        let url = format!("http://{}", listener.local_addr().unwrap());
        listener.set_nonblocking(true).unwrap();
        let seen = Arc::new(Mutex::new(Vec::new()));
        let done = Arc::new(AtomicBool::new(false));
        let captured = seen.clone();
        let stop = done.clone();
        let thread = thread::spawn(move || {
            while !stop.load(Ordering::SeqCst) {
                match listener.accept() {
                    Ok((mut stream, _)) => {
                        stream.set_nonblocking(false).unwrap();
                        stream
                            .set_read_timeout(Some(Duration::from_secs(3)))
                            .unwrap();
                        let request = read_request(&mut stream);
                        if let Ok(request) = request {
                            let response = handler(&request);
                            captured.lock().unwrap().push(request);
                            let mut headers = format!(
                                "HTTP/1.1 {} Fixture\r\nConnection: close\r\n",
                                response.status
                            );
                            if !response
                                .headers
                                .iter()
                                .any(|(k, _)| k.eq_ignore_ascii_case("content-length"))
                            {
                                headers += &format!("Content-Length: {}\r\n", response.body.len());
                            }
                            for (k, v) in response.headers {
                                headers += &format!("{k}: {v}\r\n");
                            }
                            headers += "\r\n";
                            let _ = stream.write_all(headers.as_bytes());
                            let _ = stream.write_all(&response.body);
                        }
                    }
                    Err(e) if e.kind() == std::io::ErrorKind::WouldBlock => {
                        thread::sleep(Duration::from_millis(5))
                    }
                    Err(_) => break,
                }
            }
        });
        Self {
            url,
            seen,
            done,
            thread: Some(thread),
        }
    }
    fn client(&self) -> Network {
        Network::with_bases(
            &self.url,
            &self.url,
            "fixture-github-token".into(),
            Duration::from_secs(10),
        )
        .unwrap()
    }
}
impl Drop for Server {
    fn drop(&mut self) {
        self.done.store(true, Ordering::SeqCst);
        self.thread.take().unwrap().join().unwrap();
    }
}
fn read_request(stream: &mut TcpStream) -> Result<Request> {
    let mut raw = Vec::new();
    let mut byte = [0];
    while !raw.ends_with(b"\r\n\r\n") {
        anyhow::ensure!(raw.len() < 32768, "fixture request headers exceed bounds");
        stream.read_exact(&mut byte)?;
        raw.push(byte[0]);
    }
    let text = String::from_utf8(raw)?;
    let mut lines = text.split("\r\n");
    let line = lines.next().unwrap();
    let target = line.split_whitespace().nth(1).unwrap().to_owned();
    let headers = lines
        .filter_map(|l| l.split_once(':'))
        .map(|(k, v)| (k.to_ascii_lowercase(), v.trim().to_owned()))
        .collect::<BTreeMap<_, _>>();
    let size = headers
        .get("content-length")
        .map(|s| s.parse())
        .transpose()?
        .unwrap_or(0);
    anyhow::ensure!(size <= 4 << 20, "fixture request body exceeds bounds");
    let mut body = vec![0; size];
    stream.read_exact(&mut body)?;
    Ok(Request {
        target,
        headers,
        body,
    })
}
fn identity(bytes: &[u8]) -> Release {
    Release {
        name: "@dev/addon".into(),
        tag: "Release/v1".into(),
        github_owner: "dev".into(),
        github_repo: "addon".into(),
        github_release_id: 11,
        commit_sha: "a".repeat(40),
        asset_id: 22,
        asset_name: "addon.zip".into(),
        asset_digest: format!("sha256:{:x}", Sha256::digest(bytes)),
        published_at: "2026-10-01T00:00:00Z".into(),
        ..Release::default()
    }
}
fn provider(
    bytes: Vec<u8>,
    r: Release,
    alter: impl Fn(&Request, Reply) -> Reply + Send + 'static,
) -> Server {
    Server::new(move |request| {
        let metadata = request
            .headers
            .get("accept")
            .is_some_and(|v| v == "application/vnd.github+json");
        let reply = if request.target.ends_with("/releases/11") {
            body(
                json!({"id":r.github_release_id,"tag_name":r.tag,"published_at":r.published_at,"prerelease":r.prerelease,"draft":false}),
            )
        } else if request.target.contains("/commits/") {
            body(json!({"sha":r.commit_sha}))
        } else if request.target.ends_with("/releases/assets/22") && metadata {
            body(
                json!({"id":r.asset_id,"name":r.asset_name,"digest":r.asset_digest,"state":"uploaded"}),
            )
        } else if request.target.ends_with("/releases/assets/22") {
            Reply {
                status: 200,
                headers: vec![],
                body: bytes.clone(),
            }
        } else {
            Reply {
                status: 404,
                headers: vec![],
                body: b"unknown fixture path".to_vec(),
            }
        };
        alter(request, reply)
    })
}
#[test]
fn verified_download_rehashes_cache_and_offline_never_contacts_provider() {
    let bytes = b"independent verified archive".to_vec();
    let r = identity(&bytes);
    let s = provider(bytes.clone(), r.clone(), |_, r| r);
    let client = s.client();
    let dir = tempfile::tempdir().unwrap();
    let cache = dir.path().join("cache");
    let dest = dir.path().join("archive.zip");
    client.download(&r, &dest, &cache, false).unwrap();
    assert_eq!(fs::read(&dest).unwrap(), bytes);
    let count = s.seen.lock().unwrap().len();
    assert_eq!(count, 4);
    client.download(&r, &dest, &cache, true).unwrap();
    assert_eq!(s.seen.lock().unwrap().len(), count);
    let entry = cache.join(format!(
        "{}.zip",
        github::digest_hex(&r.asset_digest).unwrap()
    ));
    fs::write(entry, b"corruption").unwrap();
    assert!(client.download(&r, &dest, &cache, true).is_err());
    assert_eq!(s.seen.lock().unwrap().len(), count);
}
#[test]
fn each_release_coordinate_is_verified_before_using_cached_or_remote_bytes() {
    for field in [
        "tag",
        "commit",
        "asset",
        "digest",
        "published",
        "prerelease",
    ] {
        let bytes = b"archive".to_vec();
        let original = identity(&bytes);
        let s = provider(bytes, original.clone(), |_, r| r);
        let mut changed = original;
        match field {
            "tag" => changed.tag = "moved".into(),
            "commit" => changed.commit_sha = "b".repeat(40),
            "asset" => changed.asset_name = "replacement.zip".into(),
            "digest" => changed.asset_digest = format!("sha256:{}", "0".repeat(64)),
            "published" => changed.published_at = "2026-10-02T00:00:00Z".into(),
            "prerelease" => changed.prerelease = true,
            _ => unreachable!(),
        };
        let temp = tempfile::tempdir().unwrap();
        let dest = temp.path().join("out");
        assert!(
            s.client()
                .download(&changed, &dest, std::path::Path::new(""), false)
                .is_err(),
            "changed {field}"
        );
        assert!(!dest.exists());
    }
}
#[test]
fn truncated_oversized_and_corrupted_downloads_never_publish_a_partial_archive() {
    for kind in ["truncated", "oversized", "corrupt"] {
        let bytes = b"archive".to_vec();
        let r = identity(&bytes);
        let s = provider(bytes, r.clone(), move |request, mut reply| {
            if request
                .headers
                .get("accept")
                .is_some_and(|s| s == "application/octet-stream")
            {
                match kind {
                    "truncated" => reply.headers.push(("Content-Length".into(), "99".into())),
                    "oversized" => reply.headers.push((
                        "Content-Length".into(),
                        (github::MAX_DOWNLOAD + 1).to_string(),
                    )),
                    "corrupt" => reply.body = b"corruption".to_vec(),
                    _ => unreachable!(),
                }
            }
            reply
        });
        let temp = tempfile::tempdir().unwrap();
        let dest = temp.path().join("out");
        assert!(
            s.client()
                .download(&r, &dest, std::path::Path::new(""), false)
                .is_err(),
            "{kind}"
        );
        assert!(!dest.exists());
    }
}
#[test]
fn github_auth_is_removed_on_an_origin_change_even_on_the_same_host() {
    let received = Server::new(|_| body(json!({"sha":"a".repeat(40)})));
    let target = received.url.clone();
    let first = Server::new(move |_| Reply {
        status: 302,
        headers: vec![("Location".into(), format!("{target}/redirected"))],
        body: Vec::new(),
    });
    first.client().github_json(&["anything"]).unwrap();
    assert_eq!(
        first.seen.lock().unwrap()[0].headers["authorization"],
        "Bearer fixture-github-token"
    );
    assert!(
        !received.seen.lock().unwrap()[0]
            .headers
            .contains_key("authorization")
    );
}
#[test]
fn registry_exact_tags_are_encoded_and_cross_checked_against_the_release_list() {
    let r = identity(b"a");
    let mut resolved = serde_json::to_value(&r).unwrap();
    let map = resolved.as_object_mut().unwrap();
    map.remove("installs");
    map.remove("tag");
    map.insert("tag_name".into(), json!(r.tag));
    let list = resolved.clone();
    let s = Server::new(move |req| {
        if req.target.contains("/resolve/") {
            body(resolved.clone())
        } else {
            body(json!([list]))
        }
    });
    let actual = s.client().resolve("@DEV/addon", "Release/v1").unwrap();
    assert_eq!(actual.identity.tag, "Release/v1");
    let requests = s.seen.lock().unwrap();
    assert!(requests[0].target.ends_with("?tag=Release%2Fv1"));
    assert!(
        requests
            .iter()
            .all(|r| !r.headers.contains_key("authorization"))
    );
}
#[test]
fn clerk_publication_uses_logical_addon_and_complete_facts_without_github_credentials() {
    let s = Server::new(|_| body(json!({"ok":true})));
    let r = identity(b"archive");
    s.client()
        .publish("ak_fixture", "logical-owner", "logical-addon", &r, 7)
        .unwrap();
    let requests = s.seen.lock().unwrap();
    let req = &requests[0];
    assert_eq!(req.headers["authorization"], "Bearer ak_fixture");
    let body: Value = serde_json::from_slice(&req.body).unwrap();
    assert_eq!(body["owner"], "logical-owner");
    assert_eq!(body["addon"], "logical-addon");
    for (key, want) in [
        ("github_release_id", json!(11)),
        ("asset_id", json!(22)),
        ("asset_size", json!(7)),
        ("tag_name", json!("Release/v1")),
        ("commit_sha", json!("a".repeat(40))),
        ("asset_digest", json!(r.asset_digest)),
        ("published_at", json!(r.published_at)),
        ("prerelease", json!(false)),
    ] {
        assert_eq!(body[key], want, "{key}");
    }
}

fn addon_archive(name: &str, dependencies: Value) -> Vec<u8> {
    use std::io::Cursor;
    let mut zip = zip::ZipWriter::new(Cursor::new(Vec::new()));
    for (name, content) in [
        ("plugin.cfg", format!("[plugin]\nname=\"{name}\"\n")),
        ("gdam.json", json!({"addons":dependencies}).to_string()),
        (
            "src/value.gd",
            "extends RefCounted\nconst UID = \"uid://abc\"\n".into(),
        ),
        ("src/value.gd.uid", "uid://abc\n".into()),
        (
            "scene.tscn",
            "[gd_scene format=3 uid=\"uid://abc\"]\n".into(),
        ),
    ] {
        zip.start_file(name, zip::write::SimpleFileOptions::default())
            .unwrap();
        zip.write_all(content.as_bytes()).unwrap();
    }
    zip.finish().unwrap().into_inner()
}
#[test]
fn complete_install_resolves_nested_dependencies_regenerates_uids_and_reuses_frozen_cache() {
    use crate::{install, manifest::Manifest, transaction::Project};
    let mut releases = Vec::new();
    for (index, (name, tag, dependencies, editor)) in [
        ("@dev/a", "v1", json!({"@dev/shared":{"tag":"v1"}}), true),
        ("@dev/shared", "v1", json!({}), false),
        ("@dev/shared", "v2", json!({}), false),
    ]
    .into_iter()
    .enumerate()
    {
        let archive = addon_archive(name, dependencies.clone());
        let mut r = identity(&archive);
        r.name = name.into();
        r.tag = tag.into();
        r.github_repo = name.split('/').next_back().unwrap().into();
        r.github_release_id = (index + 1) as i64;
        r.asset_id = (index + 101) as i64;
        r.editor_plugin = editor;
        r.dependencies = serde_json::from_value::<BTreeMap<String, Value>>(dependencies)
            .unwrap()
            .into_iter()
            .map(|(k, v)| (k, v["tag"].as_str().unwrap().into()))
            .collect();
        releases.push((r, archive));
    }
    let expected = releases.clone();
    let s = Server::new(move |req| {
        if req.target.contains("/resolve/") {
            let query = req.target.split("?tag=").nth(1).unwrap_or("");
            let repo = req
                .target
                .split('?')
                .next()
                .unwrap()
                .split('/')
                .next_back()
                .unwrap();
            let r = &releases
                .iter()
                .find(|(r, _)| r.github_repo == repo && r.tag == query)
                .unwrap()
                .0;
            let mut value = serde_json::to_value(r).unwrap();
            value["tag_name"] = json!(r.tag);
            return body(value);
        }
        if req.target.ends_with("/releases") {
            let repo = req.target.split('/').rev().nth(1).unwrap();
            return body(json!(
                releases
                    .iter()
                    .filter(|(r, _)| r.github_repo == repo)
                    .map(|(r, _)| {
                        let mut v = serde_json::to_value(r).unwrap();
                        v["tag_name"] = json!(r.tag);
                        v
                    })
                    .collect::<Vec<_>>()
            ));
        }
        if req.target.contains("/commits/") {
            return body(json!({"sha":"a".repeat(40)}));
        }
        for (r, bytes) in &releases {
            if req
                .target
                .ends_with(&format!("/releases/{}", r.github_release_id))
            {
                return body(
                    json!({"id":r.github_release_id,"tag_name":r.tag,"published_at":r.published_at,"prerelease":false,"draft":false}),
                );
            }
            if req
                .target
                .ends_with(&format!("/releases/assets/{}", r.asset_id))
            {
                return if req
                    .headers
                    .get("accept")
                    .is_some_and(|v| v == "application/vnd.github+json")
                {
                    body(
                        json!({"id":r.asset_id,"name":r.asset_name,"digest":r.asset_digest,"state":"uploaded"}),
                    )
                } else {
                    Reply {
                        status: 200,
                        headers: vec![],
                        body: bytes.clone(),
                    }
                };
            }
        }
        Reply {
            status: 404,
            headers: vec![],
            body: b"unknown fixture path".to_vec(),
        }
    });
    let dir = tempfile::tempdir().unwrap();
    let root = dir.path().join("project");
    fs::create_dir(&root).unwrap();
    fs::write(
        root.join("project.godot"),
        "config_version=5\r\n[application]\r\nconfig/name=\"Fixture\"\r\n",
    )
    .unwrap();
    let m = Manifest::parse(br#"{"addons":{"@dev/a":{"tag":"v1"},"@dev/shared":{"tag":"v2"}}}"#)
        .unwrap();
    let project = Project {
        root: cap_std::fs::Dir::open_ambient_dir(&root, cap_std::ambient_authority()).unwrap(),
        dir: root.clone(),
    };
    let cache = dir.path().join("cache");
    let opts = install::Options {
        cache: cache.clone(),
        ..Default::default()
    };
    install::install(&project, &s.client(), &opts, &m).unwrap();
    let nested = root.join("addons/@dev_a/.gdam/@dev_shared");
    assert!(nested.join("plugin.cfg").is_file());
    let uid = fs::read_to_string(nested.join("src/value.gd.uid")).unwrap();
    assert_ne!(uid, "uid://abc\n");
    assert!(
        fs::read_to_string(nested.join("src/value.gd"))
            .unwrap()
            .contains(uid.trim())
    );
    assert!(
        fs::read_to_string(nested.join("scene.tscn"))
            .unwrap()
            .contains(uid.trim())
    );
    assert_eq!(
        fs::read_to_string(root.join("addons/@dev_shared/src/value.gd.uid")).unwrap(),
        "uid://abc\n"
    );
    let deps = fs::read_to_string(root.join("addons/@dev_a/.gdam/deps.gd")).unwrap();
    assert!(deps.contains("res://addons/@dev_a/.gdam/@dev_shared/src/value.gd"));
    assert!(!deps.contains("preload(\"res://addons/@dev_shared/"));
    let godot = fs::read_to_string(root.join("project.godot")).unwrap();
    assert!(godot.contains("res://addons/@dev_a/plugin.cfg"));
    assert!(godot.contains("config/name=\"Fixture\""));
    let locked = fs::read(root.join("gdam.lock")).unwrap();
    let lock = crate::lockfile::Lock::load(&root.join("gdam.lock")).unwrap();
    assert_eq!(lock.paths().len(), 3);
    let calls = s.seen.lock().unwrap().len();
    let frozen = install::Options {
        cache: cache.clone(),
        offline: true,
        frozen: true,
    };
    install::install(&project, &s.client(), &frozen, &m).unwrap();
    assert_eq!(fs::read(root.join("gdam.lock")).unwrap(), locked);
    assert_eq!(s.seen.lock().unwrap().len(), calls);
    // A damaged later archive fails before replacing any installed addon.
    let r = &expected[2].0;
    fs::write(
        cache.join(format!(
            "{}.zip",
            github::digest_hex(&r.asset_digest).unwrap()
        )),
        b"corrupt",
    )
    .unwrap();
    let marker = root.join("addons/@dev_a/keep.txt");
    fs::write(&marker, b"unmodified").unwrap();
    assert!(install::install(&project, &s.client(), &frozen, &m).is_err());
    assert_eq!(fs::read(&marker).unwrap(), b"unmodified");
    assert_eq!(fs::read(root.join("gdam.lock")).unwrap(), locked);
    assert_eq!(s.seen.lock().unwrap().len(), calls);
    // Removing requirements prunes both the old shelf and nested dependency.
    install::install(&project, &s.client(), &opts, &Manifest::default()).unwrap();
    assert!(!root.join("addons/@dev_a").exists());
    assert!(!root.join("addons/@dev_shared").exists());
}
#[test]
fn incomplete_and_drifting_registry_identities_fail_closed() {
    for change in ["missing", "digest", "zero-time", "no-list"] {
        let r = identity(b"a");
        let mut v = serde_json::to_value(r).unwrap();
        v["tag_name"] = json!("Release/v1");
        let mut list = v.clone();
        match change {
            "missing" => v["asset_id"] = json!(0),
            "digest" => list["asset_digest"] = json!("sha256:other"),
            "zero-time" => list["published_at"] = json!("0001-01-01T00:00:00Z"),
            _ => (),
        }
        let s = Server::new(move |req| {
            if req.target.contains("/resolve/") {
                body(v.clone())
            } else if change == "no-list" {
                body(json!([]))
            } else {
                body(json!([list]))
            }
        });
        assert!(
            s.client().resolve("@dev/addon", "Release/v1").is_err(),
            "{change}"
        );
    }
}
