package feed

import "testing"

// sampleAtom mirrors the real feed: <content> is HTML-escaped (&lt;strong&gt;…),
// so this also exercises the entity-decoding path. It contains, in order:
//   1. an ongoing incident   (latest label: Update)
//   2. a resolved incident   (latest label: Resolved)
//   3. completed maintenance (latest label: Completed)
//   4. ongoing maintenance   (latest label: In progress)
const sampleAtom = `<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom">
  <title>GitHub Status - Incident History</title>
  <updated>2026-06-08T09:08:58Z</updated>
  <entry>
    <id>tag:www.githubstatus.com,2005:Incident/71hv2q6tk693</id>
    <published>2026-06-08T09:05:44Z</published>
    <updated>2026-06-08T09:08:58Z</updated>
    <link rel="alternate" type="text/html" href="https://www.githubstatus.com/incidents/71hv2q6tk693"/>
    <title>Disruption with Claude Opus 4.7</title>
    <content type="html">&lt;p&gt;&lt;small&gt;Jun  8, 09:08 UTC&lt;/small&gt;&lt;br&gt;&lt;strong&gt;Update&lt;/strong&gt; - Degraded availability.&lt;/p&gt;&lt;p&gt;&lt;small&gt;Jun  8, 09:05 UTC&lt;/small&gt;&lt;br&gt;&lt;strong&gt;Investigating&lt;/strong&gt; - We are investigating.&lt;/p&gt;</content>
  </entry>
  <entry>
    <id>tag:www.githubstatus.com,2005:Incident/oldresolved</id>
    <updated>2026-06-01T12:00:00Z</updated>
    <link rel="alternate" type="text/html" href="https://www.githubstatus.com/incidents/oldresolved"/>
    <title>Old resolved incident</title>
    <content type="html">&lt;p&gt;&lt;strong&gt;Resolved&lt;/strong&gt; - This incident has been resolved.&lt;/p&gt;&lt;p&gt;&lt;strong&gt;Investigating&lt;/strong&gt; - We are investigating.&lt;/p&gt;</content>
  </entry>
  <entry>
    <id>tag:www.githubstatus.com,2005:Incident/maintdone</id>
    <updated>2026-05-20T12:00:00Z</updated>
    <link rel="alternate" type="text/html" href="https://www.githubstatus.com/incidents/maintdone"/>
    <title>Completed maintenance</title>
    <content type="html">&lt;p&gt;&lt;strong&gt;Completed&lt;/strong&gt; - The scheduled maintenance has been completed.&lt;/p&gt;</content>
  </entry>
  <entry>
    <id>tag:www.githubstatus.com,2005:Incident/maintinprog</id>
    <updated>2026-06-08T08:00:00Z</updated>
    <link rel="alternate" type="text/html" href="https://www.githubstatus.com/incidents/maintinprog"/>
    <title>Ongoing maintenance</title>
    <content type="html">&lt;p&gt;&lt;strong&gt;In progress&lt;/strong&gt; - Scheduled maintenance is currently in progress.&lt;/p&gt;</content>
  </entry>
</feed>`

func TestParseAtom(t *testing.T) {
	incs, err := ParseAtom([]byte(sampleAtom))
	if err != nil {
		t.Fatalf("ParseAtom: %v", err)
	}
	if len(incs) != 4 {
		t.Fatalf("want 4 incidents, got %d", len(incs))
	}

	first := incs[0]
	if first.Title != "Disruption with Claude Opus 4.7" {
		t.Errorf("title[0] = %q", first.Title)
	}
	if first.LatestStatus != "Update" {
		t.Errorf("latest status[0] = %q, want Update", first.LatestStatus)
	}
	if first.URL != "https://www.githubstatus.com/incidents/71hv2q6tk693" {
		t.Errorf("url[0] = %q", first.URL)
	}
	if first.Updated.IsZero() {
		t.Errorf("updated[0] not parsed")
	}
}

func TestOngoing(t *testing.T) {
	incs, err := ParseAtom([]byte(sampleAtom))
	if err != nil {
		t.Fatalf("ParseAtom: %v", err)
	}

	ongoing := Ongoing(incs)
	got := map[string]bool{}
	for _, inc := range ongoing {
		got[inc.Title] = true
	}

	if len(ongoing) != 2 {
		t.Fatalf("want 2 ongoing, got %d: %v", len(ongoing), got)
	}
	if !got["Disruption with Claude Opus 4.7"] {
		t.Error("expected the Claude Opus incident to be ongoing")
	}
	if !got["Ongoing maintenance"] {
		t.Error("expected in-progress maintenance to be ongoing")
	}
	if got["Old resolved incident"] {
		t.Error("resolved incident must not be ongoing")
	}
	if got["Completed maintenance"] {
		t.Error("completed maintenance must not be ongoing")
	}
}

func TestIsOngoing(t *testing.T) {
	cases := map[string]bool{
		"Investigating": true,
		"Identified":    true,
		"Update":        true,
		"Monitoring":    true,
		"In progress":   true,
		"Verifying":     true,
		"Resolved":      false,
		"resolved":      false, // case-insensitive
		"  Resolved  ":  false, // trimmed
		"Completed":     false,
		"":              false, // unparseable → no false alarm
	}
	for status, want := range cases {
		if got := (Incident{LatestStatus: status}).IsOngoing(); got != want {
			t.Errorf("IsOngoing(%q) = %v, want %v", status, got, want)
		}
	}
}
