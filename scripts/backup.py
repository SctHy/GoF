#!/usr/bin/env python3
"""Local MySQL + uploads backup and isolated restore drill (Python standard library)."""
import argparse
from collections import Counter
from datetime import datetime, timezone
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import subprocess
import tarfile
import tempfile
import time
from urllib.parse import unquote, urlsplit
import uuid

ROOT = Path(__file__).resolve().parents[1]
BACKUPS = ROOT / '.local' / 'backups'
IDENTIFIER = re.compile(r'^[A-Za-z0-9_]+$')


def now():
    return datetime.now(timezone.utc).isoformat()


def private_json(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + '\n')
    path.chmod(0o600)


def digest(path):
    with path.open('rb') as file:
        return hashlib.file_digest(file, 'sha256').hexdigest()


def identifier(value):
    if not IDENTIFIER.fullmatch(value):
        raise ValueError('Invalid database/table identifier')
    return value


def container(command, *, data=None, output=None):
    # Password is expanded inside the MySQL container, never in argv or logs.
    result = subprocess.run(
        [str(ROOT / 'scripts/compose.sh'), 'exec', '-T', 'mysql', 'sh', '-c',
         'export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; exec "$@"', 'sh', *command],
        input=data, stdout=output or subprocess.PIPE, stderr=subprocess.PIPE)
    if result.returncode:
        raise RuntimeError(f'MySQL container operation failed (exit {result.returncode}); inspect service health')
    return result.stdout


def query(sql, database=None):
    args = ['mysql', '-uroot', '--batch', '--skip-column-names', '--default-character-set=utf8mb4']
    if database:
        args.append(identifier(database))
    return container(args, data=sql.encode()).decode().strip()


def dump_counts(path):
    counts = Counter()
    with path.open() as file:
        for line in file:
            table = re.match(r'CREATE TABLE `([A-Za-z0-9_]+)`', line)
            row = re.match(r'INSERT INTO `([A-Za-z0-9_]+)` VALUES ', line)
            if table:
                counts[table[1]] += 0
            if row:
                counts[row[1]] += 1
    if not counts:
        raise ValueError('Backup contains no tables')
    return dict(sorted(counts.items()))


def create(args):
    started = time.monotonic()
    database = identifier(args.database or container(['printenv', 'MYSQL_DATABASE']).decode().strip())
    non_transactional = query(
        f"SELECT COUNT(*) FROM information_schema.tables WHERE table_schema='{database}' "
        "AND table_type='BASE TABLE' AND engine <> 'InnoDB'")
    if non_transactional != '0':
        raise ValueError('Single-transaction backup requires all tables to use InnoDB')
    uploads = args.uploads.resolve()
    if not uploads.is_dir():
        raise ValueError('Uploads directory does not exist; pass the correct --uploads directory')
    backup = BACKUPS / (datetime.now(timezone.utc).strftime('%Y%m%dT%H%M%SZ') + '-' + uuid.uuid4().hex[:8])
    backup.mkdir(mode=0o700)
    manifest = {'format': 1, 'source_database': database, 'snapshot_started_at': now(),
                'source_uploads': str(uploads), 'consistency': 'InnoDB single transaction; uploads copied afterward. Quiesce writes and DDL for cross-resource consistency.'}
    dump = backup / 'database.sql'
    with dump.open('wb') as output:
        container(['mysqldump', '-uroot', '--single-transaction', '--quick', '--skip-extended-insert',
                   '--hex-blob', '--skip-comments', '--skip-dump-date', '--skip-add-locks',
                   '--set-gtid-purged=OFF', '--no-tablespaces', '--column-statistics=0', database], output=output)
    manifest['snapshot_finished_at'] = now()
    manifest['table_rows'] = dump_counts(dump)
    files = {}
    archive = backup / 'uploads.tar.gz'
    with tarfile.open(archive, 'w:gz') as tar:
        for file in sorted(uploads.rglob('*')):
            if file.is_symlink():
                raise ValueError('Uploads must not contain symlinks')
            if not file.is_file():
                continue
            before = file.stat()
            name = file.relative_to(uploads).as_posix()
            files[name] = {'sha256': digest(file), 'size': before.st_size}
            tar.add(file, arcname=name, recursive=False)
            after = file.stat()
            if (before.st_size, before.st_mtime_ns) != (after.st_size, after.st_mtime_ns):
                raise RuntimeError('An upload changed during backup; quiesce uploads and retry')
    manifest.update(files=files, artifacts={p.name: digest(p) for p in (dump, archive)},
                    completed_at=now(), duration_seconds=round(time.monotonic() - started, 3))
    private_json(backup / 'manifest.json', manifest)
    for file in backup.iterdir():
        file.chmod(0o600)
    print(backup)
    return 0


