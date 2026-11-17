"""Verify per-PR, per-head CLA acceptance without executing contributor code."""
import hashlib
import json
import os
from pathlib import Path
import re
import urllib.request

CONTEXT = 'cla'


def statement(number, head, digest):
    return (f'I agree to Mu CLA v1.0 (SHA-256 {digest}) '
            f'for PR #{number} at commit {head}.')


def contributors(pr, commits, resolve):
    required = {}
    unresolved = []

    def add(user, source):
        if not user:
            unresolved.append(source)
        elif user.get('type') != 'Bot':
            required[user['id']] = user['login']

    add(pr['user'], 'PR author')
    for commit in commits:
        add(commit.get('author'), f"unmapped author at {commit['sha']}")
        message = commit['commit']['message']
        for line in message.splitlines():
            if not re.match(r'(?i)^co-authored-by\s*:', line):
                continue
            match = re.fullmatch(r'(?i)co-authored-by:\s*.+\s+<([^<>]+)>\s*', line)
            email = match.group(1) if match else ''
            noreply = re.fullmatch(r'(?:(\d+)\+)?([\w-]+(?:\[bot\])?)@users\.noreply\.github\.com', email, re.I)
            if not noreply:
                unresolved.append(f'co-author at {commit["sha"]}: use a GitHub noreply address')
                continue
            user = resolve(noreply.group(2))
            if noreply.group(1) and str(user['id']) != noreply.group(1):
                unresolved.append(f'co-author ID mismatch at {commit["sha"]}')
                continue
            add(user, 'co-author')
    if not required:
        unresolved.append('A human contributor must take responsibility for this PR')
    return required, unresolved


def evaluate(pr, commits, comments, digest, resolve):
    required, unresolved = contributors(pr, commits, resolve)
    expected = statement(pr['number'], pr['head']['sha'], digest)
    signed = {c['user']['id']: c['html_url'] for c in comments
              if c['user'].get('type') != 'Bot' and c['body'].strip() == expected}
    missing = [login for uid, login in required.items() if uid not in signed]
    evidence = {login: signed[uid] for uid, login in required.items() if uid in signed}
    return missing, unresolved, evidence, expected


class GitHub:
    def __init__(self):
        self.token = os.environ['GITHUB_TOKEN']
        self.root = 'https://api.github.com'

    def request(self, path, data=None):
        req = urllib.request.Request(self.root + path,
            data=None if data is None else json.dumps(data).encode(),
            headers={'Authorization': 'Bearer ' + self.token,
                     'Accept': 'application/vnd.github+json',
                     'Content-Type': 'application/json',
                     'X-GitHub-Api-Version': '2022-11-28'})
        with urllib.request.urlopen(req, timeout=30) as response:
            return json.load(response)

    def pages(self, path):
        result = []
        for page in range(1, 101):
            items = self.request(f'{path}?per_page=100&page={page}')
            result.extend(items)
            if len(items) < 100:
                return result
        raise RuntimeError('Pagination limit exceeded; refusing incomplete evidence')


def main():
    event = json.loads(Path(os.environ['GITHUB_EVENT_PATH']).read_text())
    number = (event.get('pull_request') or event.get('issue') or {}).get('number')
    if not number or ('issue' in event and 'pull_request' not in event['issue']):
        return
    api = GitHub()
    repo = os.environ['GITHUB_REPOSITORY']
    prefix = f'/repos/{repo}'
    pr = api.request(f'{prefix}/pulls/{number}')
    head = pr['head']['sha']
    target = f'https://github.com/{repo}/actions/runs/{os.environ["GITHUB_RUN_ID"]}'

    def status(state, description):
        api.request(f'{prefix}/statuses/{head}', dict(state=state, context=CONTEXT,
                    description=description[:140], target_url=target))

    status('pending', 'Checking contributor acceptance')
    try:
        def shared_heads():
            # /pulls defaults to open; paginate the entire repository, not one fork.
            return sorted(p['number'] for p in api.pages(f'{prefix}/pulls')
                          if p['head']['sha'] == head)

        numbers = shared_heads()
        if pr['state'] == 'open' and number not in numbers:
            raise RuntimeError('Triggering PR moved or closed; rerun required')
        digest = hashlib.sha256(Path('CLA.md').read_bytes()).hexdigest()
        summary = ['## CLA acceptance', '',
                   f'Agreement: https://github.com/{repo}/blob/{os.environ["TRUSTED_SHA"]}/CLA.md']
        failed = False
        for shared_number in numbers:
            shared = api.request(f'{prefix}/pulls/{shared_number}')
            if shared['state'] != 'open' or shared['head']['sha'] != head:
                raise RuntimeError('Shared PR changed during verification; rerun required')
            commits = api.pages(f'{prefix}/pulls/{shared_number}/commits')
            # GitHub caps this endpoint at 250 commits; never omit authors.
            if len(commits) != shared['commits'] or shared['commits'] >= 250:
                raise RuntimeError('Commit list incomplete or too large; split this PR')
            comments = api.pages(f'{prefix}/issues/{shared_number}/comments')
            missing, unresolved, evidence, expected = evaluate(
                shared, commits, comments, digest, lambda login: api.request('/users/' + login))
            failed = failed or bool(missing or unresolved)
            summary.extend(['', f'### PR #{shared_number}', '',
                            'Each contributor must post this exact sentence:', '', expected, '',
                            'Missing: ' + (', '.join(missing) or 'none'),
                            'Unresolved: ' + ('; '.join(unresolved) or 'none'), '',
                            'Acceptance evidence:', json.dumps(evidence, indent=2)])
        # A status authorizes all PRs at this SHA. Refuse a changing population.
        if shared_heads() != numbers:
            raise RuntimeError('PRs sharing this commit changed; rerun required')
        with open(os.environ['GITHUB_STEP_SUMMARY'], 'a') as out:
            out.write('\n'.join(summary) + '\n')
        if failed:
            status('failure', 'CLA acceptance missing in a PR sharing this commit; see details')
            raise SystemExit(1)
        status('success', 'All identified human contributors accepted this agreement and head')
    except Exception as exc:
        status('error', 'CLA verification could not complete; see workflow logs')
        raise RuntimeError('CLA verification failed') from exc


if __name__ == '__main__':
    main()
