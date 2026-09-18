# apikit

One HTTP API envelope for the fleet. `authkit`, `openrails`, `contentkit`,
`doujins` and `hentai0` each grew their own error shape; four different bodies
reach the same SPAs. This module makes the envelope a type instead of a
convention.

```bash
go get github.com/open-rails/apikit              # types + vocabulary, no dependencies
go get github.com/open-rails/apikit/adapters/gin # the gin writers and locale middleware
```

## The two modules

| module | contains | depends on |
|---|---|---|
| `github.com/open-rails/apikit` | envelope types, the type/code vocabulary, status→type inference, `List`/`ListParams`, and `net/http` writers | nothing but the standard library |
| `github.com/open-rails/apikit/adapters/gin` | `gin.Context` writers, query-string binding, the locale middleware | gin, apikit |

Libraries import the root. Only services that already use gin import the
adapter. CI fails the build if anything outside the standard library reaches
the root module.

## The wire

```json
{"object":"error","error":{"type":"not_found_error","code":"resource_not_found","message":"gallery not found"}}
{"object":"list","data":[],"total":0,"limit":20,"offset":0,"has_more":false}
{"object":"message","message":"saved"}
{"object":"artist","id":"art_1","deleted":true}
```

`code`, `param`, `request_id` and `metadata` are omitted when empty.

## Type is the client's next action; code is the reason

A `type` exists when a client must do something **different**. Anything that
only explains a failure is a `code`.

| type | status | what the client does |
|---|---|---|
| `invalid_request_error` | 400, 415, 422, other 4xx | fix the input, retry |
| `authentication_error` | 401 | authenticate or refresh, retry |
| `authorization_error` | 403 | never retry as this principal |
| `not_found_error` | 404 | render an absent state |
| `conflict_error` | 409 | reload, reconcile, retry |
| `rejected_error` | 422 | show the author the reason; never retry unchanged |
| `rate_limit_error` | 429 | back off, retry later |
| `not_implemented_error` | 501 | hide the feature; never retry |
| `api_error` | 500, 502, 503 | retry with backoff |

`Type` is an **open string**, not a closed enum. OpenRails' `card_error` (collect
a new payment method) is a legitimate domain extension by the same rule, not
drift. Add one when the client's action is genuinely new.

`TypeForStatus` infers only where the status decides on its own. **422 is
ambiguous** — a malformed entity and a moderation refusal share it — so it
yields `invalid_request_error` and a caller that means `rejected_error` says so
(`apikitgin.Rejected`, or `WithType(apikit.TypeRejected)`).

`CodeForStatus` exists but **the writers never apply it**. An absent `code`
means "no machine reason beyond the status". Inventing one is wire-visible: a
client that prefers `code` over `message` would start showing machine strings
where it used to show sentences.

## What changed from each source

| concept | ginapi | authkit | openrails | contentkit | apikit |
|---|---|---|---|---|---|
| envelope | `{object,error{}}` | `{error{}}` | `{error{}}` | `{"error":"…"}` | `{object,error{}}` |
| invalid request | `invalid_request` | `invalid_request_error` | `invalid_request_error` | — | `invalid_request_error` |
| authentication | `authentication` | `authentication_error` | `authentication_error` | — | `authentication_error` |
| authorization | `forbidden` | `authorization_error` | `authorization_error` | — | `authorization_error` |
| not found | `not_found` | `invalid_request_error` | `invalid_request_error` | — | `not_found_error` |
| conflict | `conflict` | `invalid_request_error` | `invalid_request_error` | — | `conflict_error` |
| rate limit | `rate_limit` | `rate_limit_error` | `rate_limit_error` | — | `rate_limit_error` |
| internal | `api` | `api_error` | `api_error` | — | `api_error` |
| moderation refusal | 422 as `invalid_request` | — | — | 422, untyped | `rejected_error` |
| capability absent | 501 as `api` | — | — | 501/500, untyped | `not_implemented_error` |
| card decline | — | — | `card_error` | — | `card_error` (domain) |
| `param` | `string` | `*string` | `*string` | — | `string`, omitempty |
| `metadata` | — | yes | yes | — | yes |
| `request_id` | — | — | yes | — | yes |

The `_error` suffix wins because three of the four sources already use it and
it is the published Stripe taxonomy openrails' integrators read. `not_found` and
`conflict` keep their own types because collapsing them onto
`invalid_request_error` would throw away a distinction the highest-traffic
surface already ships: a 404 is not a malformed request.

`param` is a value, not a pointer: on the wire `*string` and `string`+omitempty
are identical for the two states that matter, and the pointer additionally
permits `"param": null`, which nothing wants.

`object` stays. Adding it to a body breaks no parser; removing it breaks the
consumers that branch on it today.

## Usage

```go
// A library with no framework: map your sentinel onto the transport.
if errors.Is(err, ErrNotVisible) {
    apikit.WriteError(w, apikit.E(http.StatusNotFound, apikit.CodeResourceNotFound, "not found"))
}

// A gin service.
apikitgin.NotFound(c, "gallery")
apikitgin.Rejected(c, verdict.Reason)
apikitgin.ListResponse(c, items, total, p.Limit, p.Offset)

p := apikitgin.BindDefault(c) // ?limit=20&offset=0&sort=name, capped at 100
```

`WriteError` and `Fail` scrub **500 only** — the one status that means
"unexpected", so its message may carry an internal cause. 501, 502 and 503 state
a deliberate operational condition and keep theirs.

## Locale middleware

`apikitgin.Language` resolves a request's language from, in order: `?lang=`, a
`/ja/…` path prefix, the `lang` cookie, `Accept-Language` q-values, then the
configured default. It stores the result in the gin context (`GetLanguage`) and
sets `Content-Language`. `HandleLanguageRedirect` is the `NoRoute` companion
that 302s an unprefixed path to a concrete language.

## History

This repository was `doujins-org/ginapi`. It was transferred to `open-rails` and
renamed; the history, tags and issues are the same ones.
