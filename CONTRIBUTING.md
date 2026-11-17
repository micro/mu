# Contributing to Mu

Read [Mu CLA v1.0](CLA.md) before submitting contributions. Each human contributor
must explicitly accept it for the PR head being reviewed. Contributions still
use the applicable licences in [LICENSING.md](LICENSING.md); accepting the CLA
does not activate the proposed runtime licence or change earlier releases.

## Accepting on GitHub

1. Open your PR and follow the `cla` status's **Details** link.
2. Read the linked agreement at its recorded repository revision.
3. Copy the exact acceptance sentence from the workflow summary into a new PR
   comment using your own account. It includes the agreement's SHA-256 digest,
   PR number and full head commit SHA.
4. Each commit author and co-author must do the same. After new commits or a
   rebase, accept the new head again. Editing or deleting an acceptance causes
   re-verification; a missing acceptance fails the check.

The agreement preserves copyright and attribution and grants Micro XYZ Ltd
permission to license the contribution under source-available and commercial
terms. If an employer or another party owns rights, obtain authority first and
identify the entity and your authority in a separate PR comment. The check
records acceptance; maintainers must still review authority and third-party code.

GitHub must associate each commit author with an account. Co-author trailers must
use that contributor's GitHub noreply address so the check can resolve their
account and, when present, verify its numeric ID. Unresolved identities fail the
check. Automated accounts cannot accept for people; bot-only PRs need a human
contributor to take responsibility through an authored commit and acceptance.
No maintainer or organisation member is automatically exempt.

## DCO is different

[Developer Certificate of Origin](https://developercertificate.org/) sign-off
(`git commit -s`) records the right to contribute under an open-source licence.
It is not a cryptographic signature and does not independently grant proprietary
relicensing rights. A sign-off, checkbox or review approval does not replace the
explicit CLA acceptance above. Historical permissions remain separate.

## Repository enforcement

The trusted `.github/workflows/cla.yml` workflow publishes the `cla` commit status
on the actual PR head. It runs on PR updates and comment creation, edits and
deletion, and checks only code from the repository's default branch. It never
executes PR code with its write-capable token. Acceptance evidence links appear
in the workflow summary; preserve those records with the PR when merging.

Repository administrators must require `cla` for `main` in a branch rule/ruleset,
require pull requests, and disallow bypass and direct pushes if enforcement is to
apply to everyone. Select GitHub Actions as the expected status source. Ensure
`pull_request_target` is allowed by the repository's Actions event policy. Until
the workflow is merged and this rule is enabled, the check is not a merge block.
The workflow intentionally does not accept contributions or change repository
administration on anyone's behalf.

Keep CLA versions immutable. Any agreement edit changes its digest and requires
fresh acceptance; update the displayed version when publishing new terms. After
changing agreement or gate code, rerun the check on all open PRs before merging
them. Review changes to `.github/workflows/cla.yml`, `.github/scripts/cla.py` and
`CLA.md` as security-sensitive; repository write administrators remain trusted.

Run gate tests with:

```sh
python3 -m unittest discover -s .github/scripts -p 'test_*.py'
```
