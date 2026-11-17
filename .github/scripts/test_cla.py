import unittest
from cla import evaluate, statement


def user(uid, login, kind='User'):
    return dict(id=uid, login=login, type=kind)


class CLATest(unittest.TestCase):
    def setUp(self):
        self.alice = user(1, 'alice')
        self.bob = user(2, 'bob')
        self.pr = dict(number=10, user=self.alice, head={'sha': 'abc'})
        self.commits = [dict(sha='abc', author=self.alice, commit={'message': 'change'})]
        self.comment = dict(user=self.alice, body=statement(10, 'abc', 'digest'), html_url='evidence')

    def check(self, comments=None, commits=None):
        return evaluate(self.pr, self.commits if commits is None else commits,
                        [self.comment] if comments is None else comments,
                        'digest', lambda login: self.bob)

    def test_acceptance(self):
        missing, unresolved, evidence, _ = self.check()
        self.assertEqual((missing, unresolved, evidence), ([], [], {'alice': 'evidence'}))

    def test_missing_or_quoted_or_other_agreement(self):
        for body in ['', '> ' + self.comment['body'], self.comment['body'].replace('digest', 'other'),
                     self.comment['body'].replace('#10', '#11')]:
            self.assertEqual(self.check([dict(self.comment, body=body)])[0], ['alice'])

    def test_new_head_requires_acceptance(self):
        self.pr['head']['sha'] = 'new'
        self.assertEqual(self.check()[0], ['alice'])

    def test_other_person_cannot_accept(self):
        self.assertEqual(self.check([dict(self.comment, user=self.bob)])[0], ['alice'])

    def test_coauthor_must_accept(self):
        self.commits[0]['commit']['message'] += '\nCo-authored-by: Bob <2+bob@users.noreply.github.com>'
        self.assertEqual(self.check()[0], ['bob'])

    def test_coauthor_malformed_or_unmapped_fails(self):
        for trailer in ['Co-authored-by: Bob <bob@example.org>', 'Co-authored-by: Bob',
                        'Co-authored-by: Bob <3+bob@users.noreply.github.com>']:
            self.commits[0]['commit']['message'] = trailer
            self.assertTrue(self.check()[1])

    def test_unmapped_author_fails(self):
        self.commits[0]['author'] = None
        self.assertTrue(self.check()[1])

    def test_pr_author_and_other_commit_author(self):
        self.commits[0]['author'] = self.bob
        self.assertEqual(self.check()[0], ['bob'])

    def test_bot_does_not_sign_for_human(self):
        bot = user(3, 'automation[bot]', 'Bot')
        self.assertEqual(self.check([dict(self.comment, user=bot)])[0], ['alice'])

    def test_bot_pr_requires_human(self):
        bot = user(3, 'automation[bot]', 'Bot')
        self.pr['user'] = bot
        self.commits[0]['author'] = bot
        self.assertTrue(self.check([])[1])

    def test_deleted_acceptance_fails(self):
        self.assertEqual(self.check([])[0], ['alice'])


if __name__ == '__main__':
    unittest.main()

class WorkflowTest(unittest.TestCase):
    def test_api_pagination(self):
        from cla import GitHub
        from unittest.mock import Mock
        api = GitHub.__new__(GitHub)
        api.request = Mock(side_effect=[[{'id': n} for n in range(100)], [{'id': 100}]])
        self.assertEqual(len(api.pages('/comments')), 101)
        self.assertIn('page=2', api.request.call_args.args[0])

    def test_main_statuses_and_head_race(self):
        import hashlib
        import json
        import os
        from pathlib import Path
        import tempfile
        from unittest.mock import patch
        from cla import main
        for race, signed, peer in [(False, True, None), (False, False, None),
                                   (True, True, None), (False, True, False),
                                   (False, True, True), (False, True, 'appeared')]:
            with self.subTest(race=race, signed=signed, peer=peer), tempfile.TemporaryDirectory() as tmp:
                event = Path(tmp) / 'event.json'
                event.write_text(json.dumps({'pull_request': {'number': 10}}))
                alice = user(1, 'alice')
                pr = dict(number=10, user=alice, head={'sha': 'abc'}, state='open', commits=1)
                digest = hashlib.sha256(Path('CLA.md').read_bytes()).hexdigest()
                comments = [dict(user=alice, body=statement(10, 'abc', digest), html_url='evidence')] if signed else []
                statuses = []

                class API:
                    lists = 0
                    def request(self, path, data=None):
                        if data is not None:
                            statuses.append(data['state'])
                            return {}
                        if statuses and race:
                            return dict(pr, head={'sha': 'new'})
                        return dict(pr, number=11) if path.endswith('/11') else pr

                    def pages(self, path):
                        if path.endswith('/pulls'):
                            self.lists += 1
                            include = peer is not None and (peer != 'appeared' or self.lists > 1)
                            return [pr, dict(pr, number=11)] if include else [pr]
                        if path.endswith('/commits'):
                            return [dict(sha='abc', author=alice, commit={'message': 'change'})]
                        if '/issues/11/' in path:
                            return [dict(comments[0], body=statement(11, 'abc', digest))] if peer is True else []
                        return comments

                env = {'GITHUB_EVENT_PATH': str(event), 'GITHUB_REPOSITORY': 'micro/mu',
                       'GITHUB_RUN_ID': '1', 'TRUSTED_SHA': 'trusted',
                       'GITHUB_STEP_SUMMARY': str(Path(tmp) / 'summary')}
                with patch.dict(os.environ, env), patch('cla.GitHub', API):
                    if race or peer == 'appeared':
                        with self.assertRaises(RuntimeError):
                            main()
                        self.assertEqual(statuses, ['pending', 'error'])
                    elif signed and peer is not False:
                        main()
                        self.assertEqual(statuses, ['pending', 'success'])
                    else:
                        with self.assertRaises(SystemExit):
                            main()
                        self.assertEqual(statuses, ['pending', 'failure'])
