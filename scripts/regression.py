#!/usr/bin/env python3
"""Local API regression. Creates its own accounts/video; deletes only its own cache keys."""
import json
import os
from pathlib import Path
import secrets
import subprocess
import time
import tempfile
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
BASE = os.environ.get('GCFEED_SMOKE_URL', 'http://127.0.0.1:18080').rstrip('/')
RESULTS = {}


def request(method, path, body=None, token='', headers=None, expected=200):
    headers = dict(headers or {})
    if token:
        headers['Authorization'] = 'Bearer ' + token
    data = None
    if body is not None:
        data = json.dumps(body).encode()
        headers['Content-Type'] = 'application/json'
    req = urllib.request.Request(BASE + path, data=data, headers=headers, method=method)
    try:
        response = urllib.request.urlopen(req, timeout=30)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        content = response.read()
        if response.code != expected:
            raise AssertionError(f'{method} {path}: expected {expected}, got {response.code}')
        return json.loads(content) if content else None


def account():
    credentials = {'account': 'regress_' + secrets.token_hex(6), 'password': secrets.token_urlsafe(24)}
    request('POST', '/api/users', {**credentials, 'nickname': '回归测试'}, expected=201)
    token = request('POST', '/api/sessions', credentials)['access_token']
    return token, request('GET', '/api/users/me', token=token)['id']


def upload_file(token, path, kind):
    boundary = 'gcfeed-' + secrets.token_hex(12)
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="kind"\r\n\r\n{kind}\r\n'
            f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{path.name}"\r\n'
            'Content-Type: application/octet-stream\r\n\r\n').encode() + path.read_bytes() + f'\r\n--{boundary}--\r\n'.encode()
    req = urllib.request.Request(BASE + '/api/uploads', data=body, headers={
        'Authorization': 'Bearer ' + token, 'Content-Type': f'multipart/form-data; boundary={boundary}'})
    with urllib.request.urlopen(req, timeout=60) as response:
        assert response.status == 201
        return json.load(response)['url']


def upload_assets(token):
    with tempfile.TemporaryDirectory() as directory:
        root = Path(directory)
        subprocess.run(['ffmpeg', '-loglevel', 'error', '-f', 'lavfi', '-i', 'color=c=blue:s=160x120:d=1',
                        '-c:v', 'libx264', '-pix_fmt', 'yuv420p', str(root / 'clip.mp4')], check=True)
        subprocess.run(['ffmpeg', '-loglevel', 'error', '-i', str(root / 'clip.mp4'), '-frames:v', '1',
                        str(root / 'cover.png')], check=True)
        return {'media_url': upload_file(token, root / 'clip.mp4', 'video'),
                'cover_url': upload_file(token, root / 'cover.png', 'cover')}


def check(name, condition):
    if not condition:
        raise AssertionError(name)
    RESULTS[name] = True
    print('PASS', name)


