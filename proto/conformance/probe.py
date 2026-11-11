#!/usr/bin/env python3
# SPDX-License-Identifier: Apache-2.0
"""Independent MUCP probe. Standard library only; never prints account contents/tokens.

Default: public discovery. --wire: bounded negative/read-only wire checks.
--reads: authenticated agent/service discovery. --delegate RECEIPT: one durable
agent request (may cost credits), requiring --test-account. --resume RECEIPT:
recover that exact request after a disconnect/restart, never allocate a new ID.
"""
import argparse
import http.client
import json
import os
from pathlib import Path
import sys
import time
import urllib.parse
import urllib.request
import uuid

MAX_RESPONSE = 4 * 1024 * 1024


def origin_parts(origin):
    u = urllib.parse.urlsplit(origin)
    if (u.scheme != 'https' and not (u.scheme == 'http' and u.hostname in ('localhost', '127.0.0.1', '::1'))) or not u.hostname or u.username or u.password or u.query or u.fragment or u.path not in ('', '/'):
        raise ValueError('Use an HTTPS origin; HTTP is allowed only on loopback')
    return u


def exchange(origin, method='POST', body='{}', token='', changes=None,
             duplicates=(), discard=False, timeout=20):
    u = origin_parts(origin)
    headers = {'Micro-Protocol': 'mucp/1'}
    if method != 'GET':
        headers.update({'Micro-Service': 'assistant', 'Micro-Endpoint': 'Assistant.ListAgents',
                        'Micro-Id': str(uuid.uuid4()), 'Content-Type': 'application/json'})
    if token:
        headers['Authorization'] = 'Bearer ' + token
    for key, value in (changes or {}).items():
        if value is None:
            headers.pop(key, None)
        else:
            headers[key] = value
    payload = body.encode('utf-8') if method != 'GET' else b''
    connection = (http.client.HTTPSConnection if u.scheme == 'https' else http.client.HTTPConnection)(u.hostname, u.port, timeout=timeout)
    proxy = urllib.request.getproxies().get('https') if u.scheme == 'https' and not urllib.request.proxy_bypass(u.hostname) else None
    if proxy:
        proxy_url = urllib.parse.urlsplit(proxy)
        if proxy_url.scheme != 'http' or not proxy_url.hostname or proxy_url.username or proxy_url.password:
            raise ValueError('Probe supports unauthenticated HTTP CONNECT proxies only')
        connection = http.client.HTTPSConnection(proxy_url.hostname, proxy_url.port or 80, timeout=timeout)
        connection.set_tunnel(u.hostname, u.port or 443)
    try:
        # http.client does not follow redirects, and preserves duplicate lines.
        connection.putrequest(method, '/')
        for key, value in headers.items():
            connection.putheader(key, value)
        for key in duplicates:
            connection.putheader(key, headers[key])
        connection.putheader('Content-Length', str(len(payload)))
        connection.endheaders(payload)
        response = connection.getresponse()
        status = response.status
        rh = {k.lower(): v for k, v in response.getheaders()}
        if discard:
            # Deliberately lose the acceptance body, then recover using messageId.
            return status, rh, None
        raw = response.read(MAX_RESPONSE + 1)
        if len(raw) > MAX_RESPONSE:
            raise ValueError('Response exceeds probe limit')
        if status == 200:
            if rh.get('micro-protocol') != 'mucp/1':
                raise ValueError('Wrong response protocol')
            if method != 'GET':
                for key in ('Micro-Id', 'Micro-Service', 'Micro-Endpoint'):
                    if rh.get(key.lower()) != headers[key]:
                        raise ValueError('Wrong response routing/correlation')
            if rh.get('content-type', '').split(';')[0].strip() != 'application/json':
                raise ValueError('Wrong response media type')
        try:
            value = json.loads(raw)
        except (ValueError, UnicodeError):
            value = None
        if status == 200 and not isinstance(value, dict):
            raise ValueError('Successful response is not a JSON object')
        return status, rh, value
    finally:
        connection.close()


def discovery(origin):
    status, _, d = exchange(origin, 'GET')
    if status != 200:
        raise ValueError(f'Discovery HTTP {status}')
    if d.get('protocol') != 'mucp/1' or d.get('encoding') != 'protobuf-json':
        raise ValueError('Incompatible discovery')
    if not all(isinstance(d.get(k), str) and d[k] for k in ('service', 'contract')):
        raise ValueError('Missing primary contract')
    if not isinstance(d.get('operations'), list) or not all(isinstance(x, str) and x for x in d['operations']):
        raise ValueError('Invalid operations')
    for k, typ in (('features', bool), ('limits', int)):
        if k in d and (not isinstance(d[k], dict) or not all(type(v) is typ and (typ is bool or v >= 0) for v in d[k].values())):
            raise ValueError(f'Invalid {k}')
    print('PASS HTTP-04 public discovery')
    return d


def wire(origin, token):
    fixtures = json.loads(Path(__file__).with_name('wire.json').read_text())
    count = 0
    for case in fixtures:
        if case.get('authenticated') and not token:
            continue
        body = case.get('body', '{}')
        if case.get('paddedBytes'):
            body = '{}' + ' ' * (case['paddedBytes'] - 2)
        if case.get('oversized'):
            body = '{"padding":"' + 'x' * 65536 + '"}'
        status, headers, data = exchange(origin, method=case.get('method', 'POST'),
            body=body, token=token if case.get('authenticated') else '',
            changes=case.get('headers'), duplicates=case.get('duplicates', ()))
        if status != case['status'] or headers.get('micro-error') != case.get('code'):
            raise ValueError(f'{case["id"]}: expected {case["status"]}/{case.get("code")}, got {status}/{headers.get("micro-error")}')
        if case.get('code') and case['code'] != 'authorization_failed':
            error = data.get('error') if isinstance(data, dict) else None
            if not isinstance(error, dict) or error.get('status') != status or error.get('code') != case['code']:
                raise ValueError(f'{case["id"]}: invalid structured error')
        count += 1
    print(f'PASS {count} HTTP wire cases' + ('' if token else ' (authenticated cases not run)'))


