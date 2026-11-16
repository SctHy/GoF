#!/usr/bin/env python3
"""Reproducible local mixed HTTP workload; preserves existing data and accounts."""
import argparse
from collections import Counter, defaultdict
from concurrent.futures import ThreadPoolExecutor
import datetime
import json
import math
import os
from pathlib import Path
import platform
import random
import resource
import secrets
import subprocess
import threading
import time
import urllib.request

import regression as api

ROOT = Path(__file__).resolve().parents[1]
FEED = '/api/feed-items?scene=timeline&limit=20'


def save_private(path, value):
    path.parent.mkdir(parents=True, exist_ok=True)
    with os.fdopen(os.open(path, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600), 'w') as handle:
        json.dump(value, handle)
    path.chmod(0o600)


def prepare(path):
    state = json.loads(path.read_text()) if path.exists() else {'accounts': []}
    for account in state['accounts']:
        api.request('GET', '/api/users/me', token=account['token'])
    while len(state['accounts']) < 5:
        # Retain each successful account immediately. No retry of successful registration.
        credentials = {'account': 'mixed_' + secrets.token_hex(6), 'password': secrets.token_urlsafe(24)}
        for attempt in range(3):
            try:
                api.request('POST', '/api/users', {**credentials, 'nickname': '混合负载样本'}, expected=201)
                break
            except AssertionError as error:
                if 'got 429' not in str(error) or attempt == 2:
                    raise
                print('Registration rate limit: waiting 61 seconds without changing limits.', flush=True)
                for _ in range(2):
                    time.sleep(30.5)
        token = api.request('POST', '/api/sessions', credentials)['access_token']
        user_id = api.request('GET', '/api/users/me', token=token)['id']
        state['accounts'].append({'token': token, 'id': user_id})
        save_private(path, state)
        print(f"Prepared benchmark account {len(state['accounts'])}/5", flush=True)
    if 'assets' not in state:
        state['assets'] = api.upload_assets(state['accounts'][0]['token'])
        save_private(path, state)
    return state


def sql(query, admin=False):
    # Only read-only, script-defined queries. Credentials remain in the container env.
    command = ('MYSQL_PWD="$MYSQL_ROOT_PASSWORD" mysql -uroot GCFeed --batch --skip-column-names'
               if admin else 'MYSQL_PWD="$MYSQL_PASSWORD" mysql -ugcfeed GCFeed --batch --skip-column-names')
    result = subprocess.run(
        [str(ROOT / 'scripts/compose.sh'), 'exec', '-T', 'mysql', 'sh', '-c',
         command],
        input=query + ';\n', text=True, capture_output=True, timeout=20, check=True)
    return result.stdout.strip()


def outbox_state(ids):
    value = sql('SELECT COUNT(*), COALESCE(SUM(processed_at IS NULL AND failed_at IS NULL),0), '
                'COALESCE(SUM(failed_at IS NOT NULL),0) FROM event_outbox '
                "WHERE JSON_EXTRACT(payload, '$.video_id') IN (" + ','.join(map(str, ids)) + ')')
    return dict(zip(('total', 'pending', 'failed'), map(int, value.split('\t'))))


def metrics(url):
    with urllib.request.urlopen(url + '/metrics', timeout=10) as response:
        return response.read().decode()


def selected_metrics(raw):
    result = {}
    for line in raw.splitlines():
        if not line or line.startswith('#'):
            continue
        key, value = line.rsplit(' ', 1)
        if key.startswith(('gcfeed_db_', 'gcfeed_outbox_', 'process_cpu_seconds_total',
                           'process_resident_memory_bytes', 'gcfeed_worker_jobs_total',
                           'gcfeed_feed_cache_requests_total')):
            result[key] = float(value)
    return result


