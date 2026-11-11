// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import {Mu} from '../dist/index.js';
const mu = new Mu({server: process.argv[2], token: process.env.MU_TOKEN});
await mu.protocol.discover();
await mu.agents();
const submission = mu.prepare('Acknowledge this interoperability test.');
const first = await mu.submit(submission);
const second = await mu.submit(submission);
assert.equal(first.id, second.id);
assert.equal(first.thread, second.thread);
const result = await mu.wait(second, 10000);
assert.equal(result.text, 'Acknowledged');
const recovered = await mu.status(first.id);
assert.equal(recovered.result.text, result.text);
console.log('PASS TypeScript SDK: discovery, delegation, duplicate acceptance and recovery');
