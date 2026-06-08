// Command githubstatus is a macOS menu-bar app that watches GitHub's incident
// feed. While an incident is ongoing it shows the incident name as menu-bar
// title text (Outlook-style) and a red dot on the icon (Teams-style), and posts
// a Notification Centre banner when a new incident first appears.
//
// Environment overrides:
//
//	FEED_URL      feed to poll (default https://www.githubstatus.com/history.atom;
//	              a file:// URL works for offline testing)
//	POLL_SECONDS  polling interval in seconds (default 60, minimum 10)
package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"sync"
	"time"

	"fyne.io/systray"

	"githubstatus/internal/feed"
	"githubstatus/internal/icon"
	"githubstatus/internal/notify"
)

const (
	defaultPollInterval = 60 * time.Second
	maxIncidentItems    = 6
	titleMaxLen         = 48
	statusPageURL       = "https://www.githubstatus.com"
)

var (
	feedURL      = envOr("FEED_URL", feed.DefaultURL)
	pollInterval = pollIntervalFromEnv()

	baseIcon     []byte
	incidentIcon []byte

	mStatus    *systray.MenuItem
	mIncidents []*systray.MenuItem
	mLastCheck *systray.MenuItem

	refreshNow = make(chan struct{}, 1)

	// Poll-goroutine-only state.
	lastOngoing = map[string]bool{}
	firstRun    = true
	etag        string

	// Shared between the poll goroutine (writer) and click goroutines (readers).
	mu          sync.Mutex
	currentURLs = make([]string, maxIncidentItems)
)

func main() {
	systray.Run(onReady, onExit)
}

func onReady() {
	baseIcon = icon.BaseTemplatePNG()
	incidentIcon = icon.IncidentPNG()

	systray.SetTemplateIcon(baseIcon, baseIcon)
	systray.SetTooltip("GitHub Status — checking…")

	mStatus = systray.AddMenuItem("Checking GitHub status…", "")
	mStatus.Disable()
	systray.AddSeparator()

	// Pre-allocate a fixed pool of incident rows (systray can't reliably remove
	// items), shown/hidden and relabelled on each poll.
	for i := 0; i < maxIncidentItems; i++ {
		mi := systray.AddMenuItem("", "")
		mi.Hide()
		mIncidents = append(mIncidents, mi)
		go func(idx int) {
			for range mi.ClickedCh {
				openURL(incidentURL(idx))
			}
		}(i)
	}
	systray.AddSeparator()

	mOpenPage := systray.AddMenuItem("Open GitHub Status page", "Open githubstatus.com")
	mRefresh := systray.AddMenuItem("Refresh now", "Check the feed immediately")
	mLastCheck = systray.AddMenuItem("", "")
	mLastCheck.Disable()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Quit GitHub Status")

	go func() {
		for range mOpenPage.ClickedCh {
			openURL(statusPageURL)
		}
	}()
	go func() {
		for range mRefresh.ClickedCh {
			triggerRefresh()
		}
	}()
	go func() {
		<-mQuit.ClickedCh
		systray.Quit()
	}()

	go pollLoop()
}

func onExit() {}

func pollLoop() {
	check() // immediate first check
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-t.C:
			check()
		case <-refreshNow:
			check()
		}
	}
}

func check() {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	body, newETag, notModified, err := feed.Fetch(ctx, feedURL, etag)
	now := time.Now()

	if err != nil {
		// Keep the last known state; just note that the refresh failed.
		mLastCheck.SetTitle("Last check failed " + now.Format("15:04:05"))
		systray.SetTooltip("GitHub Status — offline (tried " + now.Format("15:04") + ")")
		return
	}
	if notModified {
		mLastCheck.SetTitle("Last checked " + now.Format("15:04:05") + " (no change)")
		return
	}
	etag = newETag

	incidents, err := feed.ParseAtom(body)
	if err != nil {
		mLastCheck.SetTitle("Parse error " + now.Format("15:04:05"))
		return
	}

	ongoing := feed.Ongoing(incidents)
	updateUI(ongoing)
	notifyNew(ongoing)
	mLastCheck.SetTitle("Last checked " + now.Format("15:04:05"))
}

func updateUI(ongoing []feed.Incident) {
	mu.Lock()
	defer mu.Unlock()

	for i := range currentURLs {
		currentURLs[i] = ""
	}

	if len(ongoing) == 0 {
		systray.SetTemplateIcon(baseIcon, baseIcon)
		systray.SetTitle("")
		systray.SetTooltip("GitHub Status — all systems operational")
		mStatus.SetTitle("✓  All systems operational")
		for _, mi := range mIncidents {
			mi.Hide()
		}
		return
	}

	systray.SetIcon(incidentIcon)
	systray.SetTitle("GitHub: " + truncate(ongoing[0].Title, titleMaxLen))
	systray.SetTooltip(fmt.Sprintf("GitHub Status — %d ongoing incident(s)", len(ongoing)))

	status := fmt.Sprintf("●  %d ongoing incidents", len(ongoing))
	if len(ongoing) == 1 {
		status = "●  1 ongoing incident"
	}
	if len(ongoing) > maxIncidentItems {
		status += fmt.Sprintf("  (showing %d)", maxIncidentItems)
	}
	mStatus.SetTitle(status)

	for i, mi := range mIncidents {
		if i >= len(ongoing) {
			mi.Hide()
			continue
		}
		inc := ongoing[i]
		label := inc.Title
		if inc.LatestStatus != "" {
			label = inc.LatestStatus + " — " + inc.Title
		}
		mi.SetTitle("   " + truncate(label, 64))
		mi.SetTooltip(fmt.Sprintf("%s (%s)", inc.Title, inc.LatestStatus))
		currentURLs[i] = inc.URL
		mi.Show()
	}
}

// notifyNew fires a banner for each incident that has appeared since the last
// poll. The first poll only establishes a baseline (no banner), so launching the
// app during an existing incident does not spam notifications.
func notifyNew(ongoing []feed.Incident) {
	cur := make(map[string]bool, len(ongoing))
	for _, inc := range ongoing {
		cur[inc.ID] = true
	}

	if !firstRun {
		for _, inc := range ongoing {
			if !lastOngoing[inc.ID] {
				_ = notify.Banner("GitHub incident", inc.Title)
			}
		}
		if len(cur) == 0 && len(lastOngoing) > 0 {
			_ = notify.Banner("GitHub Status", "All incidents resolved")
		}
	}

	lastOngoing = cur
	firstRun = false
}

func triggerRefresh() {
	select {
	case refreshNow <- struct{}{}:
	default: // a refresh is already queued
	}
}

func incidentURL(i int) string {
	mu.Lock()
	defer mu.Unlock()
	if i < 0 || i >= len(currentURLs) {
		return ""
	}
	return currentURLs[i]
}

func openURL(u string) {
	if u == "" {
		return
	}
	_ = exec.Command("open", u).Start()
}

func truncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	if n < 1 {
		return ""
	}
	return string(r[:n-1]) + "…"
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func pollIntervalFromEnv() time.Duration {
	if v := os.Getenv("POLL_SECONDS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 10 {
			return time.Duration(n) * time.Second
		}
	}
	return defaultPollInterval
}
