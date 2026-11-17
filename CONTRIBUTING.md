# Contributing to Mu

Contributions currently use the licence applicable to their destination; see
[LICENSING.md](LICENSING.md). The proposed runtime licensing change in
[PR #2180](https://github.com/micro/mu/pull/2180) is not yet in effect.

## Commit sign-off and contributor agreements

The [Developer Certificate of Origin (DCO)](https://developercertificate.org/)
is the common commit sign-off mechanism. `git commit -s` adds a
`Signed-off-by: Your Name <your-email>` trailer. Under a project adopting the
DCO, that certifies the contributor has the right to submit the contribution
under the applicable open-source licence. It is not a cryptographic commit
signature and does not independently grant permission to relicense AGPL code
under proprietary terms.

Mu's proposed runtime licensing model needs an explicit contributor licence
agreement (CLA), or an equivalent recorded grant, permitting Micro XYZ Ltd to
license the covered contribution under the proposed runtime terms and separate
commercial terms. Contributors should retain their copyright and attribution.
Employer-owned or jointly owned contributions require the relevant authority.
A PR approval, co-author credit or DCO sign-off is not a substitute for that grant.

## Planned contribution process

Before activating the new runtime licence:

- Finalise and publish a versioned contributor agreement defining the covered
  contributions, copyright and any necessary patent grants, relicensing scope,
  and authority to contribute. Preserve the separate Apache-2.0 protocol/SDK terms.
- Provide a lightweight GitHub acceptance flow: an explicit PR comment accepting
  the linked agreement version, or a CLA integration recording equivalent consent.
- Record the contributor identity, agreement version, scope, acceptance permalink
  and any employer authorisation. Acceptance must come from the rights holder or
  their authorised representative, not be inferred from a maintainer's merge.
- Add a required check for contributions covered by that agreement. Recheck new
  contributors and co-authors when commits are added, and preserve the evidence
  when squashing. A ticked PR-template box alone is not proof of authority.
- Keep requests for historical contributions separate; a new policy cannot
  retroactively change their terms. Do not treat silence as acceptance.

This records the intended process, not an adopted CLA or an active automated gate.
Mu does not currently require DCO sign-off through this note. If DCO checks are
added for open-source components, keep them separate from runtime CLA acceptance;
the standard DCO refers to open-source licensing and should not be silently
repurposed for the proposed source-available runtime terms.

See the [licensing transition](docs/licensing/TRANSITION.md) for outstanding rights
work and the [Apache contributor agreement overview](https://www.apache.org/licenses/contributor-agreements.html)
for an example of explicit contributor grants.
