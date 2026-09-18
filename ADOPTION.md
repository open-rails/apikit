# Adoption plan

Nothing has adopted apikit. Doujins pins `doujins-org/ginapi` v0.4.0 and
Hentai0 v0.5.0; both still resolve through the proxy, so nothing is broken and
there is no deadline. Every row below is evidence-backed by `compat/`.

**Blocked on one owner decision:** the `object` discriminator
(`compat/DECISION-object-discriminator.md`). Hentai0's adoption cannot be
planned precisely until it is made; everything else can proceed.

## Order

1. **AuthKit** — smallest diff, no wire change.
2. **OpenRails** — one behaviour change (402/501 codes), one wire change it is
   already making itself.
3. **ContentKit** — has no envelope at all today; pure gain.
4. **Doujins** — the SPA needs locale keys before the Go side lands.
5. **Hentai0** — needs the discriminator decision first.

## AuthKit (v0.103.1)

`httperror.go` already emits the canonical shape. Replace `ErrorObject`,
`ErrorEnvelope`, `ErrorTypeForStatus` and `WriteError` with apikit's; keep the
`errmodel` catalog, which is authkit's own domain vocabulary and is not moving.

| item | today | after | wire |
|---|---|---|---|
| envelope | `{error{}}` | same | unchanged |
| `type` for 400/404/409 | `invalid_request_error` | same | unchanged |
| `param` | `*string` + catalog default | same | unchanged |
| `code` | always present | always present (authkit passes one) | unchanged |
| 402 | `invalid_request_error` | `card_error` | **authkit emits no 402 today** (checked: no 402 in the catalog) |
| `metadata` | free map | allowlisted | **register authkit's keys** or they are dropped |

Action: audit every `WithMeta`/`WithMetadata` call site and
`apikit.RegisterMetadataKey` each key in an initializer beside the catalog.
CI-check it with `apikit.MetadataRejections` over the catalog's own fixtures.

## OpenRails (v0.142.2)

Two envelopes ship today: `pkg/api` (canonical) and `pkg/billingauth`
(`{object,error{}}` with the code in the `type` field, no `request_id`).
PR #471 on master already replaced the second with the first — land and
release that before adopting, so adoption is one change and not two.

| item | today | after | wire |
|---|---|---|---|
| `pkg/api` envelope | `{error{}}` + `request_id` | same | unchanged |
| 404/409 type | `invalid_request_error` | same | unchanged |
| 402 | `card_error` + `payment_failed` | same | unchanged |
| 501 | `api_error` + `internal_error` | `api_error` + `not_implemented` | **code changes** — check any client switching on `internal_error` at 501 |
| `SimpleErrorResponse` | always fills a code | keep calling `CodeForStatus` explicitly | unchanged |
| `metadata` | free map | allowlisted | register the retry/decline keys |
| `billingauth` | `{object,error{}}` | canonical | **land #471 first** |

## ContentKit (v1.0.0-rc.3)

Emits `{"error":"not found"}` — a bare string, no type, no code. Adoption is
additive for anything that reads `.error` as text only. The four findings from
the design review land here:

- `errUnsupportedMedia` → 501 `api_error` + `not_implemented`, not a 500;
- `ErrTenant` → 500 `api_error` + `internal_error`, cause to the log only;
- taxonomy PostgreSQL conflicts → 409 `invalid_request_error` +
  `resource_conflict`; the constraint name **cannot** reach `metadata` — the
  allowlist drops it, and `compat` has that exact fixture
  (`golden/apikit/metadata-scrubbed.http`);
- the 202 held-moderation response is a success payload, never an envelope.

Taxonomy handlers need the same `statusWriter.internalErr` capture path as
content handlers; preserve it through the refactor.

## Doujins

Go side: swap `github.com/doujins-org/ginapi` for
`github.com/open-rails/apikit/adapters/gin`. The type strings change
(`invalid_request` → `invalid_request_error`, `not_found` →
`invalid_request_error`, `api` → `api_error`). No SPA branch reads them
(`compat/spa/golden/parsers.json`), so this is Go-only.

**SPA side, do this first.** `frontend/src/lib/http/client.ts` displays
`error.code` in preference to `error.message`, and components render
``t(`errors.${code}`)``. GinAPI writers send no code today, so Doujins shows
sentences. Any apikit writer that does send a code will show the raw code
string unless `i18n/locales/*.json` has an `errors.<code>` entry. Measured:

| body | what the user sees today |
|---|---|
| ginapi 404 | `gallery not found` |
| apikit 422 + `moderation_rejected` | `moderation_rejected` |
| apikit 501 + `not_implemented` | `not_implemented` |
| authkit 400 + `invalid_email` | `invalid_email` |

Action, in order: (1) add `errors.moderation_rejected`,
`errors.not_implemented`, `errors.not_configured`, `errors.resource_conflict`,
`errors.resource_not_found` and the authkit codes already reaching users to
every locale file; (2) then adopt the Go side. Consider making
`errorMessageFromData` prefer `message` and keep `code` for the localization
lookup — that removes the whole class, and `normalizeErrorData` already puts
the code in `data.error` where `authErrorLocalization.ts` reads it.

## Hentai0

**Blocked on the discriminator decision.** Two readers gate on
`object === 'error'`:

- `frontend/src/services/api/api-service.ts` — the success-path guard that
  catches an error body sent with a 2xx status. Without `object` it returns the
  envelope as the success payload.
- `frontend/src/hooks/upload/useUploadVideo.ts` — without `object` the uploader
  sees `"Upload failed - please try again"` instead of the server's reason.

The non-2xx path in both is shape-agnostic and needs no change.

If the owner picks option C (canonical stays nested, hentai0's readers are
fixed), the hentai0 PR is small and lands **before** the Go adoption:

```ts
// api-service.ts, success path
const err = (data as Record<string, unknown>)?.error
if (err && typeof err === 'object' && typeof (err as any).type === 'string') { /* throw */ }

// useUploadVideo.ts
if (errorData?.error && typeof errorData.error === 'object') {
  errorMessage = errorData.error.error_user_msg || errorData.error.message
}
```

Go side afterwards: the same ginapi → apikit swap as Doujins. `getUserErrorMessage`
switches on domain types (`video_not_found`, …), not transport types, and
`userMessage` already prefers `message`, so transport renames are invisible.

## Gate for every adoption PR

1. `cd compat && GOWORK=off go test ./... -count=1` — the fixtures still
   describe reality.
2. `cd compat/spa && ./verify-vendor.sh && node probe.mjs` — the consumer's
   parser has not changed underneath the evidence.
3. `./scripts/check-adapter-module.sh` — the adapter pin is published.
4. The adopting repo's own suite, plus a fixture in `compat/golden/` for any
   endpoint whose bytes change.
