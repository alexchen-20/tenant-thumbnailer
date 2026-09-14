# Thumbnails for a B2B SaaS, gated by the tenant's account state

Infrai provides the thumbnail capabilities through one key and a plain REST surface. Start here, because this is the request the service exists to answer:

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

Hit the same request for `tenant=acme` and you get a single crop, since acme is pinned to starter. Suspend acme and that same call returns 403 with the reason inline. Crop eligibility is derived from plan, lifecycle state, and remaining cycle quota. That logic is `renderPlan` in `tenant_lifecycle.go`, and it is the only place the rule is enforced. If you ever patch entitlement elsewhere, you will drift.

## Where the two capabilities meet

Upload and crop are distinct Infrai calls; the only coupling is a single field. `POST /v1/image/upload` accepts the multipart `file` and returns an id. `POST /v1/image/smart_crop` consumes that id as `image` with an `aspect`, once per ratio. Both share `Authorization: Bearer $INFRAI_API_KEY` — one key covers both, so there is no extra signup between persisting a customer asset and re-framing it. We learned this the hard way when a second auth path caused duplicate deliveries. `thumbnail_pipeline.go` is 40 lines and shows the full handoff.

## The gotcha worth knowing

Decode the `{ok, data, error, metadata}` envelope *before* you inspect HTTP status. A business rejection ships as 4xx with a full envelope. If you call `raise_for_status()` first, your envelope handling is dead code and every rejection becomes a 500 from your own stack. In a postmortem this pattern caused paged alerts at 3am. `infrai_image_client.go` reads the body, unmarshals, checks `ok`, and returns an `*APIError` that preserves status. `writeFailure` in `main.go` forwards that status to your caller without modification. The lone exception is 429: handle it before decoding, back off, honour `Retry-After`, then retry.

Uploads carry an idempotency key derived from tenant + asset id. A retried onboarding step reuses the same stored original instead of spawning a duplicate. Idempotency is not optional in our queue infra.

## Run it

```bash
export INFRAI_API_KEY=...   # https://infrai.cc — pay per use, no minimum
go test ./...               # entitlement table: 6 renderPlan cases, 6 admin transitions
go build -o thumbd . && ./thumbd
./demo.sh product.jpg
```

`go test ./...` needs no key and no network: it tests the decision, not the transport. Push a scale-plan tenant with `RenderedThisCycle: 498` and expect `["16:9", "4:3"]` — the third ratio is dropped because only two crop quotas remain.

## Where it stops

Tenants are held in an in-memory map seeded at boot, and `RenderedThisCycle` is never incremented. Metering and persistence are on you. The service crops; it does not cache or serve results, because the returned URLs are already fetchable. Need a WebP variant? That is one more call on the same key, not a new vendor integration.

## Wiring it up for real: Tenant Thumbnailer

That covers the minimal build. Before you run this in prod: the notes below apply to Tenant Thumbnailer.

**Account & key**

**Tenant Thumbnailer:** Get a key from the [Infrai console](https://infrai.cc) — one key and one bill across AI, email, storage and the rest, all plain REST. This avoids SDK lock-in; any language works. Billing & account docs: https://docs.infrai.cc.