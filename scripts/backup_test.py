"""Regression checks for restore isolation and corrupted backup rejection; no running DB needed."""
import contextlib
import io
import json
from pathlib import Path
import tarfile
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

import backup


class RestoreTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)
        self.backup = self.root / 'snapshot'
        self.backup.mkdir()
        self.sql = self.backup / 'database.sql'
        self.sql.write_text('CREATE TABLE `example` (`id` int);\nINSERT INTO `example` VALUES (1);\n')
        self.asset = self.root / 'asset.txt'
        self.asset.write_text('a real asset\n')
        with tarfile.open(self.backup / 'uploads.tar.gz', 'w:gz') as tar:
            tar.add(self.asset, arcname='video/asset.txt')
        self.manifest = {
            'format': 1, 'source_database': 'GCFeed', 'snapshot_started_at': '2026-01-01T00:00:00Z',
            'snapshot_finished_at': '2026-01-01T00:00:01Z', 'table_rows': {'example': 1},
            'files': {'video/asset.txt': {'size': self.asset.stat().st_size, 'sha256': backup.digest(self.asset)}},
        }
        self.write_manifest()
        self.patch_directory = patch.object(backup, 'BACKUPS', self.root)
        self.patch_directory.start()
        self.addCleanup(self.patch_directory.stop)

    def write_manifest(self):
        self.manifest['artifacts'] = {name: backup.digest(self.backup / name)
                                      for name in ('database.sql', 'uploads.tar.gz')}
        backup.private_json(self.backup / 'manifest.json', self.manifest)

    def run_drill(self, query):
        with patch.object(backup, 'query', side_effect=query) as call, \
                patch.object(backup.subprocess, 'run', return_value=SimpleNamespace(returncode=0)), \
                contextlib.redirect_stdout(io.StringIO()):
            result = backup.drill(SimpleNamespace(backup=self.backup))
        return result, call

    def test_restore_isolated_and_removed(self):
        result, calls = self.run_drill(lambda sql, database=None: '1' if sql.startswith('SELECT') else '')
        self.assertEqual(result, 0)
        report = json.loads((self.backup / 'restore-report.json').read_text())
        name = report['temporary_database']
        self.assertRegex(name, r'^gcfeed_restore_test_[a-f0-9]{32}$')
        self.assertTrue(report['temporary_resources_removed'])
        self.assertTrue(report['asset_hashes_match'])
        self.assertEqual(calls.call_args_list[-1].args[0], f'DROP DATABASE `{name}`')
        self.assertFalse(list(self.root.glob('restore-files-*')))

    def test_corrupt_sql_rejected_before_database_creation(self):
        self.sql.write_text(self.sql.read_text() + 'broken')
        with patch.object(backup, 'query') as query:
            with self.assertRaisesRegex(ValueError, 'Checksum mismatch'):
                backup.drill(SimpleNamespace(backup=self.backup))
            query.assert_not_called()

    def test_database_switch_rejected_before_database_creation(self):
        self.sql.write_text('USE GCFeed;\n' + self.sql.read_text())
        self.write_manifest()
        with patch.object(backup, 'query') as query:
            with self.assertRaisesRegex(ValueError, 'database switching'):
                backup.drill(SimpleNamespace(backup=self.backup))
            query.assert_not_called()

    def test_row_mismatch_fails_and_cleans_up(self):
        result, _ = self.run_drill(lambda sql, database=None: '0' if sql.startswith('SELECT') else '')
        self.assertEqual(result, 1)
        report = json.loads((self.backup / 'restore-report.json').read_text())
        self.assertFalse(report['table_rows_match'])
        self.assertTrue(report['temporary_resources_removed'])

    def test_unsafe_archive_cleans_up_without_writing_outside_temp(self):
        with tarfile.open(self.backup / 'uploads.tar.gz', 'w:gz') as tar:
            tar.add(self.asset, arcname='../escaped.txt')
        self.write_manifest()
        with self.assertRaisesRegex(ValueError, 'Unexpected archive'):
            self.run_drill(lambda sql, database=None: '1' if sql.startswith('SELECT') else '')
        report = json.loads((self.backup / 'restore-report.json').read_text())
        self.assertTrue(report['temporary_resources_removed'])
        self.assertFalse(report['passed'])
        self.assertFalse((self.root / 'escaped.txt').exists())


if __name__ == '__main__':
    unittest.main()