def local_asset(url):
    parsed = urlsplit(url)
    if parsed.scheme or parsed.netloc or not parsed.path.startswith('/uploads/'):
        return None
    path = PurePosixPath(unquote(parsed.path[len('/uploads/'):]))
    if path.is_absolute() or '..' in path.parts:
        raise ValueError('Unsafe upload reference in snapshot')
    return path.as_posix()


def relationships(database, tables):
    checks = {}
    queries = {
        'video_without_author': ({'video', 'account'}, 'SELECT COUNT(*) FROM video v LEFT JOIN account a ON a.id=v.author_id WHERE a.id IS NULL'),
        'video_without_stat': ({'video', 'video_stat'}, 'SELECT COUNT(*) FROM video v LEFT JOIN video_stat s ON s.video_id=v.id WHERE s.video_id IS NULL'),
        'action_without_video_or_user': ({'interaction_action', 'video', 'account'}, 'SELECT COUNT(*) FROM interaction_action x LEFT JOIN video v ON v.id=x.video_id LEFT JOIN account a ON a.id=x.user_id WHERE v.id IS NULL OR a.id IS NULL'),
        'comment_without_video_or_user': ({'interaction_comment', 'video', 'account'}, 'SELECT COUNT(*) FROM interaction_comment x LEFT JOIN video v ON v.id=x.video_id LEFT JOIN account a ON a.id=x.user_id WHERE v.id IS NULL OR a.id IS NULL'),
        'upload_without_owner': ({'upload_asset', 'account'}, 'SELECT COUNT(*) FROM upload_asset u LEFT JOIN account a ON a.id=u.owner_id WHERE a.id IS NULL'),
        'video_counter_mismatch': ({'video_stat', 'interaction_action', 'interaction_comment'}, '''SELECT COUNT(*) FROM video_stat s WHERE
          s.like_count <> (SELECT COUNT(*) FROM interaction_action a WHERE a.video_id=s.video_id AND a.action_type='LIKE' AND a.status=1) OR
          s.favorite_count <> (SELECT COUNT(*) FROM interaction_action a WHERE a.video_id=s.video_id AND a.action_type='FAVORITE' AND a.status=1) OR
          s.comment_count <> (SELECT COUNT(*) FROM interaction_comment c WHERE c.video_id=s.video_id AND c.status=1)'''),
    }
    for name, (required, sql) in queries.items():
        if required <= tables:
            checks[name] = int(query(sql, database))
    return checks


