# SPDX-License-Identifier: Apache-2.0
import unittest
from unittest.mock import AsyncMock
from mu import Client, Mu, ProtocolError

class Tests(unittest.IsolatedAsyncioTestCase):
    def test_origins(self):
        for origin in ('http://remote.example', 'https://u:p@host', 'https://host/path', 'https://host?token=x'):
            with self.assertRaises(ProtocolError):
                Client(origin)

    async def test_uncertain_submission(self):
        mu = Mu('https://runtime.example')
        mu.protocol.call = AsyncMock(side_effect=ProtocolError('lost'))
        submission = mu.prepare('hello')
        with self.assertRaises(ProtocolError) as error:
            await mu.submit(submission)
        self.assertIs(error.exception.submission, submission)
        self.assertEqual(mu.protocol.call.await_count, 1)

    async def test_recovery_identity(self):
        mu = Mu('https://runtime.example')
        mu.status = AsyncMock(return_value={'id': 'other', 'thread': 'thread', 'state': 'OPERATION_STATE_SUCCEEDED', 'result': {}})
        with self.assertRaisesRegex(ProtocolError, 'identity'):
            await mu.wait({'id': 'one', 'thread': 'thread', 'state': 'OPERATION_STATE_RUNNING'}, timeout=2)

    async def test_terminal_and_content(self):
        mu = Mu('https://runtime.example')
        result = {'text': 'ok', 'content': [{'fallback': 'new', 'future': 1}]}
        self.assertIs(await mu.wait({'id': 'one', 'thread': 'thread', 'state': 'OPERATION_STATE_SUCCEEDED', 'result': result}), result)
        with self.assertRaises(ProtocolError):
            await mu.wait({'id': 'one', 'thread': 'thread', 'state': 'OPERATION_STATE_INTERRUPTED'})


class TransportTests(unittest.IsolatedAsyncioTestCase):
    async def test_wire_guards_and_no_redirects(self):
        import json
        import threading
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
        calls = []
        mode = ['good']
        response_body = [{'ok': True}]
        received = []
        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass
            def do_POST(self):
                calls.append(self.path)
                received.append(json.loads(self.rfile.read(int(self.headers['Content-Length']))))
                if mode[0] == 'redirect':
                    self.send_response(307)
                    self.send_header('Location', '/unexpected')
                    self.end_headers()
                    return
                self.send_response(200)
                for key in ('Micro-Protocol', 'Micro-Id', 'Micro-Service', 'Micro-Endpoint'):
                    self.send_header(key, 'wrong' if mode[0] == key else self.headers[key])
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(b'not json' if mode[0] == 'json' else json.dumps(response_body[0]).encode())
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            client = Client(f'http://127.0.0.1:{server.server_port}', 'test-token')
            self.assertEqual(await client.call('contacts', 'Contacts.List'), {'ok': True})
            for bad in ('Micro-Protocol', 'Micro-Id', 'Micro-Service', 'Micro-Endpoint', 'json', 'redirect'):
                mode[0] = bad
                before = len(calls)
                with self.assertRaises(ProtocolError):
                    await client.call('contacts', 'Contacts.List')
                self.assertEqual(len(calls), before + 1)
            self.assertNotIn('/unexpected', calls)
            from pathlib import Path
            fixtures = json.loads((Path(__file__).resolve().parents[3] / 'proto/testdata/exchanges.json').read_text())
            mode[0] = 'good'
            for fixture in fixtures:
                response_body[0] = fixture['response']
                result = await client.call('assistant', 'Assistant.' + fixture['operation'], fixture['request'])
                self.assertEqual(received[-1], fixture['request'])
                self.assertEqual(result, fixture['response'])
        finally:
            server.shutdown()
            server.server_close()
            worker.join()

if __name__ == '__main__':
    unittest.main()
