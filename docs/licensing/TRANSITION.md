# Runtime licensing transition

Status: proposal, not activated. Baseline inspected:
`71b92d00eb93a6aa5c7dc635b4df8ac854c9f053`.

## Intended release boundary

Apply the proposed source-available terms to future Mu runtime code only after
rights and legal review. Keep MUCP specifications, `proto/` contracts and generated
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

The current [contribution policy](../../LICENSING.md) explicitly says contributions
do not assign copyright or separately grant commercial relicensing rights. No
contributor agreement was located in the inspected repository. Commit authorship
is an investigation aid, not proof of copyright ownership or absence of a separate
agreement.

One specific contribution to resolve is [PR #1455](https://github.com/micro/mu/pull/1455),
commit `f0debf02d773818beddbd3ec4ea5d5de7b26155e`, attributed to Oshadha Gunawardena.
It touches OpenRouter/provider code, agent execution, setup and related tests and
docs. Some paths have since moved from `internal/ai` to `internal/model`; the
review must follow the surviving changes, not just today's filenames.

Before activation, document ownership or sufficient permissions for all covered
code, including surviving external contributions, imported code, and work created
under other entities. Where permissions are absent, obtain them or remove/replace
the affected material lawfully. Check dependency licence compatibility for the
combined runtime. Do not assume this one contribution is an exhaustive audit.

## Activation work

1. Obtain legal review of the draft grant, paid-product boundary, professional
   services exception, termination and commercial subscription agreement. Confirm
   the licensor's identity and authority. This draft has not received legal review.
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
