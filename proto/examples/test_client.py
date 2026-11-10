"""Independent binding conformance; fake server has no Mu implementation imports."""
import json
import threading
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer
from client import call

class BindingTest(unittest.TestCase):
    def test_independent_client(self):
        case = self
        class Runtime(BaseHTTPRequestHandler):
            def do_POST(self):
                case.assertEqual(self.path, '/')
                case.assertEqual(self.headers['Micro-Service'], 'assistant')
                case.assertEqual(self.headers['Micro-Endpoint'], 'Home.Overview')
                case.assertEqual(self.headers['Authorization'], 'Bearer test-only')
                case.assertEqual(json.loads(self.rfile.read(int(self.headers['Content-Length']))), {})
                self.send_response(200)
                self.send_header('Micro-Protocol', 'mucp/1')
                self.send_header('Micro-Id', self.headers['Micro-Id'])
                self.end_headers()
                self.wfile.write(b'{"agents":[],"futureField":true}')
            def log_message(self, *args):
                pass
        server = HTTPServer(('127.0.0.1', 0), Runtime)
        worker = threading.Thread(target=server.serve_forever, daemon=True)
        worker.start()
        try:
            response = call(f'http://127.0.0.1:{server.server_port}', 'assistant', 'Home.Overview', {}, 'test-only')
            self.assertEqual(response['agents'], [])
        finally:
            server.shutdown()
            server.server_close()
            worker.join()

if __name__ == '__main__':
    unittest.main()
