"""Exercise the native CLI in disposable projects, including legacy recovery."""
import hashlib
import io
import zipfile
import http.server
import json
import os
from pathlib import Path
import subprocess
import tempfile
import threading
import unittest

BINARY = Path(os.environ.get('GDAM_EXECUTABLE', str(Path(os.environ.get('CARGO_TARGET_DIR', 'target'))/'debug/gdam'))).resolve()
REFERENCE = os.environ.get('GDAM_REFERENCE_EXECUTABLE')


def environment():
    env = {k: v for k, v in os.environ.items() if k not in ('GDAM_API_KEY', 'GITHUB_TOKEN')}
    env['GDAM_API_URL'] = 'http://127.0.0.1:9'
    return env


def invoke(binary, root, *args, env=None):
    return subprocess.run([str(binary), *args], cwd=root, env=env or environment(),
                          capture_output=True, text=True, timeout=20)


def inventory(root):
    return {str(p.relative_to(root)): ('link', str(p.readlink())) if p.is_symlink()
            else ('file', p.read_bytes()) for p in root.rglob('*')
            if '.gdam-state' not in p.relative_to(root).parts and (p.is_symlink() or p.is_file())}


class NativeCLI(unittest.TestCase):
    def setUp(self):
        temp = tempfile.TemporaryDirectory(prefix='gdam native journeys ')
        self.addCleanup(temp.cleanup)
        self.root = Path(temp.name)
        self.project = self.root/'project'
        self.project.mkdir()
        (self.project/'project.godot').write_text('config_version=5\r\n\r\n[application]\r\nconfig/name="Fixture"\r\n')
        self.source = self.root/'linked source 回転'
        self.source.mkdir()
        (self.source/'plugin.cfg').write_text('[plugin]\nname="Fixture"\n')
        (self.source/'keep.gd').write_text('extends RefCounted\n')

    def success(self, *args, root=None, binary=BINARY):
        result = invoke(binary, root or self.project, *args)
        self.assertEqual(result.returncode, 0, result.stderr)
        return result

    def test_init_finds_parent_project_and_preserves_existing_manifest(self):
        sub = self.project/'deep'/'inside'
        sub.mkdir(parents=True)
        self.success('init', root=sub)
        self.assertEqual(json.loads((self.project/'gdam.json').read_bytes()), {'addons': {}})
        before = (self.project/'gdam.json').read_bytes()
        self.success('init', root=sub)
        self.assertEqual((self.project/'gdam.json').read_bytes(), before)

    def test_frozen_and_offline_refuse_missing_lock_without_metadata_changes(self):
        self.success('init')
        before = inventory(self.project)
        for args in [('install', '--frozen-lockfile'), ('install', '--offline')]:
            result = invoke(BINARY, self.project, *args)
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertEqual(inventory(self.project), before)
        self.success('install')
        locked = inventory(self.project)
        self.success('install', '--offline', '--cache-dir', str(self.root/'cache'))
        self.assertEqual(inventory(self.project), locked)

    def test_link_unlink_and_stored_relink_leave_external_source_untouched(self):
        self.success('init')
        before = inventory(self.source)
        self.success('link', '@test/addon', str(self.source))
        destination = self.project/'addons/@test_addon'
        self.assertTrue(destination.is_symlink())
        self.assertEqual(destination.resolve(), self.source.resolve())
        self.assertNotIn('link', json.loads((self.project/'gdam.json').read_text())['addons']['@test/addon'])
        self.success('unlink', '@test/addon')
        self.assertFalse(destination.exists())
        self.success('link', '@test/addon')
        self.assertEqual(destination.resolve(), self.source.resolve())
        self.success('unlink', '--all')
        self.assertEqual(inventory(self.source), before)
        self.assertFalse(json.loads((self.project/'gdam.link.json').read_text())['addons']['@test/addon']['enabled'])

    def test_install_retains_local_link_when_replacing_a_registered_copy(self):
        self.success('init')
        name = '@test/addon'
        (self.project/'gdam.json').write_text(json.dumps({'addons': {name: {'tag': 'v1'}}}))
        release = {'name': name, 'tag': 'v1', 'github_owner': 'test', 'github_repo': 'addon',
                   'github_release_id': 1, 'commit_sha': 'a'*40, 'asset_id': 1,
                   'asset_name': 'fixture.zip', 'asset_digest': 'sha256:'+'0'*64,
                   'published_at': '2026-10-01T00:00:00Z', 'prerelease': False,
                   'editor_plugin': False, 'installs': [{'path': 'addons/@test_addon', 'required_by': ['gdam.json']}]}
        (self.project/'gdam.lock').write_text(json.dumps({'version': 1, 'requirements': {name: 'v1'}, 'addons': {name+'@v1': release}}))
        self.success('link', name, str(self.source))
        before = inventory(self.source)
        self.success('install')
        self.assertEqual((self.project/'addons/@test_addon').resolve(), self.source.resolve())
        self.assertEqual(inventory(self.source), before)

    def test_invalid_specs_and_colliding_paths_restore_project(self):
        self.success('init')
        before = inventory(self.project)
        for spec in ('@missing', '@test/repo@tag', '@test/../../escape'):
            result = invoke(BINARY, self.project, 'link', spec, str(self.source))
            self.assertEqual(result.returncode, 2, result.stderr)
            self.assertEqual(inventory(self.project), before)
        self.success('link', '@test/a_b', str(self.source))
        before = inventory(self.project)
        result = invoke(BINARY, self.project, 'link', '@test_a/b', str(self.source))
        self.assertEqual(result.returncode, 2, result.stderr)
        self.assertEqual(inventory(self.project), before)

    def test_frozen_archive_install_preserves_lock_and_rolls_back_missing_plugin(self):
        self.success('init')
        name = '@test/addon'
        (self.project/'gdam.json').write_text(json.dumps({'addons': {name: {'tag': 'v1'}}}))
        cache = self.root/'cache'
        cache.mkdir()
        for valid in [True, False]:
            output = io.BytesIO()
            with zipfile.ZipFile(output, 'w', zipfile.ZIP_DEFLATED) as archive:
                archive.writestr('src/example.gd', 'extends RefCounted\n')
                if valid:
                    archive.writestr('plugin.cfg', '[plugin]\nname="Fixture"\n')
            raw = output.getvalue()
            digest = hashlib.sha256(raw).hexdigest()
            (cache/(digest+'.zip')).write_bytes(raw)
            release = {'name': name, 'tag': 'v1', 'github_owner': 'test', 'github_repo': 'addon',
                       'github_release_id': 1, 'commit_sha': 'a'*40, 'asset_id': 1,
                       'asset_name': 'fixture.zip', 'asset_digest': 'sha256:'+digest,
                       'published_at': '2026-10-01T00:00:00Z', 'prerelease': False,
                       'editor_plugin': True, 'installs': [{'path': 'addons/@test_addon', 'required_by': ['gdam.json']}]}
            locked = json.dumps({'version': 1, 'requirements': {name: 'v1'}, 'addons': {name+'@v1': release}}, indent=4).encode()
            (self.project/'gdam.lock').write_bytes(locked)
            before = inventory(self.project)
            result = invoke(BINARY, self.project, 'install', '--offline', '--cache-dir', str(cache))
            if valid:
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertTrue((self.project/'addons/@test_addon/plugin.cfg').is_file())
                self.assertIn('res://addons/@test_addon/plugin.cfg', (self.project/'project.godot').read_text())
            else:
                self.assertEqual(result.returncode, 2, result.stderr)
                self.assertEqual(inventory(self.project), before)
            self.assertEqual((self.project/'gdam.lock').read_bytes(), locked)

    def test_symlinked_addons_root_is_refused_without_touching_external_directory(self):
        self.success('init')
        external = self.root/'external'
        external.mkdir()
        (external/'sentinel').write_bytes(b'preserve me')
        (self.project/'addons').symlink_to(external, target_is_directory=True)
        before = inventory(external)
        result = invoke(BINARY, self.project, 'link', '@test/addon', str(self.source))
        self.assertEqual(result.returncode, 1, result.stderr)
        self.assertEqual(inventory(external), before)
        self.assertEqual((self.project/'addons').resolve(), external.resolve())

    @unittest.skipUnless(REFERENCE, 'legacy executable supplied for independent compatibility acceptance')
    def test_metadata_and_links_match_original_main(self):
        before = inventory(self.source)
        outputs = []
        for binary in [Path(REFERENCE), BINARY]:
            root = self.root/('legacy' if binary != BINARY else 'rust')
            root.mkdir()
            (root/'project.godot').write_bytes((self.project/'project.godot').read_bytes())
            for args in [('init',), ('install',), ('link', '@test/addon', str(self.source)),
                         ('unlink', '@test/addon'), ('link', '@test/addon'), ('unlink', '--all')]:
                self.success(*args, root=root, binary=binary)
            outputs.append(inventory(root))
        self.assertEqual(outputs[0], outputs[1])
        self.assertEqual(inventory(self.source), before)

    @unittest.skipUnless(REFERENCE, 'legacy executable supplied for independent recovery acceptance')
    def test_recovers_original_go_prepared_journal_before_reading_manifest(self):
        self.success('init', binary=Path(REFERENCE))
        self.success('link', '@test/addon', str(self.source), binary=Path(REFERENCE))
        before = inventory(self.project)
        entered, release = threading.Event(), threading.Event()
        class Handler(http.server.BaseHTTPRequestHandler):
            def do_GET(self):
                entered.set()
                release.wait(15)
                try:
                    self.send_response(503)
                    self.end_headers()
                except (BrokenPipeError, ConnectionResetError):
                    pass
            def log_message(self, *_):
                pass
        server = http.server.ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        server.daemon_threads = True
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        process = None
        try:
            env = environment()
            env['GDAM_API_URL'] = f'http://127.0.0.1:{server.server_port}'
            process = subprocess.Popen([REFERENCE, 'add', '@test/new'], cwd=self.project,
                                       env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE)
            self.assertTrue(entered.wait(10), 'original CLI never reached its prepared transaction')
            competing = invoke(BINARY, self.project, 'unlink', '--all')
            self.assertEqual(competing.returncode, 1, competing.stderr)
            self.assertIn('another gdam operation', competing.stderr)
            self.assertEqual(inventory(self.project), before)
            process.kill()
            process.communicate(timeout=5)
            journal = self.project/'.gdam-state/transaction/journal.json'
            self.assertEqual(json.loads(journal.read_text())['state'], 'prepared')
            (self.project/'gdam.json').write_text('interrupted metadata is invalid')
            (self.project/'addons/@test_addon').unlink()
            self.success('unlink', '--all')
            self.assertFalse(journal.exists())
            self.assertEqual(json.loads((self.project/'gdam.json').read_text()),
                             json.loads(before['gdam.json'][1]))
            self.assertFalse(json.loads((self.project/'gdam.link.json').read_text())['addons']['@test/addon']['enabled'])
            self.assertEqual((self.source/'keep.gd').read_text(), 'extends RefCounted\n')
        finally:
            release.set()
            if process is not None and process.poll() is None:
                process.kill()
                process.communicate(timeout=5)
            server.shutdown()
            server.server_close()
            thread.join(timeout=2)


if __name__ == '__main__':
    unittest.main()
