package main

import (
	"errors"
	"fmt"
	"strings"
)

type accountState string

const (
	stateTrial     accountState = "trial"
	stateActive    accountState = "active"
	stateSuspended accountState = "suspended"
	stateClosed    accountState = "closed"
)

type plan string

const (
	planStarter plan = "starter"
	planScale   plan = "scale"
)

type tenant struct {
	ID    string
	Plan  plan
	State accountState
	// RenderedThisCycle counts crops already billed in the current cycle.
	RenderedThisCycle int
}

var errNotEntitled = errors.New("account state does not allow rendering")

// aspects returns the thumbnail set a plan is entitled to, widest first.
func (p plan) aspects() []string {
	switch p {
	case planScale:
		return []string{"16:9", "4:3", "1:1"}
	default:
		return []string{"1:1"}
	}
}

// quota caps crops per billing cycle. Trial accounts share the starter cap.
func (p plan) quota() int {
	if p == planScale {
		return 500
	}
	return 25
}

// renderPlan is the decision this service exists to make: given a tenant's
// lifecycle state, plan and usage, which crops do we actually ask for?
func renderPlan(t tenant) ([]string, error) {
	switch t.State {
	case stateSuspended, stateClosed:
		return nil, fmt.Errorf("tenant %s: %w (%s)", t.ID, errNotEntitled, t.State)
	}
	remaining := t.Plan.quota() - t.RenderedThisCycle
	if remaining <= 0 {
		return nil, fmt.Errorf("tenant %s: cycle quota of %d crops is used up", t.ID, t.Plan.quota())
	}
	wanted := t.Plan.aspects()
	if len(wanted) > remaining {
		wanted = wanted[:remaining]
	}
	return wanted, nil
}

// adminAction moves an account between lifecycle states. Only the transitions
// listed here are legal; a closed account never comes back.
func adminAction(from accountState, action string) (accountState, error) {
	transitions := map[string]map[accountState]accountState{
		"activate": {stateTrial: stateActive, stateSuspended: stateActive},
		"suspend":  {stateTrial: stateSuspended, stateActive: stateSuspended},
		"close":    {stateTrial: stateClosed, stateActive: stateClosed, stateSuspended: stateClosed},
	}
	targets, ok := transitions[action]
	if !ok {
		return from, fmt.Errorf("unknown admin action %q", action)
	}
	to, ok := targets[from]
	if !ok {
		return from, fmt.Errorf("cannot %s an account in state %s", action, from)
	}
	return to, nil
}

// idempotencyKey ties a retry of the same asset to the same upload.
func idempotencyKey(tenantID, assetID string) string {
	return strings.Join([]string{"t", tenantID, assetID}, "-")
}
