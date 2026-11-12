# SPDX-License-Identifier: Apache-2.0
import asyncio
import os
import sys
from mu import Mu

async def main():
    mu = Mu(sys.argv[1], os.environ['MU_TOKEN'])
    await mu.protocol.discover()
    await mu.agents()
    submission = mu.prepare('Acknowledge this interoperability test.')
    first = await mu.submit(submission)
    second = await mu.submit(submission)
    assert first['id'] == second['id'] and first['thread'] == second['thread']
    result = await mu.wait(second, timeout=10)
    assert result['text'] == 'Acknowledged'
    recovered = await mu.status(first['id'])
    assert recovered['result']['text'] == result['text']
    print('PASS Python SDK: discovery, delegation, duplicate acceptance and recovery')

asyncio.run(main())