def drill(args):
    started = time.monotonic()
    backup = args.backup.resolve()
    manifest = json.loads((backup / 'manifest.json').read_text())
    if manifest['format'] != 1:
        raise ValueError('Unsupported backup format')
    for name in ('database.sql', 'uploads.tar.gz'):
        if digest(backup / name) != manifest['artifacts'][name]:
            raise ValueError(f'Checksum mismatch: {name}')
    if dump_counts(backup / 'database.sql') != manifest['table_rows']:
        raise ValueError('Manifest table counts differ from SQL snapshot')
    # Dumps are generated without --databases; refuse database-switching statements.
    with (backup / 'database.sql').open() as file:
        if any(re.match(r'\s*(USE\s|(?:CREATE|DROP)\s+DATABASE\s)', line, re.I) for line in file):
            raise ValueError('Dump must not contain database switching/creation/deletion')
    database = 'gcfeed_restore_test_' + uuid.uuid4().hex
    report = {'backup': str(backup), 'snapshot_started_at': manifest['snapshot_started_at'],
              'snapshot_finished_at': manifest['snapshot_finished_at'], 'started_at': now(),
              'temporary_database': database, 'artifact_hashes_match': True, 'passed': False,
              'comparison': 'Restored data versus exported SQL snapshot, not concurrently changing source database.'}
    temp = Path(tempfile.mkdtemp(prefix='restore-files-', dir=BACKUPS))
    created = False
    try:
        query(f'CREATE DATABASE `{database}` CHARACTER SET utf8mb4 COLLATE utf8mb4_0900_ai_ci')
        created = True
        restore_started = time.monotonic()
        # Stream the SQL rather than loading the full database into memory.
        with (backup / 'database.sql').open('rb') as source:
            result = subprocess.run([str(ROOT / 'scripts/compose.sh'), 'exec', '-T', 'mysql', 'sh', '-c',
                                     'export MYSQL_PWD="$MYSQL_ROOT_PASSWORD"; exec mysql -uroot "$1"',
                                     'sh', database], stdin=source, stdout=subprocess.DEVNULL, stderr=subprocess.PIPE)
        if result.returncode:
            raise RuntimeError(f'Snapshot restore failed (exit {result.returncode})')
        report['sql_restore_seconds'] = round(time.monotonic() - restore_started, 3)
        actual = {table: int(query(f'SELECT COUNT(*) FROM `{identifier(table)}`', database))
                  for table in manifest['table_rows']}
        report['table_rows'] = actual
        report['table_rows_match'] = actual == manifest['table_rows']
        seen = set()
        with tarfile.open(backup / 'uploads.tar.gz') as tar:
            for member in tar:
                name = member.name
                path = PurePosixPath(name)
                if (not member.isfile() or path.is_absolute() or '..' in path.parts
                        or name not in manifest['files'] or name in seen):
                    raise ValueError('Unexpected archive entry')
                seen.add(name)
                target = temp / name
                target.parent.mkdir(parents=True, exist_ok=True, mode=0o700)
                with tar.extractfile(member) as source, target.open('wb') as destination:
                    shutil.copyfileobj(source, destination)
                expected = manifest['files'][name]
                if target.stat().st_size != expected['size'] or digest(target) != expected['sha256']:
                    raise ValueError('Restored asset checksum mismatch')
        if seen != set(manifest['files']):
            raise ValueError('Archive has missing files')
        report['asset_hashes_match'] = True
        report['restored_files'] = len(seen)
        report['relationship_violations'] = relationships(database, set(actual))
        missing_registered = []
        if 'upload_asset' in actual:
            for line in query('SELECT url, size FROM upload_asset', database).splitlines():
                url, size = line.split('\t')
                name = local_asset(url)
                if name is None or name not in seen or (temp / name).stat().st_size != int(size):
                    missing_registered.append(url)
        report['missing_or_invalid_registered_assets'] = missing_registered
        report['registered_assets_checked'] = actual.get('upload_asset', 0)
        legacy_missing = set()
        if 'video' in actual:
            for line in query('SELECT media_url, cover_url FROM video', database).splitlines():
                for url in line.split('\t'):
                    name = local_asset(url)
                    if name is not None and name not in seen:
                        legacy_missing.add(url)
        report['missing_legacy_video_files'] = sorted(legacy_missing)
        report['passed'] = (report['table_rows_match'] and not missing_registered
                            and not any(report['relationship_violations'].values()))
        report['legacy_note'] = 'Pre-registry video paths may be historical fixtures; missing paths remain explicitly reported, not counted as recoverable media.'
    except Exception as error:
        report['error'] = str(error)
        raise
    finally:
        # Only this invocation's random database can reach DROP; source DB is never passed here.
        report['temporary_resources_removed'] = False
        try:
            if created:
                query(f'DROP DATABASE `{database}`')
            report['temporary_resources_removed'] = True
        finally:
            shutil.rmtree(temp)
            report.update(finished_at=now(), duration_seconds=round(time.monotonic() - started, 3))
            private_json(backup / 'restore-report.json', report)
    print(json.dumps(report, ensure_ascii=False, indent=2))
    return 0 if report['passed'] else 1


def main():
    os.umask(0o077)
    BACKUPS.mkdir(parents=True, exist_ok=True, mode=0o700)
    BACKUPS.chmod(0o700)
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest='command', required=True)
    create_parser = commands.add_parser('create', help='Back up local MySQL and native uploads')
    create_parser.add_argument('--database', help='Defaults to Compose MYSQL_DATABASE; read-only source')
    create_parser.add_argument('--uploads', type=Path, default=ROOT / 'apps/api/uploads')
    drill_parser = commands.add_parser('drill', help='Restore a backup into a fresh throwaway database/files directory')
    drill_parser.add_argument('backup', type=Path)
    args = parser.parse_args()
    try:
        return create(args) if args.command == 'create' else drill(args)
    except (OSError, ValueError, KeyError, RuntimeError, tarfile.TarError) as error:
        print(f'Backup/drill failed: {error}', file=__import__('sys').stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