def snapshot(ids, target, tag, lock_samples=False):
    result = {'at': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'tag': tag}
    # macOS Go processes may not expose the Linux process collector.
    process_rows = subprocess.check_output(['ps', '-axo', 'pid=,time=,rss=,comm='], text=True)
    result['processes'] = {}
    for line in process_rows.splitlines():
        pid, cpu_time, rss, command = line.split(None, 3)
        if command in (str(ROOT / '.local/gcfeed-api'), str(ROOT / '.local/gcfeed-worker')):
            seconds = 0.0
            for component in cpu_time.split(':'):
                seconds = seconds * 60 + float(component)
            result['processes'][Path(command).name] = {
                'pid': int(pid), 'cpu_seconds': seconds, 'rss_kib': int(rss)}
    for service, base in [('api', api.BASE), ('worker', 'http://127.0.0.1:19091')]:
        try:
            raw = metrics(base)
            (target / f'{tag}-{service}.prom').write_text(raw)
            result[service] = selected_metrics(raw)
        except Exception as error:
            result[service + '_error'] = type(error).__name__
    result['outbox_fixture'] = outbox_state(ids)
    result['innodb'] = dict(line.split('\t') for line in sql(
        "SHOW GLOBAL STATUS WHERE Variable_name IN ('Innodb_row_lock_waits', 'Innodb_row_lock_time', "
        "'Innodb_deadlocks', 'Threads_running', 'Threads_connected')").splitlines())
    if lock_samples:
        raw = sql("SELECT l.OBJECT_NAME, l.INDEX_NAME, l.LOCK_MODE, "
                  "COUNT(DISTINCT w.REQUESTING_ENGINE_LOCK_ID) FROM performance_schema.data_lock_waits w "
                  "JOIN performance_schema.data_locks l ON l.ENGINE=w.ENGINE "
                  "AND l.ENGINE_LOCK_ID=w.REQUESTING_ENGINE_LOCK_ID WHERE l.OBJECT_SCHEMA='GCFeed' "
                  "GROUP BY l.OBJECT_NAME,l.INDEX_NAME,l.LOCK_MODE", admin=True)
        result['lock_wait_objects'] = [dict(zip(('table', 'index', 'mode', 'waiting_locks'), row.split('\t')))
                                      for row in raw.splitlines()]
    return result


def summarize(rows, elapsed):
    groups = defaultdict(list)
    for operation, latency, error in rows:
        groups[operation].append((latency, error))
    output = {}
    for operation, values in sorted(groups.items()):
        latencies = sorted(value[0] for value in values)
        errors = Counter(error for _, error in values if error)
        output[operation] = {
            'requests': len(values), 'successes': len(values) - sum(errors.values()),
            'success_rate': (len(values) - sum(errors.values())) / len(values),
            'errors': dict(errors), 'elapsed_seconds': elapsed,
            'completed_per_second': len(values) / elapsed,
            **{f'p{int(p * 100)}_ms': latencies[max(0, math.ceil(len(latencies) * p) - 1)]
               for p in (.50, .95, .99)},
        }
    return output


def phase(scenario, duration, concurrency, accounts, ids, seed):
    barrier = threading.Barrier(concurrency + 1)
    start = [0.0]

    def actor(index):
        token = accounts[1 + index % 4]['token']
        rng = random.Random(seed + index)
        writer = index < 4  # At most one writer per user; additional actors only read.
        rows = []
        barrier.wait()
        deadline = start[0] + duration

        def call(operation, method, path):
            before = time.perf_counter()
            error = None
            try:
                api.request(method, path, token=token)
            except Exception as exc:
                error = str(exc) if isinstance(exc, AssertionError) else type(exc).__name__
            rows.append((operation, (time.perf_counter() - before) * 1000, error))

        while time.perf_counter() < deadline:
            video_id = ids[-1] if scenario == 'single_video_hotspot' else rng.choice(ids)
            # Writers do 6 reads + a complete like/unlike pair. Readers do 6 reads.
            for _ in range(3):
                call('feed', 'GET', FEED)
                call('video', 'GET', f'/api/videos/{video_id}')
            if writer:
                call('like', 'PUT', f'/api/videos/{video_id}/like')
                call('unlike', 'DELETE', f'/api/videos/{video_id}/like')
        return rows

    cpu_start = time.process_time()
    with ThreadPoolExecutor(max_workers=concurrency) as executor:
        futures = [executor.submit(actor, index) for index in range(concurrency)]
        start[0] = time.perf_counter()
        barrier.wait()
        rows = [row for future in futures for row in future.result()]
    elapsed = time.perf_counter() - start[0]
    cpu = time.process_time() - cpu_start
    return {'elapsed_seconds': elapsed, 'configured_seconds': duration, 'requests': len(rows),
            'completed_per_second': len(rows) / elapsed, 'client_cpu_seconds': cpu,
            'client_cpu_cores': cpu / elapsed, 'operations': summarize(rows, elapsed)}


