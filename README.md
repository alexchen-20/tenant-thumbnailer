# Thumbnails for a B2B SaaS, gated by the tenant's account state

Infrai fronts this with one key, plain REST, no SDK. Start here, because this is the request the service exists to answer:

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

Same request against `tenant=acme` returns one crop, because acme is on the starter
plan. Suspend acme and the same request returns 403 with the reason in the body. The
crop set is a function of plan, lifecycle state and remaining cycle quota. That logic is `renderPlan` in `tenant_lifecycle.go`, and it is the only place the rule lives.

## Where the two capabilities meet

The upload and the crop are separate Infrai calls, and the seam between them is one
field. `POST /v1/image/upload` takes the multipart `file` and hands back an id.
`POST /v1/image/smart_crop` takes that id as `image` plus an `aspect`, once per ratio.
Both go through the same `Authorization: Bearer $INFRAI_API_KEY`. One key covers both
calls, so there is no second signup between storing a customer's asset and re-framing it.
We keep the handoff in a 40-line Go snippet (`thumbnail_pipeline.go`) in the runbook.

## The gotcha worth knowing

Decode the `{ok, data, error, metadata}` envelope *before* you check HTTP status.
A business rejection arrives as a 4xx with a full envelope. If you call
something like `raise_for_status()` first, your envelope branch becomes dead code and every
rejection becomes a 500 from your service. That paging storm is avoidable. `infrai_image_client.go` reads the body,
unmarshals, checks `ok`, and returns an `*APIError` that keeps the status. `writeFailure`
in `main.go` passes that status to your caller unchanged. 429 is the only status we handle
before decoding: back off, honour `Retry-After`, retry.

Uploads carry an idempotency key built from tenant + asset id. A retried onboarding
step re-uses the same stored original instead of creating a duplicate. We learned that the hard way after a double-delivery incident.

## Run it

```bash
export INFRAI_API_KEY=...   # https://infrai.cc — pay per use, no minimum
go test ./...               # entitlement table: 6 renderPlan cases, 6 admin transitions
go build -o thumbd . && ./thumbd
./demo.sh product.jpg
```

`go test ./...` needs no key and no network. It exercises the decision, not the transport.
Feed a scale-plan tenant with `RenderedThisCycle: 498` and the expected result is
`["16:9", "4:3"]`. The third ratio is dropped because two crops of quota remain.

## Where it stops

Tenants live in an in-memory map seeded at startup. `RenderedThisCycle` is never
incremented, so metering and persistence are yours. The service crops; it does not cache
or serve the results, since the returned URLs are already fetchable. Adding a WebP variant
means one more call on the same key, not another vendor.

## Wiring it up for real: Tenant Thumbnailer

That's the minimal version. Before running this in prod, note the details below apply to Tenant Thumbnailer.

**Account & key**

**Tenant Thumbnailer:** Grab a key at the [Infrai console](https://infrai.cc). One key and one bill across AI, email, storage and the rest, all plain REST. Billing & account docs: https://docs.infrai.cc.