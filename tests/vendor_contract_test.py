"""Verify the exact upstream filesystem source and narrow Windows API patch."""
import hashlib,json
from pathlib import Path
root=Path(__file__).resolve().parents[1]/'vendor/cap-primitives'
record=json.loads((root/'SOURCE.json').read_text())
assert record['crate']=='cap-primitives' and record['version']=='3.4.6'
assert record['crate_sha256']=='8e0bf07d379916947be6c4a07f43684153d710a2896c31f9e97781362895596c'
original=record['original_files_sha256'];patch=record['patched_files_sha256']
assert set(patch)=={'Cargo.toml','src/fs/mod.rs','src/fs/symlink.rs'}
assert {p.relative_to(root).as_posix() for p in root.rglob('*') if p.is_file()}==set(original)|{'SOURCE.json'}
for name,sha in {**original,**patch}.items():
 p=root/name
 assert p.is_file() and not p.is_symlink() and hashlib.sha256(p.read_bytes()).hexdigest()==sha,name
assert all((root/name).is_file() for name in ['LICENSE-MIT','LICENSE-APACHE','LICENSE-Apache-2.0_WITH_LLVM-exception'])
print('Exact upstream filesystem source, licenses and Windows contents API patch verified')
