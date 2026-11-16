#!/usr/bin/env python3
"""Local integration smoke; leaves test account/files, soft-deletes its video."""
import json
import os
from pathlib import Path
import secrets
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

BASE = os.environ.get('GCFEED_SMOKE_URL', 'http://127.0.0.1:18080').rstrip('/')
TOKEN = ''

def request(method, path, body=None, headers=None, expected=200):
    merged = dict(headers or {})
    if TOKEN:
        merged['Authorization'] = f'Bearer {TOKEN}'
    if isinstance(body, dict):
        body = json.dumps(body).encode()
        merged['Content-Type'] = 'application/json'
    req = urllib.request.Request(BASE + path, data=body, headers=merged, method=method)
    try:
        with urllib.request.urlopen(req, timeout=30) as response:
            data = response.read()
            if response.status != expected:
                raise RuntimeError(f'{method} {path}: expected {expected}, got {response.status}')
            if response.headers.get_content_type() == 'application/json':
                return json.loads(data)
            return data
    except urllib.error.HTTPError as error:
        raise RuntimeError(f'{method} {path}: HTTP {error.code}: {error.read().decode()[:300]}') from None

def upload(path, kind):
    boundary = 'gcfeed-' + secrets.token_hex(12)
    mime = 'video/mp4' if kind == 'video' else 'image/png'
    body = (f'--{boundary}\r\nContent-Disposition: form-data; name="kind"\r\n\r\n{kind}\r\n'
            f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{path.name}"\r\n'
            f'Content-Type: {mime}\r\n\r\n').encode() + path.read_bytes() + f'\r\n--{boundary}--\r\n'.encode()
    return request('POST', '/api/uploads', body, {'Content-Type': f'multipart/form-data; boundary={boundary}'}, 201)['url']

def wait_count(video_id, count):
    for _ in range(50):
        if request('GET', f'/api/videos/{video_id}')['like_count'] == count:
            return
        time.sleep(.2)
    raise RuntimeError(f'Database did not commit like_count={count} within 10 seconds')

def main():
    global TOKEN
    request('GET', '/health')
    suffix = secrets.token_hex(6)
    account = 'smoke_' + suffix
    password = secrets.token_urlsafe(24)
    request('POST', '/api/users', {'account': account, 'password': password, 'nickname': '环境验收'}, expected=201)
    TOKEN = request('POST', '/api/sessions', {'account': account, 'password': password})['access_token']
    request('GET', '/api/users/me')
    print('PASS register/login/profile')
    video_id = None
    try:
        with tempfile.TemporaryDirectory() as temp:
            root = Path(temp)
            subprocess.run(['ffmpeg', '-loglevel', 'error', '-f', 'lavfi', '-i', 'color=c=blue:s=160x120:d=1', '-c:v', 'libx264', '-pix_fmt', 'yuv420p', str(root / 'clip.mp4')], check=True)
            subprocess.run(['ffmpeg', '-loglevel', 'error', '-i', str(root / 'clip.mp4'), '-frames:v', '1', str(root / 'cover.png')], check=True)
            media = upload(root / 'clip.mp4', 'video')
            cover = upload(root / 'cover.png', 'cover')
            print('PASS upload/video processing')
            request('GET', media, headers={'Range': 'bytes=0-15'}, expected=206)
            video = request('POST', '/api/videos', {'title': '环境验收 ' + suffix, 'description': 'Temporary integration smoke video', 'media_url': media, 'cover_url': cover}, {'Idempotency-Key': suffix}, 201)
            video_id = video['id']
            for _ in range(20):
                feed = request('GET', '/api/feed-items?scene=timeline&limit=50')
                if any(item.get('video_id', item.get('id')) == video_id for item in feed['items']):
                    break
                time.sleep(.5)
            else:
                raise RuntimeError('Published video did not appear in feed')
            print('PASS publish/feed/range')
            request('PUT', f'/api/videos/{video_id}/like', headers={'Idempotency-Key': suffix + '-like'})
            wait_count(video_id, 1)
            request('DELETE', f'/api/videos/{video_id}/like', headers={'Idempotency-Key': suffix + '-unlike'})
            wait_count(video_id, 0)
            print('PASS like/unlike committed with durable events')
            request('GET', '/metrics')
            print('PASS metrics')
    finally:
        if video_id:
            request('DELETE', f'/api/videos/{video_id}', expected=204)
    print('Smoke passed; test video soft-deleted. Account and upload files retained.')

if __name__ == '__main__':
    main()
