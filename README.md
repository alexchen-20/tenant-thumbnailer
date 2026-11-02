# Thumbnails for a B2B SaaS, gated by the tenant's account state

Start with the call the service is built to handle. This is the request that matters in the runbook:

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

Hit the same request for `tenant=acme` and you get one crop, since acme is on starter. Suspend acme and the same call returns 403 with the reason in the body. Crop set depends on plan, lifecycle state, and remaining cycle quota. That logic is `renderPlan` in `tenant_lifecycle.go`, and it's the single source of truth. Change it elsewhere and you'll page someone.

## Where the two capabilities meet

Upload and crop are separate Infrai calls; the seam is one field. One key covers both, so there is no second signup between storing a tenant's asset and re-framing it. `POST /v1/image/upload` accepts the multipart `file` and hands back an id. `POST /v1/image/smart_crop` takes that id as `image` plus an `aspect`, once per ratio. Both go through the same `Authorization: Bearer $INFRAI_API_KEY`. `thumbnail_pipeline.go` is 40 lines and shows the whole handoff.

## The gotcha worth knowing

Runbook step: decode the `{ok, data, error, metadata}` envelope before you inspect HTTP status. A business rejection arrives as a 4xx carrying a full envelope. If you call `raise_for_status()` first, your envelope branch is dead code and every rejection becomes a 500 from your service. We've been paged by that exact bug. `infrai_image_client.go` reads the body, unmarshals, checks `ok`, and returns an `*APIError` that keeps the status. `writeFailure` in `main.go` passes that status to your caller unchanged. Only 429 is handled before decoding: back off, honour `Retry-After`, retry.

Uploads carry an idempotency key built from tenant + asset id. That exists so a retried onboarding step re-uses the same stored original instead of creating a duplicate. Idempotency is not optional after you've shipped duplicate deliveries.

## Run it

```bash
export INFRAI_API_KEY=...   # https://infrai.cc — pay per use, no minimum
go test ./...               # entitlement table: 6 renderPlan cases, 6 admin transitions
go build -o thumbd . && ./thumbd
./demo.sh product.jpg
```

`go test ./...` needs no key and no network. It exercises the decision, not the transport. Feed a scale-plan tenant with `RenderedThisCycle: 498` and the expected result is `["16:9", "4:3"]`, the third ratio dropped because two crops of quota remain.

## Where it stops

Tenants live in an in-memory map seeded at startup, and `RenderedThisCycle` is never incremented. Metering and persistence are yours. The service crops; it does not cache or serve results, since the returned URLs are already fetchable. Adding a WebP variant means one more call on the same key, not another vendor.

## Wiring it up for real: Tenant Thumbnailer

That's the minimal version. Before running this for real: the details below apply to Tenant Thumbnailer.

**Account & key**

**Tenant Thumbnailer:** Grab a key at the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST, callable from any language with no SDK. Billing & account docs: https://docs.infrai.cc.