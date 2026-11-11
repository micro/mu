# Licensing

The root [LICENSE](LICENSE) is the default licence for this repository. The
following explicit exceptions apply:

| Material | Licence |
| --- | --- |
| Mu runtime and existing clients, including `internal/`, `service/` and `client/` | AGPL-3.0 |
| `proto/`, including schemas, generated contract types, examples, fixtures and contract tests | [Apache-2.0](proto/LICENSE), except `proto/generate/` |
| `proto/generate/` schema-generation implementation | [AGPL-3.0](proto/generate/LICENSE) |
| `docs/MUCP.md` protocol specification | [Apache-2.0](proto/LICENSE) |
| Other material without an explicit notice | Root AGPL-3.0 licence |

Generated contract code produced from the Apache-licensed schemas is covered by
Apache-2.0. The generator is separately licensed; its AGPL licence does not replace
the explicit Apache licence on its output. Third-party generators, libraries and
other dependencies retain their own terms.

Independent implementations may reuse the protocol specification and contracts
in open-source or proprietary software under Apache-2.0. This does not relicense
Mu's runtime or client implementations. Preserve licence and attribution notices,
include the applicable licence when redistributing, and identify modifications
as required by that licence. Neither licence grants rights to Micro or Mu branding.

Commercial use of AGPL-covered Mu is permitted under the AGPL. Alternative paid
terms for covered proprietary modifications are available by agreement; see the
[commercial section](README.md#commercial). Such an agreement covers only the
rights and material it specifies, not third-party components automatically.

Contributions should use the licence applicable to their destination. Contributing
does not by itself assign copyright or grant a separate right to relicense a
contributor's work commercially.