def drain(ids, timeout=120):
    start = time.monotonic()
    while True:
        state = outbox_state(ids)
        if not state['pending'] or time.monotonic() - start >= timeout:
            return {**state, 'wait_seconds': time.monotonic() - start,
                    'drained': state['pending'] == 0 and state['failed'] == 0}
        time.sleep(1)


def reconcile(ids):
    rows = sql('SELECT s.video_id, s.like_count, COUNT(a.id) FROM video_stat AS s '
               'LEFT JOIN interaction_action AS a ON a.video_id=s.video_id '
               "AND a.action_type='LIKE' AND a.status=1 WHERE s.video_id IN (" +
               ','.join(map(str, ids)) + ') GROUP BY s.video_id, s.like_count ORDER BY s.video_id')
    values = [list(map(int, row.split('\t'))) for row in rows.splitlines()]
    return {'columns': ['video_id', 'stored_likes', 'active_like_facts'], 'rows': values,
            'matched': len(values) == len(ids) and all(row[1] == row[2] == 0 for row in values)}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--prepare-only', action='store_true')
    parser.add_argument('--sessions', default='.local/mixed-sessions.json')
    parser.add_argument('--output', default='.local/mixed-benchmark')
    parser.add_argument('--concurrency', type=int, default=8)
    parser.add_argument('--warmup', type=float, default=5)
    parser.add_argument('--seconds', type=float, default=30)
    parser.add_argument('--videos', type=int, default=24)
    parser.add_argument('--seed', type=int, default=1729)
    parser.add_argument('--lock-samples', action='store_true',
                        help='Read performance_schema lock wait objects with the local container root account')
    args = parser.parse_args()
    if api.BASE != 'http://127.0.0.1:18080':
        parser.error('Restricted to local GCFeed API 127.0.0.1:18080')
    if not 4 <= args.concurrency <= 64 or not 20 <= args.videos <= 1000:
        parser.error('concurrency must be 4-64; videos must be 20-1000')
    if not 0 < args.warmup <= 60 or not 0 < args.seconds <= 300:
        parser.error('warmup must be 0-60s; seconds must be 0-300s')
    state = prepare(Path(args.sessions))
    if args.prepare_only:
        return
    target = Path(args.output)
    target.mkdir(parents=True, exist_ok=True)
    token = state['accounts'][0]['token']
    ids = []
    cpu_model = (subprocess.check_output(['sysctl', '-n', 'machdep.cpu.brand_string'], text=True).strip()
                 if platform.system() == 'Darwin' else platform.processor())
    result = {'created_at': datetime.datetime.now(datetime.timezone.utc).isoformat(),
              'environment': {'platform': platform.platform(), 'logical_cpus': os.cpu_count(),
                              'cpu_model': cpu_model, 'python': platform.python_version()},
              'config': {k: v for k, v in vars(args).items() if k != 'sessions'},
              'model': 'Closed-loop Python thread pool, urllib new connection per request; 4 unique '
                       'writer users, remaining threads read-only. Writers cycle 3 Feed + 3 video + '
                       'like/unlike, readers cycle 3 Feed + 3 video. No pacing, finish in-flight cycle '
                       'at deadline. Latencies include errors; warmup excluded. Seeded uniform video '
                       'selection or one hot video. No idempotency header, no automatic HTTP retries.',
              'scenarios': []}
    try:
        for index in range(args.videos):
            video = api.request('POST', '/api/videos', {
                'title': f'混合压测 {index}', 'description': 'temporary mixed workload fixture',
                **state['assets']}, token=token, expected=201)
            ids.append(video['id'])
            (target / 'fixture-ids.json').write_text(json.dumps(ids))
        result['fixture_video_ids'] = ids
        result['hot_video_id'] = ids[-1]
        for _ in range(50):
            items = api.request('GET', FEED, token=state['accounts'][1]['token'])['items']
            if len(items) == 20 and ids[-1] in [item['video_id'] for item in items]:
                break
            time.sleep(.2)
        else:
            raise RuntimeError('Expected a 20-item Feed page containing the hot video')
        result['feed_preflight'] = {'items': len(items), 'contains_hot_video': True}
        result['database_before'] = sql('SELECT '
            '(SELECT COUNT(*) FROM video), (SELECT COUNT(*) FROM video WHERE status=2), '
            '(SELECT COUNT(*) FROM account), (SELECT COUNT(*) FROM interaction_action), '
            '(SELECT COUNT(*) FROM event_outbox)')
        result['database_columns'] = ['video_total', 'video_published', 'account_total',
                                      'interaction_action_total', 'event_outbox_total']
        explain_queries = {
            'timeline': 'SELECT id, author_id, published_at FROM video WHERE status=2 AND '
                        'published_at IS NOT NULL ORDER BY published_at DESC,id DESC LIMIT 21',
            'action_lookup': f'SELECT * FROM interaction_action WHERE user_id={state["accounts"][1]["id"]} '
                             f'AND video_id={ids[0]} AND action_type="LIKE"',
            'stat_lookup': f'SELECT * FROM video_stat WHERE video_id={ids[0]}',
        }
        for scenario in ('uniform', 'single_video_hotspot'):
            print(f'{scenario}: warming {args.warmup}s', flush=True)
            warmup = phase(scenario, args.warmup, args.concurrency, state['accounts'], ids, args.seed)
            before = snapshot(ids, target, scenario + '-before', args.lock_samples)
            samples = [before]
            stop = threading.Event()

            def monitor():
                index = 1
                while not stop.wait(5):
                    try:
                        samples.append(snapshot(ids, target, f'{scenario}-{index:02d}', args.lock_samples))
                    except Exception as error:
                        samples.append({'error': type(error).__name__})
                    index += 1

            watcher = threading.Thread(target=monitor)
            watcher.start()
            print(f'{scenario}: sampling {args.seconds}s', flush=True)
            try:
                measured = phase(scenario, args.seconds, args.concurrency, state['accounts'], ids, args.seed)
            finally:
                stop.set()
                watcher.join()
            samples.append(snapshot(ids, target, scenario + '-after', args.lock_samples))
            measured.update({'scenario': scenario, 'warmup': warmup, 'samples': samples,
                             'reconciliation': reconcile(ids), 'outbox_drain': drain(ids)})
            result['scenarios'].append(measured)
            print(json.dumps({key: value for key, value in measured.items()
                              if key not in ('warmup', 'samples', 'reconciliation')}, ensure_ascii=False), flush=True)
        result['explain'] = {name: {'query': query, 'plan': sql('EXPLAIN ' + query)}
                             for name, query in explain_queries.items()}
        result['passed'] = all(s['reconciliation']['matched'] and s['outbox_drain']['drained'] and
                               all(o['success_rate'] == 1 for o in s['operations'].values()) and
                               all(o['success_rate'] == 1 for o in s['warmup']['operations'].values()) and
                               not any('error' in key for sample in s['samples'] for key in sample)
                               for s in result['scenarios'])
    finally:
        cleanup_errors = []
        for video_id in ids:
            try:
                api.request('DELETE', f'/api/videos/{video_id}', token=token, expected=204)
            except Exception as error:
                cleanup_errors.append({'video_id': video_id, 'error': type(error).__name__})
        result['cleanup_errors'] = cleanup_errors
        result['max_rss_platform_units'] = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss
        (target / 'result.json').write_text(json.dumps(result, ensure_ascii=False, indent=2) + '\n')
    if not result.get('passed') or result['cleanup_errors']:
        raise SystemExit('Benchmark failed; inspect result.json')


if __name__ == '__main__':
    main()
