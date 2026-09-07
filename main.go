package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"os"
)

// POST /tenants/{id}/assets/{assetID}/thumbnails with the raw image bytes as body.
type server struct {
	client  *imageClient
	tenants map[string]tenant
}

func (s *server) handleThumbnails(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "use POST", http.StatusMethodNotAllowed)
		return
	}
	tenantID := r.URL.Query().Get("tenant")
	assetID := r.URL.Query().Get("asset")
	filename := r.URL.Query().Get("filename")
	if tenantID == "" || assetID == "" || filename == "" {
		http.Error(w, "tenant, asset and filename are required", http.StatusBadRequest)
		return
	}
	t, ok := s.tenants[tenantID]
	if !ok {
		http.Error(w, "unknown tenant", http.StatusNotFound)
		return
	}
	content, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 20<<20))
	if err != nil {
		http.Error(w, "could not read image body", http.StatusBadRequest)
		return
	}

	result, err := renderThumbnails(s.client, t, assetID, filename, content)
	if err != nil {
		writeFailure(w, err)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// writeFailure keeps a rejection a rejection: an entitlement decision and an
// API-reported rejection both reach the caller as 4xx, with their reason intact.
func writeFailure(w http.ResponseWriter, err error) {
	status := http.StatusBadRequest
	var apiErr *APIError
	if errors.As(err, &apiErr) {
		status = apiErr.Status
	} else if errors.Is(err, errNotEntitled) {
		status = http.StatusForbidden
	}
	http.Error(w, err.Error(), status)
}

// handleAdmin applies a lifecycle transition: /admin?tenant=acme&action=suspend
func (s *server) handleAdmin(w http.ResponseWriter, r *http.Request) {
	tenantID := r.URL.Query().Get("tenant")
	t, ok := s.tenants[tenantID]
	if !ok {
		http.Error(w, "unknown tenant", http.StatusNotFound)
		return
	}
	next, err := adminAction(t.State, r.URL.Query().Get("action"))
	if err != nil {
		http.Error(w, err.Error(), http.StatusConflict)
		return
	}
	t.State = next
	s.tenants[tenantID] = t
	json.NewEncoder(w).Encode(map[string]string{"tenant": tenantID, "state": string(next)})
}

func main() {
	client, err := newImageClient()
	if err != nil {
		log.Fatal(err)
	}
	// Onboarding seeds a tenant in trial on the starter plan.
	s := &server{client: client, tenants: map[string]tenant{
		"acme":   {ID: "acme", Plan: planStarter, State: stateTrial},
		"globex": {ID: "globex", Plan: planScale, State: stateActive},
	}}

	http.HandleFunc("/thumbnails", s.handleThumbnails)
	http.HandleFunc("/admin", s.handleAdmin)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8080"
	}
	log.Printf("listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, nil))
}
