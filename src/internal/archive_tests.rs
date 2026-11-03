use crate::archive;
use std::{fs, io::Write, path::Path};
fn package(path: &Path, entries: &[(&str, &[u8])]) {
    let mut zip = zip::ZipWriter::new(fs::File::create(path).unwrap());
    for (name, data) in entries {
        zip.start_file(
            *name,
            zip::write::SimpleFileOptions::default()
                .compression_method(zip::CompressionMethod::Deflated)
                .unix_permissions(0o644),
        )
        .unwrap();
        zip.write_all(data).unwrap();
    }
    zip.finish().unwrap();
}
#[test]
fn flat_and_single_directory_packages_preserve_the_existing_release_layouts() {
    let t = tempfile::tempdir().unwrap();
    let p = t.path().join("archive.zip");
    package(
        &p,
        &[
            ("plugin.cfg", b"fixture"),
            ("script.gd", b"extends RefCounted"),
        ],
    );
    assert_eq!(
        archive::extract(&p, &t.path().join("flat"), false).unwrap(),
        t.path().join("flat")
    );
    assert!(archive::extract(&p, &t.path().join("single-refused"), true).is_err());
    package(
        &p,
        &[
            ("addon/plugin.cfg", b"fixture"),
            ("addon/script.gd", b"extends RefCounted"),
        ],
    );
    let root = archive::extract(&p, &t.path().join("single"), true).unwrap();
    assert_eq!(root, t.path().join("single/addon"));
    assert_eq!(
        fs::read(root.join("script.gd")).unwrap(),
        b"extends RefCounted"
    );
}
#[test]
fn absolute_parent_and_backslash_paths_are_refused_without_writing_outside_the_root() {
    for name in [
        "../escape",
        "/absolute",
        "folder/../../escape",
        "folder\\escape",
    ] {
        let t = tempfile::tempdir().unwrap();
        let p = t.path().join("bad.zip");
        package(&p, &[(name, b"bad")]);
        assert!(
            archive::extract(&p, &t.path().join("output"), false).is_err(),
            "{name}"
        );
        assert!(!t.path().join("escape").exists());
    }
}
#[test]
fn symlink_entries_and_duplicate_files_are_refused() {
    let t = tempfile::tempdir().unwrap();
    let p = t.path().join("bad.zip");
    let mut zip = zip::ZipWriter::new(fs::File::create(&p).unwrap());
    zip.add_symlink(
        "link",
        "../../escape",
        zip::write::SimpleFileOptions::default(),
    )
    .unwrap();
    zip.finish().unwrap();
    assert!(archive::extract(&p, &t.path().join("link"), false).is_err());
    // Duplicate destinations can also arise from distinct normalized names.
    package(
        &p,
        &[("root/file", b"first"), ("root/./file", b"replacement")],
    );
    assert!(archive::extract(&p, &t.path().join("duplicate"), false).is_err());
}
