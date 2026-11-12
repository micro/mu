# SPDX-License-Identifier: Apache-2.0
"""Black-box crash recovery against an unmodified Mu binary and a local model.

Usage: python3 proto/conformance/deployment.py /absolute/path/to/mu
Never points at a deployed instance or reads an existing account store.
"""
import concurrent.futures
import http.cookiejar
import json
import os
from pathlib import Path
import socket
import subprocess
import sys
import tempfile
import threading
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer


def port():
    with socket.socket() as s:
        s.bind(('127.0.0.1', 0))
        return s.getsockname()[1]


def eventually(fn, timeout=60):
    end = time.monotonic() + timeout
    while time.monotonic() < end:
        try:
            value = fn()
            if value:
                return value
        except (OSError, urllib.error.URLError):
            pass
        time.sleep(.1)
    raise AssertionError('Timed out waiting for runtime')


def main(binary):
    calls = []
    entered, release = threading.Event(), threading.Event()
    class Model(BaseHTTPRequestHandler):
        def log_message(self, *_):
            pass
        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
            prompt = json.dumps(body)
            if 'crash-first' in str(prompt):
                calls.append('first')
                entered.set()
                release.wait(60)
            elif 'crash-second' in str(prompt):
                calls.append('second')
            response = {'choices': [{'message': {'role': 'assistant', 'content': 'Acknowledged'}, 'finish_reason': 'stop'}], 'usage': {'prompt_tokens': 1, 'completion_tokens': 1, 'total_tokens': 2}}
            raw = json.dumps(response).encode()
            self.send_response(200)
            self.send_header('Content-Type', 'application/json')
            self.send_header('Content-Length', str(len(raw)))
            self.end_headers()
            try:
                self.wfile.write(raw)
            except (BrokenPipeError, ConnectionResetError):
                pass
    model = ThreadingHTTPServer(('127.0.0.1', 0), Model)
    threading.Thread(target=model.serve_forever, daemon=True).start()
    process = None
    with tempfile.TemporaryDirectory(prefix='mu-deployment-') as work:
        origin = 'http://127.0.0.1:' + str(port())
        env = {'HOME': work, 'PATH': os.defpath, 'OPENAI_BASE_URL': f'http://127.0.0.1:{model.server_port}/v1', 'OPENAI_API_KEY': 'local-test', 'OPENAI_MODEL': 'test-model', 'AI_PROVIDER': 'local', 'BLOG_DIGESTS': 'off'}
        log = open(Path(work) / 'runtime.log', 'w+')
        def start():
            nonlocal process
            process = subprocess.Popen([binary, '--serve', '--address', origin.removeprefix('http://')], env=env, cwd=work, stdout=log, stderr=log)
            eventually(lambda: urllib.request.urlopen(origin + '/setup', timeout=2).status == 200)
        cookies = http.cookiejar.CookieJar()
        web = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(cookies))
        def browser(path, body=None, form=False):
            headers = {'Origin': origin, 'Accept': 'application/json'}
            if body is not None:
                headers['Content-Type'] = 'application/x-www-form-urlencoded' if form else 'application/json'
                headers['X-CSRF-Token'] = next((c.value for c in cookies if c.name == 'csrf_token'), '')
                body = urllib.parse.urlencode(body).encode() if form else json.dumps(body).encode()
            return web.open(urllib.request.Request(origin + path, data=body, headers=headers), timeout=10)
        token = ''
        def call(endpoint, body):
            exchange = str(uuid.uuid4())
            headers = {'Authorization': 'Bearer ' + token, 'Micro-Protocol': 'mucp/1', 'Micro-Service': 'assistant', 'Micro-Endpoint': endpoint, 'Micro-Id': exchange, 'Content-Type': 'application/json'}
            with urllib.request.urlopen(urllib.request.Request(origin + '/', data=json.dumps(body).encode(), headers=headers), timeout=10) as response:
                assert response.headers['Micro-Id'] == exchange
                return json.load(response)
        try:
            start()
            browser('/setup', {'username': 'deployment', 'password': str(uuid.uuid4()), 'provider': 'later'}, True).close()
            browser('/account/tokens').close()
            with browser('/account/tokens', {'name': 'deployment', 'client': 'api', 'permissions': ['read', 'write', 'api:agent']}) as response:
                credential = json.load(response)
                token = credential['token']
            eventually(lambda: call('Assistant.ListAgents', {}))
            def submission(prompt):
                return {'messageId': str(uuid.uuid4()), 'request': {'prompt': prompt, 'mode': 'ACCESS_MODE_PERSONAL'}}
            first = submission('crash-first: reply Acknowledged without tools')
            call('Assistant.Submit', first)  # Discard acceptance, as with a lost response.
            operation = call('Assistant.Submit', first)
            assert entered.wait(20), call('Assistant.ReadOperation', {'id': operation['id']})
            second = submission('crash-second: reply Acknowledged without tools')
            # Concurrent identical submissions must accept one durable operation.
            with concurrent.futures.ThreadPoolExecutor(max_workers=4) as pool:
                copies = list(pool.map(lambda _: call('Assistant.Submit', second), range(4)))
            assert len({x['id'] for x in copies}) == 1
            queued = copies[0]
            assert queued['state'] == 'OPERATION_STATE_QUEUED', queued
            process.kill()
            process.wait(timeout=10)
            release.set()
            start()
            interrupted = call('Assistant.ReadOperation', {'id': operation['id']})
            assert interrupted['state'] == 'OPERATION_STATE_INTERRUPTED', interrupted
            assert call('Assistant.Submit', first)['id'] == operation['id']
            def completed():
                value = call('Assistant.ReadOperation', {'id': queued['id']})
                return value if value['state'] not in ('OPERATION_STATE_QUEUED', 'OPERATION_STATE_RUNNING') else None
            answer = eventually(completed)
            assert answer['state'] == 'OPERATION_STATE_SUCCEEDED', answer
            assert answer['result']['text'] == 'Acknowledged', answer
            assert calls == ['first', 'second'], calls
            browser('/account/tokens').close()
            browser('/account/tokens?id=' + credential['id'], {'_method': 'DELETE'}, True).close()
            try:
                call('Assistant.ReadOperation', {'id': queued['id']})
            except urllib.error.HTTPError as error:
                assert error.code == 401, error.code
            else:
                raise AssertionError('Revoked credential still reads an operation')
            print('PASS release binary: concurrent deduplication, interrupted work is not replayed, queued work resumes after SIGKILL, revoked credentials are denied')
        except Exception:
            log.flush()
            log.seek(0)
            # Disposable fixture only; never contains real credentials or user data.
            print(log.read()[-2000:], file=sys.stderr)
            app_log = Path(work) / '.mu/logs/mu.log'
            if app_log.exists():
                print(app_log.read_text()[-6000:], file=sys.stderr)
            raise
        finally:
            release.set()
            if process and process.poll() is None:
                process.kill()
                process.wait(timeout=10)
            log.close()
            model.shutdown()


if __name__ == '__main__':
    main(str(Path(sys.argv[1]).resolve()))
