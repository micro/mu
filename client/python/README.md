# Mu Python SDK

Async standard-library client for Python 3.11+. Apache-2.0.
The proposed distribution is `micro-mu`, importing as `mu`; it is not yet published
to PyPI. Install this source with `python -m pip install ./client/python`.

```python
import asyncio
import os
from mu import Mu

async def main():
    mu = Mu('https://micro.mu', os.environ['MU_TOKEN'])
    answer = await mu.ask('What is on my calendar?', agent_id='micro')
    print(answer['text'])

asyncio.run(main())
```

Install the SDK in your application and point it at any Mu runtime you have access to,
including a self-hosted server. Use a separate client and credential for each server;
server selection does not imply federated identity or cross-server permissions.

A configured runtime and account credential are required. Agent requests need
`read write api:agent`, use the selected agent's tools and may cost credits.
`agents()`, `threads()`, `read(thread)`, `home()`, `inbox()` and `services()` return
JSON dictionaries with TypedDict annotations for assistant contracts; paging tokens and unknown content are retained. Home, Inbox and
Services require their own `api:` capabilities. The SDK has no runtime dependencies.

`Client` provides generic MUCP discovery and calls. Use
`await mu.call('contacts', 'Contacts.List', {})` for a supported direct
service, with a service-scoped token. Assistant scopes do not grant service access.
Agents, Inbox and service state remain owned by the runtime.

Supply a token or synchronous token callback for host-managed renewal. OAuth login
and credential storage belong to the calling application. HTTP is allowed only on
loopback, redirects are refused, and actions are never automatically retried.
Polling backs off to 30 seconds. Standard-library network I/O runs in worker threads;
cancelling an await or timing out a wait does not cancel server work. An in-flight
network call may run until its request timeout.
