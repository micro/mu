# SPDX-License-Identifier: Apache-2.0
from __future__ import annotations

import asyncio
import http.client
import json
import time
import urllib.error
import urllib.parse
import urllib.request
import uuid
from typing import Any, Callable, cast
from .types import Agents, Answer, History, Operation, Submission, Threads


class ProtocolError(Exception):
    def __init__(self, message: str, status: int = 0, thread: str = ''):
        super().__init__(message)
        self.status = status
        self.thread = thread
        self.submission: Submission | None = None
        self.operation: Operation | None = None


class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class Client:
    """Generic MUCP binding. HTTPS except loopback; no redirects or retries.

    Blocking standard-library I/O runs in a worker thread. Cancelling an await
    does not cancel accepted server work or immediately stop that worker.
    """
    def __init__(self, server: str, token: str | Callable[[], str] = '', *, timeout: float = 20):
        u = urllib.parse.urlsplit(server)
        if (u.username or u.password or u.query or u.fragment or u.path not in ('', '/') or not u.hostname
                or (u.scheme != 'https' and not (u.scheme == 'http' and u.hostname in ('localhost', '127.0.0.1', '::1')))):
            raise ProtocolError('Use an HTTPS origin, or HTTP on loopback, without credentials or a path.')
        if not 0 < timeout < float('inf'):
            raise ProtocolError('Timeout must be positive.')
        self.server = urllib.parse.urlunsplit((u.scheme, u.netloc, '', '', ''))
        self.token = token
        self.timeout = timeout

    async def discover(self) -> dict[str, Any]:
        return await asyncio.to_thread(self._exchange, 'GET', '', '', None)

    async def call(self, service: str, endpoint: str, body: Any = None) -> dict[str, Any]:
        return await asyncio.to_thread(self._exchange, 'POST', service, endpoint, {} if body is None else body)

    def _exchange(self, method: str, service: str, endpoint: str, body: Any) -> dict[str, Any]:
        request_id = str(uuid.uuid4())
        headers = {'Micro-Protocol': 'mucp/1'}
        if method == 'POST':
            headers.update({'Micro-Service': service, 'Micro-Endpoint': endpoint, 'Micro-Id': request_id, 'Content-Type': 'application/json'})
        token = self.token() if callable(self.token) else self.token
        if token:
            if any(ord(c) < 33 or ord(c) > 126 for c in token):
                raise ProtocolError('Invalid token characters.')
            headers['Authorization'] = 'Bearer ' + token
        payload = json.dumps(body, ensure_ascii=False, allow_nan=False).encode('utf-8') if method == 'POST' else None
        if payload is not None and len(payload) > 65536:
            raise ProtocolError('Request exceeds 65536 bytes.')
        request = urllib.request.Request(self.server + '/', data=payload, headers=headers, method=method)
        opener = urllib.request.build_opener(_NoRedirect())
        try:
            try:
                response = opener.open(request, timeout=self.timeout)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                raw = response.read(4 * 1024 * 1024 + 1)
                status, rh = response.status, response.headers
        except (OSError, urllib.error.URLError, http.client.HTTPException):
            raise ProtocolError('Connection failed; the outcome may be unknown.') from None
        if len(raw) > 4 * 1024 * 1024:
            raise ProtocolError('Response exceeds client limit.')
        try:
            data = json.loads(raw)
        except (ValueError, UnicodeError):
            raise ProtocolError('Invalid JSON response.', status) from None
        if not 200 <= status < 300:
            detail = data.get('error') if isinstance(data, dict) else None
            message = detail if isinstance(detail, str) else detail.get('message') if isinstance(detail, dict) else None
            raise ProtocolError(message or 'Request failed.', status, rh.get('Micro-Thread', ''))
        if (rh.get('Micro-Protocol') != 'mucp/1'
                or (method == 'POST' and any(rh.get(key) != headers[key] for key in ('Micro-Id', 'Micro-Service', 'Micro-Endpoint')))
                or rh.get_content_type() != 'application/json' or not isinstance(data, dict)):
            raise ProtocolError('Invalid MUCP response.')
        return data


