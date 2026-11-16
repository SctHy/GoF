#!/usr/bin/env python3
"""Exercise a real, isolated Prometheus with synthetic Outbox metrics; no business service changes."""
import argparse
from datetime import datetime, timezone
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
import os
from pathlib import Path
import socket
import subprocess
import tempfile
import threading
import time
from urllib.request import urlopen

DIRECTORY = Path(__file__).resolve().parent
ROOT = DIRECTORY.parents[1]


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--prometheus', default='prometheus', help='Path to the official Prometheus executable')
    args = parser.parse_args()
    os.umask(0o077)
    output = ROOT / '.local/monitoring'
    output.mkdir(parents=True, exist_ok=True, mode=0o700)
    failed = threading.Event()
    failed.set()

    class Metrics(BaseHTTPRequestHandler):
        def do_GET(self):
            body = (f'gcfeed_outbox_failed {int(failed.is_set())}\n'
                    'gcfeed_outbox_pending 0\ngcfeed_outbox_oldest_pending_age_seconds 0\n'
                    f'gcfeed_outbox_stats_up 1\ngcfeed_outbox_stats_updated_timestamp_seconds {time.time()}\n').encode()
            self.send_response(200)
            self.send_header('Content-Type', 'text/plain; version=0.0.4')
            self.end_headers()
            self.wfile.write(body)

        def log_message(self, *args):
            pass

    exporter = ThreadingHTTPServer(('127.0.0.1', 0), Metrics)
    threading.Thread(target=exporter.serve_forever, daemon=True).start()
    with socket.socket() as reservation:
        reservation.bind(('127.0.0.1', 0))
        port = reservation.getsockname()[1]
    report = {'started_at': datetime.now(timezone.utc).isoformat(),
              'engine': subprocess.check_output([args.prometheus, '--version'], stderr=subprocess.STDOUT).decode().splitlines()[0],
              'alert': 'GCFeedOutboxFailed', 'rule_file': str(DIRECTORY / 'alerts.yml'),
              'scope': 'Own temporary Prometheus and synthetic exporter. Production rules unchanged; no external notification receiver.',
              'observations': [], 'passed': False}
    started = time.monotonic()
    with tempfile.TemporaryDirectory(prefix='alert-drill-', dir=output) as temporary:
        config = Path(temporary) / 'prometheus.yml'
        config.write_text(json.dumps({
            'global': {'scrape_interval': '1s', 'evaluation_interval': '1s'},
            'rule_files': [str(DIRECTORY / 'alerts.yml')],
            'scrape_configs': [{'job_name': 'gcfeed-worker', 'static_configs': [
                {'targets': [f'127.0.0.1:{exporter.server_port}']}]}],
        }))
        with (output / 'alert-drill.log').open('w') as log:
            process = subprocess.Popen([args.prometheus, f'--config.file={config}',
                                        f'--storage.tsdb.path={temporary}/data',
                                        f'--web.listen-address=127.0.0.1:{port}'], stdout=log, stderr=log)
            try:
                def wait_state(expected, timeout):
                    deadline = time.monotonic() + timeout
                    while time.monotonic() < deadline:
                        if process.poll() is not None:
                            raise RuntimeError('Temporary Prometheus exited; inspect .local/monitoring/alert-drill.log')
                        try:
                            with urlopen(f'http://127.0.0.1:{port}/api/v1/alerts', timeout=2) as response:
                                alerts = json.load(response)['data']['alerts']
                            matches = [a for a in alerts if a['labels']['alertname'] == report['alert']]
                            state = matches[0]['state'] if matches else 'inactive'
                            if state == expected:
                                report['observations'].append({'state': state, 'at': datetime.now(timezone.utc).isoformat(),
                                                               'elapsed_seconds': round(time.monotonic() - started, 3),
                                                               'alerts': matches})
                                return
                        except OSError:
                            pass
                        time.sleep(1)
                    raise RuntimeError(f'Alert did not reach {expected} within {timeout}s')

                wait_state('pending', 15)
                print('Real Prometheus reports pending; waiting for the unchanged 1m hold duration.', flush=True)
                wait_state('firing', 90)
                print('Real Prometheus reports firing; injecting recovery.', flush=True)
                failed.clear()
                wait_state('inactive', 15)
                report['passed'] = True
            finally:
                process.terminate()
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.kill()
                    process.wait()
                exporter.shutdown()
                exporter.server_close()
                report['duration_seconds'] = round(time.monotonic() - started, 3)
                (output / 'alert-drill.json').write_text(json.dumps(report, ensure_ascii=False, indent=2) + '\n')
    print(json.dumps(report, ensure_ascii=False, indent=2))


if __name__ == '__main__':
    main()
