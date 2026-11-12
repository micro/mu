// SPDX-License-Identifier: Apache-2.0
import assert from 'node:assert/strict';
import {Mu} from '../dist/index.js';
const mu = new Mu({server: process.argv[2], token: process.env.MU_TOKEN});
await mu.discover();
await mu.agents();
const call = mu.call.bind(mu);
let accepted;
mu.call = async (service, endpoint, body) => {
  const result = await call(service, endpoint, body);
  if (endpoint === 'Assistant.Submit') {
    const duplicate = await call(service, endpoint, body);
    assert.equal(result.id, duplicate.id);
    assert.equal(result.thread, duplicate.thread);
    accepted = result;
  }
  return result;
};
const result = await mu.ask('Acknowledge this interoperability test.');
assert.equal(result.text, 'Acknowledged');
const recovered = await mu.status(accepted.id);
assert.equal(recovered.result.text, result.text);
console.log('PASS TypeScript SDK: discovery, delegation, duplicate acceptance and recovery');
