"""Fetch the immutable former Go CLI solely for native compatibility tests."""
import hashlib
import json
import os
from pathlib import Path
import platform
import subprocess
import sys
import tarfile
import zipfile

ROOT = Path(__file__).resolve().parents[1]
record = json.loads((ROOT/'internal/testdata/legacy-release.json').read_text())
label = {'Linux':'Linux','Darwin':'Darwin','Windows':'Windows'}[platform.system()]
arch = {'x86_64':'x86_64','AMD64':'x86_64','aarch64':'arm64','arm64':'arm64','ARM64':'arm64'}[platform.machine()]
name = f'gdam_{label}_{arch}.'+('zip' if label == 'Windows' else 'tar.gz')
folder = ROOT/'.artifacts/legacy'/f'{label}-{arch}'
folder.mkdir(parents=True, exist_ok=True)
subprocess.run(['gh','release','download',record['tag'],'--repo','aviorstudio/gdam',
                '--pattern',name,'--pattern','checksums.txt','--dir',str(folder),'--clobber'],check=True,timeout=90)
raw = (folder/name).read_bytes()
assert hashlib.sha256(raw).hexdigest() == record['archives'][name], 'legacy archive digest changed'
checks = (folder/'checksums.txt').read_bytes()
assert hashlib.sha256(checks).hexdigest() == record['checksums_sha256'], 'legacy checksum manifest changed'
assert f"{record['archives'][name]}  {name}" in checks.decode()
executable = 'gdam.exe' if label == 'Windows' else 'gdam'
if label == 'Windows':
    with zipfile.ZipFile(folder/name) as archive:
        assert sorted(archive.namelist()) == sorted([executable,'README.md'])
        binary = archive.read(executable)
else:
    with tarfile.open(folder/name,'r:gz') as archive:
        members = archive.getmembers()
        assert sorted(m.name for m in members) == sorted([executable,'README.md'])
        assert all(m.isfile() for m in members)
        binary = archive.extractfile(executable).read()
assert len(binary) < 64 << 20
output = folder/executable
output.write_bytes(binary)
output.chmod(0o755)
env = {k:v for k,v in os.environ.items() if k not in ('GDAM_API_KEY','GITHUB_TOKEN')}
version = subprocess.check_output([str(output),'--version'],text=True,env=env,timeout=15)
assert version.splitlines()[0] in ('gdam 0.1.5','gdam v0.1.5'), version
assert f"commit: {record['source_commit']}" in version, 'legacy executable source differs'
print(output)
