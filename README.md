# apikit

One HTTP API envelope for the fleet. `authkit`, `openrails`, `contentkit`,
`doujins` and `hentai0` each grew their own error shape; four different bodies
reach the same SPAs. This module makes the envelope a type instead of a
convention.

```bash
go get github.com/open-rails/apikit              # types + vocabulary, no dependencies
go get github.com/open-rails/apikit/adapters/gin # the gin writers and locale middleware
```

> **v0.6.0 is superseded — do not adopt it.** It invented four transport types
> (`not_found_error`, `conflict_error`, `rejected_error`,
> `not_implemented_error`), put GinAPI's `object` discriminator on every
> library response, and made `param` a value. All four decisions were taken
> before anyone measured what the deployed writers emit. They are reverted in
> v0.7.0. Nothing had adopted v0.6.0, so nothing broke; the tag stays because
> tags are immutable.

## The two modules

| module | contains | depends on |
|---|---|---|
| `github.com/open-rails/apikit` | envelope types, the type/code vocabulary, status→type inference, `List`/`ListParams`, and `net/http` writers | nothing but the standard library |
| `github.com/open-rails/apikit/adapters/gin` | `gin.Context` writers, query-string binding, the locale middleware | gin, apikit |

Libraries import the root. Only services that already use gin import the
adapter. CI fails the build if anything outside the standard library reaches
the root module, and `scripts/check-adapter-module.sh` fails it if the adapter
pins a root version that is not published.

## The wire

```json
{"error":{"type":"invalid_request_error","code":"moderation_rejected","message":"the artwork was refused"}}
{"object":"list","data":[],"total":0,"limit":20,"offset":0,"has_more":false}
{"object":"message","message":"saved"}
{"object":"artist","id":"art_1","deleted":true}
```

The error envelope is the shape AuthKit and OpenRails deploy today: nested
under `error`, with **no top-level discriminator**. GinAPI adds
`"object":"error"`; that is a **compatibility difference**, available as
`apikit.WithObject` and applied by no writer. Whether it belongs in the
canonical envelope is an open owner decision with measured consequences —
[`compat/DECISION-object-discriminator.md`](compat/DECISION-object-discriminator.md).

`code`, `param`, `request_id` and `metadata` are omitted when empty.
`request_id` is OpenRails' correlation id and is preserved.

## The stable code is what a client branches on

`type` is the **transport** category: what a client does with the response
before it knows anything about the domain. `code` is the **reason**, and it is
the field a SPA switches on. Adding a transport type is a wire migration for
every parser in the fleet; adding a code is not.

| type | status | what the client does |
|---|---|---|
| `invalid_request_error` | 400, **404**, **409**, 415, **422**, other 4xx | fix the input, retry |
| `authentication_error` | 401 | authenticate or refresh, retry |
| `authorization_error` | 403 | never retry as this principal |
| `card_error` | 402 | collect a new payment method (OpenRails) |
| `rate_limit_error` | 429 | back off, retry later |
| `api_error` | 5xx, **501 included** | retry with backoff, or hide the feature if the code says so |

404 and 409 stay `invalid_request_error` because that is what AuthKit and
OpenRails put on the wire today (`compat/golden/`); splitting them out is a
deliberate major migration, not a bug fix. `Type` is an open string, so a
domain may still add its own.

The two cases that tempt a new type, and their codes instead:

| condition | status | type | code |
|---|---|---|---|
| moderation refused the content | 422 | `invalid_request_error` | `moderation_rejected` |
| the capability is absent from this build | 501 | `api_error` | `not_implemented` |
| the operator never wired an optional capability | 501 | `api_error` | `not_configured` |

`CodeForStatus` exists but **the writers never apply it**. An absent `code`
means "no machine reason beyond the status". Inventing one is wire-visible:
Doujins' client displays `code` in preference to `message`, so an invented code
replaces a human sentence with a machine string — measured, in
`compat/spa/golden/parsers.json`.

## `metadata` is an allowlist, not a map

Doujins' HTTP client spreads `error.metadata` straight onto the top-level
object it hands the UI. A value put here is shown to end users, and a key can
shadow an envelope field. So it is enforced, not trusted:

