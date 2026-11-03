use crate::*;
use serde_json::{Value, json};
use std::collections::BTreeMap;
#[test]
fn latest_go_main_independently_freezes_specs_manifests_uids_paths_placement_and_project_edits() {
    let fixture: Value = serde_json::from_str(include_str!("testdata/go-contracts.json")).unwrap();
    assert_eq!(
        fixture["source_commit"],
        "14e92be1cd9588aef152d3995ab0c40136a18e40"
    );
    let cases = fixture["cases"].as_array().unwrap();
    assert!(cases.len() >= 80);
    for (row, case) in cases.iter().enumerate() {
        let i = &case["input"];
        let actual: Result<Value> = (|| {
            Ok(match case["kind"].as_str().unwrap() {
                "spec" => {
                    let s = spec::PackageSpec::parse(i.as_str().unwrap())?;
                    json!({"owner":s.owner,"repo":s.repo,"tag":s.tag})
                }
                "manifest" => serde_json::to_value(manifest::Manifest::parse(
                    i.as_str().unwrap().as_bytes(),
                )?)?,
                "uid" => json!(uid::text(i.as_i64().unwrap())),
                "install_path" => json!(lockfile::valid_install_path(
                    i.as_str().unwrap(),
                    "@test/addon"
                )),
                "editor_plugin" => json!(
                    project::editor_plugins(
                        i["text"].as_str().unwrap(),
                        i["plugin"].as_str().unwrap(),
                        i["enabled"].as_bool().unwrap()
                    )?
                    .unwrap_or_else(|| i["text"].as_str().unwrap().into())
                ),
                "resolve" => {
                    let pins: BTreeMap<String, String> = serde_json::from_value(i["pins"].clone())?;
                    let plan = resolve::resolve(&pins, |name, tag| {
                        let r = &i["releases"][lockfile::key(name, tag)];
                        anyhow::ensure!(!r.is_null(), "missing release");
                        let deps = if r["dependencies"].is_null() {
                            BTreeMap::new()
                        } else {
                            serde_json::from_value(r["dependencies"].clone())?
                        };
                        Ok(resolve::Release {
                            identity: lockfile::Release {
                                name: r["name"].as_str().unwrap().into(),
                                tag: r["tag"].as_str().unwrap().into(),
                                published_at: r["published_at"].as_str().unwrap().into(),
                                dependencies: deps,
                                ..lockfile::Release::default()
                            },
                            local: r["local"].as_bool().unwrap(),
                            ..resolve::Release::default()
                        })
                    })?;
                    let copies=plan.copies.into_iter().map(|c|json!({"name":c.release.identity.name,"tag":c.release.identity.tag,"local":c.release.local,"path":c.path,"nested":c.nested,"required_by":c.required_by,"dependencies":c.dependencies})).collect::<Vec<_>>();
                    json!({"hoisted":plan.hoisted,"copies":copies})
                }
                other => panic!("unknown original contract {other}"),
            })
        })();
        assert_eq!(
            actual.is_err(),
            case["error"].as_bool().unwrap(),
            "original case {row}: {case}; actual {actual:?}"
        );
        if let Ok(actual) = actual {
            assert_eq!(actual, case["expected"], "original case {row}: {case}");
        }
    }
}
