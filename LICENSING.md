# Licensing

The root [LICENSE](LICENSE) is the default licence for this repository. The
following explicit exceptions apply:

| Material | Licence |
| --- | --- |
| Mu runtime and existing clients, including `internal/`, `service/` and existing applications in `client/` | AGPL-3.0 |
| `client/typescript/` and `client/python/` reusable SDKs | [Apache-2.0](client/typescript/LICENSE) |
| `proto/`, including schemas, generated contract types, examples, fixtures and contract tests | [Apache-2.0](proto/LICENSE), except `proto/generate/` |
| `proto/generate/` schema-generation implementation | [AGPL-3.0](proto/generate/LICENSE) |
| `docs/MUCP.md`, `docs/MUCP-HTTP.md`, `docs/MUCP-RUNTIME.md`, `docs/MUCP-CONFORMANCE.md` specifications and conformance guide | [Apache-2.0](proto/LICENSE) |
| Other material without an explicit notice | Root AGPL-3.0 licence |

Generated contract code produced from the Apache-licensed schemas is covered by
Apache-2.0. The generator is separately licensed; its AGPL licence does not replace
the explicit Apache licence on its output. Third-party generators, libraries and
other dependencies retain their own terms.

Independent implementations may reuse the protocol specification and contracts
in open-source or proprietary software under Apache-2.0. The SDK exception does not relicense
Mu's runtime or existing client applications. Preserve licence and attribution notices,
include the applicable licence when redistributing, and identify modifications
as required by that licence. Neither licence grants rights to Micro or Mu branding.

Commercial use of AGPL-covered Mu is permitted under the AGPL. Alternative paid
terms for covered proprietary modifications are available by agreement; see the
[commercial section](README.md#commercial). Such an agreement covers only the
rights and material it specifies, not third-party components automatically.

Contributions use the licence applicable to their destination. Submitting code
alone does not assign copyright or grant separate commercial relicensing rights.
Explicit acceptance of [CLA.md](CLA.md) grants the additional rights defined there
for the identified contribution. See [CONTRIBUTING.md](CONTRIBUTING.md) for the
acceptance process and required GitHub enforcement settings.

## Commercial terms and future runtime licence

[COMMERCIAL.md](COMMERCIAL.md) separates the US$10,000/month commercial licence
from US$1,000/month support. The [commercial agreement](docs/licensing/COMMERCIAL-AGREEMENT.md)
defines the customer terms, subject to a signed order and sufficient rights.
The [proposed runtime licence](docs/licensing/RUNTIME-LICENSE-DRAFT.md) is not
active; the existing licensing map remains authoritative.
[Transition status](docs/licensing/TRANSITION.md) records contributor permissions.