def invoke(origin, token, endpoint, body):
    status, _, data = exchange(origin, body=json.dumps(body), token=token,
        changes={'Micro-Endpoint': endpoint})
    if status != 200:
        raise ValueError(f'{endpoint}: HTTP {status}; no automatic action replay')
    return data


def reads(origin, token):
    agents = invoke(origin, token, 'Assistant.ListAgents', {})
    services = invoke(origin, token, 'Services.List', {})
    if not isinstance(agents.get('agents'), list) or not isinstance(services.get('services'), list):
        raise ValueError('Invalid Mu discovery results')
    print('PASS authenticated agent and service discovery (contents withheld)')


def receipt(path, origin, agent_id, resume):
    if resume:
        saved = json.loads(Path(path).read_text())
        if saved['origin'] != origin.rstrip('/'):
            raise ValueError('Receipt belongs to a different origin')
        if time.time() - saved['created'] >= 7 * 86400:
            raise ValueError('Receipt is outside the safe replay window')
        return saved
    saved = {'origin': origin.rstrip('/'), 'created': time.time(), 'submission': {
        'messageId': str(uuid.uuid4()), 'request': {'agentId': agent_id,
        'prompt': 'MUCP interoperability check. Reply with a short acknowledgement. Do not use tools or access personal data.',
        'mode': 'ACCESS_MODE_PERSONAL'}}}
    # Persist the identity before sending. Refuse overwrite; protect the receipt.
    fd = os.open(path, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(fd, 'w') as out:
        json.dump(saved, out)
        out.flush()
        os.fsync(out.fileno())
    return saved


def delegate(origin, token, path, agent_id, resume, wait):
    saved = receipt(path, origin, agent_id, resume)
    submission = saved['submission']
    if not resume:
        # This deliberately omits reading the receipt body, not an assertion that
        # the server has cancelled or that an arbitrary network failure is safe.
        status, _, _ = exchange(origin, body=json.dumps(submission), token=token,
            changes={'Micro-Endpoint': 'Assistant.Submit'}, discard=True)
        if status != 200:
            raise ValueError(f'Submit HTTP {status}; receipt retained for inspection')
    first = invoke(origin, token, 'Assistant.Submit', submission)
    second = invoke(origin, token, 'Assistant.Submit', submission)
    if not first.get('id') or not first.get('thread') or (first['id'], first['thread']) != (second.get('id'), second.get('thread')):
        raise ValueError('Stable submission ID did not recover the same operation')
    print('PASS same-ID receipt recovery after lost acceptance body')
    end = time.monotonic() + wait
    delay = 2
    while True:
        op = invoke(origin, token, 'Assistant.ReadOperation', {'id': first['id']})
        if op.get('id') != first['id'] or op.get('thread') != first['thread']:
            raise ValueError('Recovery changed operation identity')
        state = op.get('state')
        if state == 'OPERATION_STATE_SUCCEEDED':
            result = op.get('result')
            if not isinstance(result, dict) or not isinstance(result.get('text'), str):
                raise ValueError('Success has no answer')
            print('PASS durable agent result (answer withheld)')
            return
        if state in ('OPERATION_STATE_FAILED', 'OPERATION_STATE_INTERRUPTED'):
            raise ValueError(f'Agent ended {state}; receipt retained, no new submission')
        if state not in ('OPERATION_STATE_QUEUED', 'OPERATION_STATE_RUNNING'):
            raise ValueError('Unknown operation state')
        remaining = end - time.monotonic()
        if remaining <= 0:
            raise ValueError('Local wait deadline; server work is not cancelled. Resume with this receipt.')
        time.sleep(min(delay, remaining))
        delay = min(delay * 2, 30)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('origin')
    parser.add_argument('--wire', action='store_true')
    parser.add_argument('--reads', action='store_true')
    group = parser.add_mutually_exclusive_group()
    group.add_argument('--delegate', metavar='RECEIPT')
    group.add_argument('--resume', metavar='RECEIPT')
    parser.add_argument('--test-account', action='store_true')
    parser.add_argument('--agent-id', default='')
    parser.add_argument('--wait', type=int, default=180)
    args = parser.parse_args()
    if not 1 <= args.wait <= 600:
        parser.error('--wait must be 1..600 seconds')
    token = os.environ.get('MU_TOKEN', '')
    if (args.reads or args.delegate or args.resume) and not token:
        parser.error('Set MU_TOKEN for authenticated checks')
    if (args.delegate or args.resume) and not args.test_account:
        parser.error('Agent execution requires --test-account and may spend credits')
    d = discovery(args.origin)
    if args.wire:
        wire(args.origin, token)
    if args.reads:
        reads(args.origin, token)
    if args.delegate or args.resume:
        if d.get('service') != 'assistant' or d.get('features', {}).get('durableSubmission') is not True:
            raise ValueError('Durable assistant submission is not advertised')
        delegate(args.origin, token, args.delegate or args.resume, args.agent_id,
                 bool(args.resume), args.wait)


if __name__ == '__main__':
    try:
        main()
    except (ValueError, KeyError, OSError, http.client.HTTPException) as error:
        # Exception bodies from an HTTP peer are never printed.
        print(f'FAIL {error}', file=sys.stderr)
        sys.exit(1)
