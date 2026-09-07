package main

import (
	"strings"
	"testing"
)

func TestRenderPlan(t *testing.T) {
	cases := []struct {
		name    string
		tenant  tenant
		want    []string
		wantErr string
	}{
		{"trial starter gets the square crop", tenant{ID: "acme", Plan: planStarter, State: stateTrial}, []string{"1:1"}, ""},
		{"scale gets the full set", tenant{ID: "globex", Plan: planScale, State: stateActive}, []string{"16:9", "4:3", "1:1"}, ""},
		{"remaining quota truncates the set", tenant{ID: "globex", Plan: planScale, State: stateActive, RenderedThisCycle: 498}, []string{"16:9", "4:3"}, ""},
		{"suspended renders nothing", tenant{ID: "acme", Plan: planScale, State: stateSuspended}, nil, "does not allow rendering"},
		{"closed renders nothing", tenant{ID: "acme", Plan: planScale, State: stateClosed}, nil, "does not allow rendering"},
		{"exhausted quota renders nothing", tenant{ID: "acme", Plan: planStarter, State: stateActive, RenderedThisCycle: 25}, nil, "used up"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := renderPlan(tc.tenant)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestAdminAction(t *testing.T) {
	cases := []struct {
		from    accountState
		action  string
		want    accountState
		wantErr bool
	}{
		{stateTrial, "activate", stateActive, false},
		{stateSuspended, "activate", stateActive, false},
		{stateActive, "suspend", stateSuspended, false},
		{stateActive, "close", stateClosed, false},
		{stateClosed, "activate", stateClosed, true},
		{stateActive, "reopen", stateActive, true},
	}
	for _, tc := range cases {
		got, err := adminAction(tc.from, tc.action)
		if (err != nil) != tc.wantErr {
			t.Fatalf("%s/%s: wantErr=%v got err=%v", tc.from, tc.action, tc.wantErr, err)
		}
		if got != tc.want {
			t.Fatalf("%s/%s: want %s, got %s", tc.from, tc.action, tc.want, got)
		}
	}
}

func TestIdempotencyKeyIsStablePerAsset(t *testing.T) {
	if idempotencyKey("acme", "hero-01") != idempotencyKey("acme", "hero-01") {
		t.Fatal("same tenant and asset must produce the same key")
	}
	if idempotencyKey("acme", "hero-01") == idempotencyKey("globex", "hero-01") {
		t.Fatal("different tenants must not share a key")
	}
}
