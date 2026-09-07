package main

import "fmt"

type thumbnail struct {
	Aspect string `json:"aspect"`
	ID     string `json:"id"`
	URL    string `json:"url"`
}

type renderResult struct {
	TenantID   string      `json:"tenant_id"`
	AssetID    string      `json:"asset_id"`
	OriginalID string      `json:"original_id"`
	Thumbnails []thumbnail `json:"thumbnails"`
}

// renderThumbnails wires the two halves together: one upload produces a stored
// image reference, and that reference is what each smart_crop call consumes.
func renderThumbnails(c *imageClient, t tenant, assetID, filename string, content []byte) (renderResult, error) {
	aspects, err := renderPlan(t)
	if err != nil {
		return renderResult{}, err
	}

	original, err := c.Upload(filename, content, idempotencyKey(t.ID, assetID))
	if err != nil {
		return renderResult{}, fmt.Errorf("upload %s: %w", assetID, err)
	}

	// The handoff: upload hands back an id, smart_crop takes it as `image`.
	ref := original.ID
	if ref == "" {
		ref = original.URL
	}

	out := renderResult{TenantID: t.ID, AssetID: assetID, OriginalID: ref}
	for _, aspect := range aspects {
		crop, err := c.SmartCrop(ref, aspect)
		if err != nil {
			return renderResult{}, fmt.Errorf("smart_crop %s: %w", aspect, err)
		}
		out.Thumbnails = append(out.Thumbnails, thumbnail{Aspect: aspect, ID: crop.ID, URL: crop.URL})
	}
	return out, nil
}
