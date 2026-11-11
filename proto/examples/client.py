#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Independent MUCP client: Python standard library, no Mu or protobuf dependency.

MU_TOKEN=... python3 proto/examples/client.py https://micro.mu assistant Home.Overview '{}'
Read request JSON from stdin with '-' to keep private text out of shell history.
"""
import json
import os
import sys
import urllib.error
import urllib.parse
import urllib.request
import uuid

class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, *args, **kwargs):
        return None

def call(origin, service, endpoint, body, token):
    url = urllib.parse.urlsplit(origin)
    if (url.scheme != 'https' and not (url.scheme == 'http' and url.hostname in ('localhost', '127.0.0.1', '::1'))) or url.username or url.password or url.query or url.fragment or url.path not in ('', '/'):
        raise ValueError('Use an HTTPS runtime origin (HTTP allowed on loopback).')
    request_id = str(uuid.uuid4())
    request = urllib.request.Request(origin.rstrip('/') + '/', json.dumps(body).encode(), {
        'Content-Type': 'application/json', 'Authorization': 'Bearer ' + token,
        'Micro-Protocol': 'mucp/1', 'Micro-Service': service,
        'Micro-Endpoint': endpoint, 'Micro-Id': request_id,
    }, method='POST')
    with urllib.request.build_opener(NoRedirect()).open(request, timeout=135) as response:
        if response.headers.get('Micro-Protocol') != 'mucp/1' or response.headers.get('Micro-Id') != request_id:
            raise ValueError('Invalid protocol or correlation header')
        data = response.read(4 * 1024 * 1024 + 1)
        if len(data) > 4 * 1024 * 1024:
            raise ValueError('Response too large')
        return json.loads(data)

if __name__ == '__main__':
    if len(sys.argv) != 5:
        sys.exit('Usage: client.py ORIGIN SERVICE ENDPOINT JSON_OR_DASH; set MU_TOKEN')
    try:
        raw = sys.stdin.read() if sys.argv[4] == '-' else sys.argv[4]
        print(json.dumps(call(*sys.argv[1:4], json.loads(raw), os.environ['MU_TOKEN']), indent=2))
    except urllib.error.HTTPError as error:
        sys.exit(f'HTTP {error.code}: {error.read(4096).decode(errors="replace")}')
