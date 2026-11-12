# SPDX-License-Identifier: Apache-2.0
import asyncio
import os
import sys
from mu import Mu

async def main():
    mu = Mu(sys.argv[1], os.environ['MU_TOKEN'])
    await mu.discover()
    await mu.agents()
    call = mu.call
    accepted = None
    async def duplicate_call(service, endpoint, body=None):
        nonlocal accepted
        result = await call(service, endpoint, body)
        if endpoint == 'Assistant.Submit':
            duplicate = await call(service, endpoint, body)
            assert result['id'] == duplicate['id'] and result['thread'] == duplicate['thread']
            accepted = result
        return result
    mu.call = duplicate_call
    result = await mu.ask('Acknowledge this interoperability test.')
    assert result['text'] == 'Acknowledged'
    recovered = await mu.status(accepted['id'])
    assert recovered['result']['text'] == result['text']
    print('PASS Python SDK: discovery, delegation, duplicate acceptance and recovery')

asyncio.run(main())
