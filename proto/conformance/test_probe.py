# SPDX-License-Identifier: Apache-2.0
import contextlib
import io
import json
import os
from pathlib import Path
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from unittest.mock import patch

from probe import delegate, exchange, origin_parts, receipt


class ProbeTest(unittest.TestCase):
    def test_origin_credentials_and_redirect_boundary(self):
        for origin in ('http://example.org', 'https://user:pass@example.org',
                       'https://example.org/private', 'https://example.org/?token=secret'):
            with self.assertRaises(ValueError):
                origin_parts(origin)

    def test_receipt_created_before_network_and_never_overwritten(self):
        with tempfile.TemporaryDirectory() as directory:
            path = str(Path(directory) / 'receipt.json')
            saved = receipt(path, 'https://example.org', 'chosen', False)
            self.assertEqual(os.stat(path).st_mode & 0o777, 0o600)
            self.assertEqual(saved, receipt(path, 'https://example.org', '', True))
            with self.assertRaises(FileExistsError):
                receipt(path, 'https://example.org', '', False)
            with self.assertRaises(ValueError):
                receipt(path, 'https://different.example', '', True)
            with patch('probe.time.time', return_value=saved['created'] + 7 * 86400):
                with self.assertRaises(ValueError):
                    receipt(path, 'https://example.org', '', True)

    def test_lost_receipt_and_new_process_resume_use_same_identity(self):
        # A separate test peer validates client recovery behaviour. The Go test
        # executes the wire suite against Mu; this peer is not proof of Mu's scheduler.
        submissions = []
        mode = {'redirect': False, 'wrong_id': False, 'state': 'OPERATION_STATE_SUCCEEDED'}
        class Peer(BaseHTTPRequestHandler):
            def do_POST(self):
                body = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                endpoint = self.headers['Micro-Endpoint']
                if mode['redirect']:
                    self.send_response(307)
                    self.send_header('Location', '/must-not-follow')
                    self.end_headers()
                    return
                if endpoint == 'Assistant.Submit':
                    submissions.append(body)
                self.send_response(200)
                for header in ('Micro-Protocol', 'Micro-Service', 'Micro-Endpoint', 'Micro-Id'):
                    self.send_header(header, 'wrong-correlation' if header == 'Micro-Id' and mode['wrong_id'] else self.headers[header])
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                try:
                    self.wfile.write(json.dumps({'id':'operation-1','thread':'thread-1',
                        'state':mode['state'],'result':{'text':'Acknowledged'}}).encode())
                except (BrokenPipeError, ConnectionResetError):
                    pass  # Probe deliberately discarded acceptance body.
            def log_message(self, *args):
                pass
        server = HTTPServer(('127.0.0.1', 0), Peer)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        origin = f'http://127.0.0.1:{server.server_port}'
        try:
            with tempfile.TemporaryDirectory() as directory, contextlib.redirect_stdout(io.StringIO()):
                path = str(Path(directory) / 'receipt.json')
                delegate(origin, 'test-only', path, '', False, 1)
                delegate(origin, 'test-only', path, '', True, 1)
                self.assertEqual(len(submissions), 5)
                self.assertTrue(all(s == submissions[0] for s in submissions))
                mode['state'] = 'OPERATION_STATE_INTERRUPTED'
                with self.assertRaisesRegex(ValueError, 'INTERRUPTED'):
                    delegate(origin, 'test-only', path, '', True, 1)
                mode['wrong_id'] = True
                with self.assertRaisesRegex(ValueError, 'correlation'):
                    exchange(origin, token='test-only')
                mode['redirect'] = True
                status, _, _ = exchange(origin, token='test-only')
                self.assertEqual(status, 307)
        finally:
            server.shutdown()
            server.server_close()
            worker.join()


if __name__ == '__main__':
    unittest.main()
