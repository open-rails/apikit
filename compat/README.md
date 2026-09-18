# compat — what the fleet already puts on the wire

Evidence, not a library. Nothing imports this module; it is the only one here
allowed to depend on authkit, openrails and ginapi, and it is excluded from
`go.work` because it needs a newer Go than the root module targets.

```bash
GOWORK=off go test ./... -count=1        # verify
GOWORK=off go test ./... -update         # regenerate
cd spa && node probe.mjs [--update]      # the SPA parsers
```

## golden/

Real bytes off a real socket (`httptest.NewServer` + `http.Get`), including
status line, header order and trailing newline. `Date` and `Content-Length`
are dropped; everything else is what a client receives.

| directory | writer | version |
|---|---|---|
| `authkit/` | `authkit.WriteError` (`httperror.go`) | v0.103.1 — Doujins' and Hentai0's pin |
| `openrails/` | `pkg/api.SimpleErrorResponse`, `APIError.ToResponse`, `pkg/billingauth.WriteJSONError` | v0.142.2 — both pins |
| `openrails-unreleased/` | `pkg/billingauth.WriteJSONError` after PR #471 | `~/openrails` `e4d109e06`, not yet tagged |
| `ginapi/` | `response.*` | doujins-org/ginapi v0.5.0 |
| `apikit/` | `apikit.WriteError` | this tree |
| `apikit-compat-object/` | `apikit.WriteJSON(w, WithObject(env))` | this tree |

Doujins pins ginapi v0.4.0 and Hentai0 v0.5.0. `TestGinAPIVersionsAgree` hashes
`response/errors.go`, `response/response.go` and `response/list.go` in both
module versions and fails if they ever diverge, so one `ginapi/` directory
covers both pins. It asserts the COUNT of files compared — a skipped file
would otherwise be a silent pass.

## spa/

The consumers' real parsers, run unmodified against every body above.
`vendor/` holds byte-for-byte copies (see `PROVENANCE.md` for repo, commit and
sha256; `./verify-vendor.sh` re-extracts and diffs). Only leaf modules the
error path never reaches are stubbed, and the one source transform —
`import.meta.env`, which Vite substitutes at build time and node cannot — is
counted in `resolver.mjs` so it can never grow.

`golden/parsers.json` records, per fixture, what each parser produced: the
message a user sees, the key Doujins' localization looks up, which metadata
keys were spread onto the top-level error object, and whether the 200 guard
fired. It is the evidence behind three decisions in the root module:

- **writers never invent a `code`** — Doujins' client displays `code` in
  preference to `message`, so an invented code replaces a human sentence with a
  machine string;
- **metadata is allowlisted and scrubbed** — Doujins spreads it straight onto
  the object it hands the UI, where a key can shadow a top-level field;
- **`object` is unresolved** — see `DECISION-object-discriminator.md`.