def main():
    # HTTP requests may target only the local compose environment whose Redis is modified below.
    if BASE != 'http://127.0.0.1:18080':
        raise SystemExit('Regression is restricted to the local GCFeed environment (127.0.0.1:18080).')
    env = dict(line.split('=', 1) for line in (ROOT / 'apps/.env').read_text().splitlines()
               if line and not line.startswith('#') and '=' in line)
    internal = {'X-Internal-Token': env['GCFEED_INTERNAL_TOKEN']}
    ready = request('GET', '/ready')
    check('api_dependencies_ready', ready['ready'] and all(ready['dependencies'].values()))
    for path in ('recommendation-candidates', 'exposure-decisions', 'exposures', 'messages', 'playback-qos-reports'):
        for headers in ({}, {'X-Internal-Token': 'invalid'}):
            request('POST', '/internal/' + path, {}, headers=headers, expected=401)
    check('all_internal_routes_require_service_token', True)
    author, author_id = account()
    reader, reader_id = account()
    video_id = None
    following = '/api/users/me/following/' + str(author_id)
    try:
        assets = upload_assets(author)
        publication = {'title': 'asset ownership check', 'description': '', **assets}
        request('POST', '/api/videos', publication, token=reader, expected=403)
        request('POST', '/api/videos', {**publication, 'media_url': assets['cover_url']}, token=author, expected=403)
        request('POST', '/api/videos', {**publication, 'media_url': 'https://example.invalid/video.mp4'}, token=author, expected=403)
        request('PATCH', '/api/users/me', {'nickname': '回归测试', 'avatar_url': assets['cover_url'], 'bio': ''}, token=reader, expected=403)
        check('unowned_missing_and_wrong_kind_assets_rejected', True)
        video = request('POST', '/api/videos', {
            'title': '一致性回归 ' + secrets.token_hex(4), 'description': 'temporary fixture',
            **assets,
        }, token=author, headers={'Idempotency-Key': secrets.token_hex(16)}, expected=201)
        video_id = video['id']
        request('POST', '/internal/recommendation-candidates', {'user_id': reader_id, 'scene': 'recommend', 'limit': 1}, headers=internal)
        request('POST', '/internal/exposure-decisions', {'user_id': reader_id, 'scene': 'recommend', 'video_ids': [video_id]}, headers=internal)
        check('valid_internal_token_accepted', True)
        request('PUT', following, token=reader)
        def feed(scene):
            return request('GET', '/api/feed-items?scene=' + scene + '&limit=50', token=reader)['items']
        def contains(items):
            return any(item['video_id'] == video_id for item in items)
        check('followed_video_visible', contains(feed('following')))
        request('DELETE', following, token=reader)
        check('unfollow_removes_cached_video', not contains(feed('following')))
        request('PUT', following, token=reader)
        check('refollow_restores_video', contains(feed('following')))
        def wait_count(count):
            for _ in range(50):
                if request('GET', f'/api/videos/{video_id}', token=reader)['like_count'] == count:
                    return
                time.sleep(.2)
            raise AssertionError('database failed to commit like count')
        def clear_own_cache():
            keys = [f'video:stat:v1:{video_id}']
            subprocess.run([str(ROOT / 'scripts/compose.sh'), 'exec', '-T', 'redis', 'redis-cli', 'DEL', *keys],
                           check=True, capture_output=True, timeout=15)
        request('PUT', f'/api/videos/{video_id}/like', token=reader, headers={'Idempotency-Key': 'initial'})
        wait_count(1)
        clear_own_cache()
        liked = request('PUT', f'/api/videos/{video_id}/like', token=reader, headers={'Idempotency-Key': 'cold-repeat'})
        check('cold_repeat_like_does_not_double_count', liked['like_count'] == 1)
        clear_own_cache()
        unliked = request('DELETE', f'/api/videos/{video_id}/like', token=reader, headers={'Idempotency-Key': 'cold-cancel'})
        check('cold_unlike_returns_zero', unliked['like_count'] == 0)
        wait_count(0)
        check('cold_unlike_committed_in_database', True)
        historical = request('PUT', f'/api/videos/{video_id}/like', token=reader, headers={'Idempotency-Key': 'initial'})
        current = request('GET', f'/api/videos/{video_id}', token=reader)
        check('historical_request_returns_original_result', historical['like_count'] == 1)
        check('historical_request_does_not_revert_current_state', current['like_count'] == 0)
        # The real Worker must project notifications after the API committed the action.
        notification_id = f'interaction:like:{video_id}:{reader_id}'
        for _ in range(50):
            messages = request('GET', '/api/messages', token=author)['items']
            matching = [item for item in messages if item.get('event_id') == notification_id]
            if matching:
                break
            time.sleep(.2)
        check('durable_like_notification_delivered_once', len(matching) == 1 and matching[0]['actor_id'] == reader_id)
        viewed = request('POST', '/api/video-view-events', {
            'video_id': video_id, 'scene': 'timeline', 'event_type': 'complete',
            'watch_ms': 1000, 'completed': True,
        }, token=reader, expected=201)
        check('view_event_committed', viewed['event']['id'] > 0 and viewed['event']['user_id'] == reader_id)
        # Warm both page and card caches, then delete through the author API.
        for _ in range(30):
            if contains(feed('timeline')):
                break
            time.sleep(.2)
        else:
            raise AssertionError('timeline did not contain fixture before deletion')
        check('following_warm_before_delete', contains(feed('following')))
        request('DELETE', f'/api/videos/{video_id}', token=author, expected=204)
        request('GET', f'/api/videos/{video_id}', expected=404)
        check('deleted_video_hidden_from_cached_following', not contains(feed('following')))
        check('deleted_video_hidden_from_cached_timeline', not contains(feed('timeline')))
        video_id = None
    finally:
        if video_id:
            request('DELETE', f'/api/videos/{video_id}', token=author, expected=204)
        request('DELETE', following, token=reader)
    output = ROOT / '.local/regression-results.json'
    output.parent.mkdir(exist_ok=True)
    output.write_text(json.dumps(RESULTS, ensure_ascii=False, indent=2) + '\n')
    print('Regression passed; fixture video soft-deleted; test accounts retained.')


if __name__ == '__main__':
    main()
