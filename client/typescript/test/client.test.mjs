// SPDX-License-Identifier: Apache-2.0
import test from 'node:test';
import assert from 'node:assert/strict';
import {Client, Mu, ProtocolError} from '../dist/index.js';
const server = 'https://runtime.example';
function reply(init, body, overrides = {}) {
  return new Response(JSON.stringify(body), {headers: {...Object.fromEntries(init.headers), ...overrides}});
}
test('reject insecure origins and cross-origin cookie credentials', () => {
  for (const value of ['http://remote.example', 'https://u:p@host', 'https://host/path', 'https://host?token=x']) assert.throws(() => new Client({server: value}));
  assert.throws(() => new Client({server, session: {origin: 'https://other.example', csrf: () => ''}}));
});
test('transport rejects wrong routing, correlation, media type', async () => {
  for (const overrides of [{'Micro-Id': 'wrong'}, {'Micro-Service': 'wrong'}, {'Micro-Endpoint': 'wrong'}, {'Micro-Protocol': 'wrong'}, {'Content-Type': 'text/html'}]) {
    const client = new Client({server, fetch: async (_, init) => reply(init, {}, overrides)});
    await assert.rejects(client.call('contacts', 'Contacts.List'), /Invalid MUCP/);
  }
});
test('an ambiguous submission retains its receipt and is never retried', async () => {
  let calls = 0;
  const mu = new Mu({server, token: 'test-token', fetch: async (_, init) => {
    calls++; assert.equal(init.redirect, 'error'); assert.equal(init.credentials, 'omit'); throw Error('lost');
  }});
  const submission = mu.prepare('hello');
  await assert.rejects(mu.submit(submission), e => e instanceof ProtocolError && e.submission === submission);
  assert.equal(calls, 1);
});
test('recovery rejects a changed operation identity and retains the last receipt', async () => {
  const mu = new Mu({server, fetch: async (_, init) => reply(init, {id: 'different', thread: 'thread', state: 'OPERATION_STATE_SUCCEEDED', result: {text: 'wrong'}})});
  await assert.rejects(mu.wait({id: 'one', thread: 'thread', state: 'OPERATION_STATE_RUNNING'}, 2000), e => /identity/.test(e.message) && e.thread === 'thread');
});
test('unknown structured content survives and terminal failure never submits again', async () => {
  const mu = new Mu({server});
  const answer = {text: 'ok', content: [{fallback: 'new content', future: {value: 1}}]};
  assert.equal(await mu.wait({id: 'one', thread: 'thread', state: 'OPERATION_STATE_SUCCEEDED', result: answer}), answer);
  await assert.rejects(mu.wait({id: 'one', thread: 'thread', state: 'OPERATION_STATE_INTERRUPTED'}), /stopped/);
});