class Mu:
    """Mu's composition over the generic client. Service calls use call."""
    def __init__(self, server: str | Client, token: str | Callable[[], str] = '', *, timeout: float = 20):
        self._client = server if isinstance(server, Client) else Client(server, token, timeout=timeout)

    async def call(self, service: str, endpoint: str, body: Any = None) -> dict[str, Any]:
        return await self._client.call(service, endpoint, body)

    async def discover(self) -> dict[str, Any]:
        return await self._client.discover()

    async def agents(self) -> Agents:
        return cast(Agents, await self.call('assistant', 'Assistant.ListAgents'))

    async def threads(self, *, agent_id: str = '', page_token: str = '', page_size: int = 25) -> Threads:
        return cast(Threads, await self.call('assistant', 'Assistant.ListThreads', {'agentId': agent_id, 'pageToken': page_token, 'pageSize': page_size}))

    async def read(self, thread: str, *, page_token: str = '') -> History:
        return cast(History, await self.call('assistant', 'Assistant.ReadThread', {'thread': thread, 'pageToken': page_token}))

    async def home(self) -> dict[str, Any]:
        return await self.call('assistant', 'Home.Overview')

    async def inbox(self, *, page_token: str = '') -> dict[str, Any]:
        return await self.call('assistant', 'Inbox.List', {'pageToken': page_token})

    async def services(self, *, page_token: str = '') -> dict[str, Any]:
        return await self.call('assistant', 'Services.List', {'pageToken': page_token})

    def _prepare(self, prompt: str, *, agent_id: str = '', thread: str = '', voice: bool = False) -> Submission:
        if not prompt.strip() or len(prompt.encode('utf-8')) > 8000:
            raise ProtocolError('Prompt must contain 1–8000 UTF-8 bytes.')
        return {'messageId': str(uuid.uuid4()), 'request': {'agentId': agent_id, 'thread': thread, 'prompt': prompt, 'voice': voice, 'mode': 'ACCESS_MODE_PERSONAL'}}

    async def _submit(self, submission: Submission) -> Operation:
        try:
            return cast(Operation, await self.call('assistant', 'Assistant.Submit', submission))
        except ProtocolError as error:
            error.submission = submission
            raise

    async def status(self, operation_id: str) -> Operation:
        return cast(Operation, await self.call('assistant', 'Assistant.ReadOperation', {'id': operation_id}))

    async def wait(self, operation: Operation, *, timeout: float = 120) -> Answer:
        if not 0 < timeout < float('inf'):
            raise ProtocolError('Timeout must be positive.')
        deadline, delay = time.monotonic() + timeout, 1.0
        operation_id, thread = operation.get('id'), operation.get('thread', '')
        try:
            while True:
                if not operation_id or not thread or operation.get('id') != operation_id or operation.get('thread') != thread:
                    raise ProtocolError('Operation identity changed.')
                state = operation.get('state')
                if state == 'OPERATION_STATE_SUCCEEDED' and isinstance(operation.get('result'), dict):
                    return operation['result']
                if state not in ('OPERATION_STATE_QUEUED', 'OPERATION_STATE_RUNNING'):
                    raise ProtocolError(operation.get('error') or 'Operation stopped or returned an unknown state.', 500, thread)
                remaining = deadline - time.monotonic()
                if remaining <= 0:
                    raise ProtocolError('Request is still running; recover it using its operation ID.', 504, thread)
                await asyncio.sleep(min(delay, remaining))
                if time.monotonic() >= deadline:
                    raise ProtocolError('Request is still running; recover it using its operation ID.', 504, thread)
                next_operation = await self.status(operation_id)
                if next_operation.get('id') != operation_id or next_operation.get('thread') != thread:
                    raise ProtocolError('Operation identity changed.')
                operation = next_operation
                delay = min(delay * 2, 30)
        except ProtocolError as error:
            error.thread, error.operation = thread, operation
            raise

    async def ask(self, prompt: str, *, agent_id: str = '', thread: str = '', voice: bool = False) -> Answer:
        submission = self._prepare(prompt, agent_id=agent_id, thread=thread, voice=voice)
        try:
            return await self.wait(await self._submit(submission))
        except ProtocolError as error:
            error.submission = submission
            raise
