# Commercial licensing and support

## Current licence and proposed change

Mu currently uses the [AGPL v3](LICENSE), with the exceptions in
[LICENSING.md](LICENSING.md). Commercial hosting and resale are permitted when
you comply with that licence. A paid commercial agreement offers alternative
terms only for rights Micro is authorised to grant.

We are preparing a different boundary for future runtime releases: free to run
and modify for yourself or internally within your organisation; a commercial
licence required to sell Mu or provide its functionality as a paid product or
hosted service. Modified and unmodified versions would be treated alike.

The [draft runtime licence](docs/licensing/RUNTIME-LICENSE-DRAFT.md) sets out that
proposal. **It is not in effect.** Existing AGPL permissions remain valid; see
the [transition requirements](docs/licensing/TRANSITION.md).

The [proposed commercial agreement](docs/licensing/COMMERCIAL-AGREEMENT.md) now
sets out the customer grant, downstream rights, monthly term, cancellation and
contributor implications. It remains a review proposal, not an executed agreement.

## Commercial licence — US$10,000/month

The [commercial licensing tier on GitHub Sponsors](https://github.com/sponsors/micro/sponsorships?tier_id=680807)
is for businesses commercialising Mu. It is separate from paid support.

A written agreement must identify the licensed entity, covered code and versions,
and the products, redistribution and hosted deployments it permits. It must also
set out the subscription term, payment and renewal arrangements, and what happens
on cancellation or non-payment, including treatment of copies already distributed
to customers. Do not assume perpetual rights, unlimited deployments, sublicensing
rights, trademark rights, or support commitments from the price alone.

GitHub Sponsors is the payment channel. The commercial agreement defines the
rights purchased. Arrange that agreement before relying on alternative licensing
terms; a sponsorship payment alone is not a commercial licence. Third-party code
is covered only by its own terms unless the relevant rights have separately been
obtained.

## Support — US$1,000/month

The [support tier on GitHub Sponsors](https://github.com/sponsors/micro/sponsorships?tier_id=248798)
is for help using and operating Mu. Support does not grant additional commercial
licensing rights. Scope, availability and any response commitments must be agreed
explicitly; this page does not promise an SLA or unlimited support.

You can use Mu under its applicable licence without purchasing support.

## Proposed runtime boundary

These examples explain the draft, not restrictions on current AGPL releases.

| Use | Proposed terms |
| --- | --- |
| Run Mu for yourself or your family | Free |
| Run or modify Mu for employees and contractors acting for your organisation | Free |
| Use Mu internally to produce a report for a paying customer | Free |
| Share a fork for free or operate a genuinely free community instance | Free, subject to the draft's conditions |
| Sell hosted Mu assistants, including under another brand or through an API | Commercial licence |
| Bundle Mu runtime code into a paid product or appliance | Commercial licence |
| Charge for installation, development or support of a customer's own permitted deployment | Permitted; does not include selling access to your own hosted Mu service |
| Sell independently written clients, services or MUCP implementations | No Mu commercial licence required merely for interoperability |

## Protocol and ecosystem

MUCP specifications, protocol contracts, generated contract types and the reusable
TypeScript and Python SDKs remain Apache-2.0 licensed. Independent commercial
implementations remain permitted. Go Micro is a separate project governed by its
own licence. The proposed restriction concerns Mu runtime code; it does not make
Micro the exclusive commercial provider of the protocol.

Existing standalone client applications and the schema generator remain AGPL
under this proposal. They are not the Apache-licensed SDKs. Combining a client
with runtime code does not remove the runtime's applicable conditions.
