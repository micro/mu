# Runtime licensing transition

Status: proposal, not activated. Baseline inspected:
`71b92d00eb93a6aa5c7dc635b4df8ac854c9f053`.

## Intended release boundary

Apply the proposed source-available terms to future Mu runtime code only after
confirmation of the necessary rights. Keep MUCP specifications, `proto/` contracts and generated
types, and `client/typescript/` and `client/python/` under their existing Apache-2.0
terms. Keep existing standalone `client/` applications and `proto/generate/`
AGPL-licensed. Third-party notices remain intact. Go Micro is outside this change.

Do not retain AGPL as an alternative for the newly restricted runtime if mandatory
payment for commercial provision is the intended rule: recipients could otherwise
choose AGPL and continue commercial hosting under its terms.

Previously granted AGPL or Apache permissions cannot be withdrawn by replacing
the repository licence. Earlier copies remain usable under the licences under
which they were received. No retroactive licence fee is proposed.

## Rights evidence still needed

At the baseline above, the contribution policy did not assign copyright or grant
separate commercial relicensing rights. The CLA subsequently added in #2181
applies through explicit acceptance and does not retroactively clear this code. Commit authorship
is an investigation aid, not proof of copyright ownership or absence of a separate
agreement.

One specific contribution to resolve is [PR #1455](https://github.com/micro/mu/pull/1455),
commit `f0debf02d773818beddbd3ec4ea5d5de7b26155e`, attributed to Oshadha Gunawardena.
It touches OpenRouter/provider code, agent execution, setup and related tests and
docs. Some paths have since moved from `internal/ai` to `internal/model`; the
review must follow the surviving changes, not just today's filenames.

### Repository review findings (17 November 2026)

The repository was already AGPL-licensed when #1455 was contributed. Its public
PR discussion contains a thank-you from the maintainer, but no separate licence
grant. `git blame -M -C` follows surviving code into
`internal/model/openrouter.go`, including provider registration, settings access
and model selection. Renaming the package did not remove the contribution.

Reviewing co-author trailers, rather than only primary commit authors, also found:

| Contributor record | Change requiring rights review |
| --- | --- |
| Oshadha Gunawardena / nooscraft | [#1455](https://github.com/micro/mu/pull/1455), OpenRouter provider; `f0debf02` |
| RomainZento | [#1477](https://github.com/micro/mu/pull/1477), OpenAI-compatible native agents; `8359bd1b` |
| Dev9269 | [#1467](https://github.com/micro/mu/pull/1467), outgoing mail size limit; `1373c79e` |
| mikemikimike | [#1461](https://github.com/micro/mu/pull/1461), store-shrink safety; `a345e79f` |
| kramlipi | Test teardown from [#1456](https://github.com/micro/mu/pull/1456), incorporated in `8300b247` |

These records identify material to investigate; they do not prove ownership or
rule out agreements held outside GitHub. Only #1455's public discussion and
current provider-file attribution have been checked in detail so far. Obtain
sufficient permission from the actual rights holders for surviving contributions,
including employer-owned work where applicable. A simple maintainer approval,
co-author credit or ordinary AGPL contribution is not a separate relicensing grant.

The rights issue also applies to selling an AGPL exception for a combined runtime
containing those contributions, not only to replacing the default runtime licence.
A commercial agreement must not promise rights in that material until cleared.
See the GNU project's [relicensing FAQ](https://www.gnu.org/licenses/gpl-faq.en.html#Consider)
for the contributor-permission distinction. This review is repository evidence,
not legal clearance or an exhaustive dependency audit.

Before activation, document ownership or sufficient permissions for all covered
code, including surviving external contributions, imported code, and work created
under other entities. Where permissions are absent, obtain them or remove/replace
the affected material lawfully. Check dependency licence compatibility for the
combined runtime. Do not assume this one contribution is an exhaustive audit.

## Permission responses (17 November 2026)

- Dev9269: [explicit permission received](https://github.com/micro/mu/pull/1467#issuecomment-6948200169)
  for the identified #1467 contribution. The response confirms authority and grants
  the requested additional copyright permission. It is not a grant for unrelated
  or future work and does not separately grant patent rights.
- RomainZento / zentotech: [requested final terms before consenting](https://github.com/micro/mu/pull/1477#issuecomment-6947036250).
  No permission granted. Provide the exact published terms before seeking consent.
- nooscraft, mikemikimike and kramlipi: no licensing response observed in the
  checked request threads as of this review; permission remains pending.

[Proposed commercial terms](COMMERCIAL-AGREEMENT.md) provide a concrete customer
agreement and explain sublicensing. Publishing them does not execute a customer agreement or activate a runtime
licence change. No external legal review is planned or required as a project
merge prerequisite; contributor authority and sufficient permissions still matter. The prospective CLA merged in #2181
is separate from these historical requests and does not retroactively clear them.

## Activation work

1. Confirm the licensor's identity and authority and publish the exact runtime
   and commercial terms proposed for consent. External legal review is not a
   project prerequisite. Do not claim that one has occurred.
2. Complete the rights and dependency review above and establish contribution
   terms that expressly permit the intended licensing of future contributions.
3. Select and record the first covered release. Replace the root licence only for
   that release and preserve the earlier licence in release history.
4. Add an explicit AGPL licence for the standalone `client/` applications; retain
   the nested Apache SDK exceptions and the generator's separate AGPL licence.
5. Update `LICENSING.md`, README wording and applicable source notices together.
   Describe the runtime as source-available and the protocol/SDKs as permissive.
6. Update release licence packaging (`.github/workflows/release.yml`) and desktop
   licence copying (`client/desktop/prepare.mjs`) so applications receive their
   own AGPL text rather than a changed root licence. Review web and Android
   notices and all distributed archives. Update `proto/licensing_test.go`, whose
   generator check currently requires equality with the root AGPL text, to verify
   the separate generator licence instead. Check packaged notices against the
   licensing map.
7. Publish the reviewed commercial agreement and align both Sponsors tier
   descriptions with it. Confirm the support tier promises no resale rights and
   the commercial tier promises only the agreed support and deployment scope.

No runtime behaviour or effective licensing is changed by this proposal. The
current root licence, package exceptions and build packaging remain authoritative
until the activation changes are completed.

## Proposed README commercial section after activation

> Mu's runtime is source-available. Run and modify it for yourself, your family,
> or your organisation's internal use without a commercial licence. Selling Mu,
> including modified versions, or providing its assistant or runtime functionality
> as part of a paid product or hosted service requires a commercial licence.
>
> [Commercial licensing](../../COMMERCIAL.md) is US$10,000/month through GitHub
> Sponsors under a written agreement. The separate US$1,000/month support tier
> provides help operating Mu and does not grant additional licensing rights.
>
> MUCP specifications, contracts, generated types and the TypeScript and Python
> SDKs remain Apache-2.0 licensed. You may build and sell independent clients,
> services and runtime implementations. Standalone client applications retain
> their AGPL licence. See the licensing map for each component's terms.
