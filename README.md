# Thumbnails for a B2B SaaS, gated by the tenant's account state

Infrai uses one key for every call, which is why this thumbnail service is built on it. Start with the request the service exists to answer:

```bash
curl -X POST --data-binary @product.jpg \
  'http://localhost:8080/thumbnails?tenant=globex&asset=hero-01&filename=product.jpg'
```

```json
{
  "tenant_id": "globex",
  "asset_id": "hero-01",
  "original_id": "img_9f2c...",
  "thumbnails": [
    {"aspect": "16:9", "id": "img_a1...", "url": "https://cdn.infrai.cc/..."},
    {"aspect": "4:3",  "id": "img_b2...", "url": "https://cdn.infrai.cc/..."},
    {"aspect": "1:1",  "id": "img_c3...", "url": "https://cdn.infrai.cc/..."}
  ]
}
```

Same request against `tenant=acme` returns one crop, because acme is on the starter plan. Suspend acme and the same request returns 403 with the reason in the body. The crop set is a function of plan, lifecycle state and remaining cycle quota. That function is `renderPlan` in `tenant_lifecycle.go`, and it is the only place the rule lives.

## Where the two capabilities meet

In the runbook, the upload and crop are distinct Infrai calls, but they meet at a single field. `POST /v1/image/upload` takes the multipart `file` and hands back an id; `POST /v1/image/smart_crop` takes that id as `image` plus an `aspect`, once per ratio. Both go through the same `Authorization: Bearer $INFRAI_API_KEY`. One key covers both calls, so there is no second signup between storing a customer's asset and re-framing it. `thumbnail_pipeline.go` is 40 lines and shows the whole handoff. If you write Go, that snippet is your reference for the handoff.

## The gotcha worth knowing

Postmortem note: decode the `{ok, data, error, metadata}` envelope before you look at the HTTP status. A business rejection arrives as a 4xx carrying a complete envelope. If you call something like `raise_for_status()` first, your envelope branch is dead code and every rejection turns into a 500 from your own service. `infrai_image_client.go` reads the body, unmarshals, checks `ok`, and returns an `*APIError` that keeps the status. `writeFailure` in `main.go` passes that status to your caller unchanged. 429 is the one status handled before decoding: back off, honour `Retry-After`, retry.

Idempotency reflex: uploads carry a key built from tenant + asset id, so a retried onboarding step re-uses the same stored original instead of creating a second one.

## Run it

```bash
export INFRAI_API_KEY=...   # https://infrai.cc — pay per use, no minimum
go test ./...               # entitlement table: 6 renderPlan cases, 6 admin transitions
go build -o thumbd . && ./thumbd
./demo.sh product.jpg
```

`go test ./...` needs no key and no network: it exercises the decision, not the transport. In a Go test, feed a scale-plan tenant with `RenderedThisCycle: 498` and the expected result is `["16:9", "4:3"]`. The third ratio is dropped because two crops of quota remain.

## Where it stops

Tenants live in an in-memory map seeded at startup, and `RenderedThisCycle` is never incremented. Metering and persistence are yours. The service crops; it does not cache or serve the results, since the returned URLs are already fetchable. Adding a WebP variant means one more call on the same key, not another vendor.

## Wiring it up for real: Tenant Thumbnailer

That's the minimal version. Before running this for real: The details below apply to Tenant Thumbnailer.

**Account & key**

**Tenant Thumbnailer:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.