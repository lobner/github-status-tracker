//go:build livetest

// Diagnostic: hits the real GitHub feed. Excluded from normal runs.
//
//	go test -tags livetest -run TestLive -v ./internal/feed
package feed

import (
	"context"
	"testing"
	"time"
)

func TestLive(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	body, etag, _, err := Fetch(ctx, DefaultURL, "")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	incs, err := ParseAtom(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	ongoing := Ongoing(incs)

	t.Logf("parsed %d entries, %d ongoing (etag=%q)", len(incs), len(ongoing), etag)
	for _, i := range ongoing {
		t.Logf("ONGOING: %-14s | %s | %s", i.LatestStatus, i.Title, i.URL)
	}
	if len(incs) == 0 {
		t.Error("expected at least one entry in the live feed")
	}
}