- a key survives only if it is **registered** (`apikit.RegisterMetadataKey`);
- reserved names (`object`, `error`, `message`, `code`, `type`, `param`,
  `request_id`, `status`, `data`, `body`, …) can never be registered;
- values must be **scalars** — no nested maps or slices;
- a value that looks like an internal detail is dropped: driver text,
  constraint names, stack frames, JWTs and bearer tokens, serialized payloads,
  multi-line traces, URLs, email addresses;
- the map is bounded: 8 keys, 200 runes per string, 1 KiB total.

`SanitizeMetadata` runs inside `NewEnvelope`, so no writer can opt out.
`MetadataRejections` reports why a key was dropped, so a test can assert a leak
is refused instead of hoping it was.

## What changed from each source

| concept | ginapi | authkit | openrails | apikit |
|---|---|---|---|---|
| envelope | `{object,error{}}` | `{error{}}` | `{error{}}` (`billingauth` still `{object,error{}}` at v0.142.2) | `{error{}}` |
| invalid request | `invalid_request` | `invalid_request_error` | `invalid_request_error` | `invalid_request_error` |
| authentication | `authentication` | `authentication_error` | `authentication_error` | `authentication_error` |
| authorization | `forbidden` | `authorization_error` | `authorization_error` | `authorization_error` |
| not found | `not_found` | `invalid_request_error` | `invalid_request_error` | `invalid_request_error` |
| conflict | `conflict` | `invalid_request_error` | `invalid_request_error` | `invalid_request_error` |
| rate limit | `rate_limit` | `rate_limit_error` | `rate_limit_error` | `rate_limit_error` |
| internal | `api` | `api_error` | `api_error` | `api_error` |
| 501 | `api`, no code | `api_error` + `not_implemented` | `api_error` + `internal_error` | `api_error` + `not_implemented` |
| moderation refusal | 422 `invalid_request`, no code | — | — | 422 `invalid_request_error` + `moderation_rejected` |
| `param` | `string` | `*string` | `*string` | `*string`, set from a plain string |
| `code` | omitted by every writer | always present | always present | omitted unless the caller gives one |
| `metadata` | — | free map | free map | allowlisted, scrubbed, bounded |
| `request_id` | — | — | yes | yes |

The `_error` suffix wins because two of the three sources already use it and it
is the published Stripe taxonomy OpenRails' integrators read. The losing GinAPI
strings (`api`, `authentication`, `forbidden`, …) are a v1 wire migration: no
SPA switches on them today (`compat/spa/golden/parsers.json`), so the change is
Go-only, but it still lands in a coordinated adoption PR.

## Usage

```go
// A library with no framework: map your sentinel onto the transport.
if errors.Is(err, ErrNotVisible) {
    apikit.WriteError(w, apikit.E(http.StatusNotFound, apikit.CodeResourceNotFound, "not found"))
}

// A gin service.
apikitgin.NotFound(c, "gallery")
apikitgin.ModerationRejected(c, verdict.Reason)
apikitgin.ListResponse(c, items, total, p.Limit, p.Offset)

p := apikitgin.BindDefault(c) // ?limit=20&offset=0&sort=name, capped at 100
```

`Fail` is `apikit.WriteError` against `c.Writer` — the same function, so the gin
and `net/http` bytes cannot drift. Both scrub **500 only**: the one status that
means "unexpected", so its message may carry an internal cause. 501, 502 and
503 state a deliberate operational condition and keep theirs.

## Locale middleware

`apikitgin.Language` resolves a request's language from, in order: `?lang=`, a
`/ja/…` path prefix, the `lang` cookie, `Accept-Language` q-values, then the
configured default. It stores the result in the gin context (`GetLanguage`) and
sets `Content-Language`. `HandleLanguageRedirect` is the `NoRoute` companion
that 302s an unprefixed path to a concrete language.

## Compatibility evidence

[`compat/`](compat/) holds byte-for-byte fixtures for every deployed writer
and, in [`compat/spa/`](compat/spa/), the consumers' own parsers run unmodified
against all of them. Any wire change is argued from there. Adoption is planned
per consumer in [`ADOPTION.md`](ADOPTION.md).

## History

This repository was `doujins-org/ginapi`. It was transferred to `open-rails` and
renamed; the history, tags and issues are the same ones.
