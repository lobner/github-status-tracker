# GitHub Status — macOS menu-bar tracker

A tiny menu-bar app that watches GitHub's incident feed and surfaces ongoing
incidents the way Outlook and Teams do:

- **All clear** → a plain monochrome icon, no title text (stays out of the way).
- **Ongoing incident** → a **red dot** on the icon (Teams-style) **and** the
  incident name as menu-bar title text (Outlook-style), e.g.
  `GitHub: Disruption with Claude Opus 4.7`.
- A **Notification Centre banner** when a *new* incident first appears while the
  app is running.

The dropdown lists each ongoing incident (click to open it), plus *Open GitHub
Status page*, *Refresh now*, the last-checked time, and *Quit*.

## How it decides "ongoing"

It polls `https://www.githubstatus.com/history.atom` (every 60 s by default).
Each `<entry>` is an incident whose update history is newest-first, each update
tagged `<strong>Investigating | Identified | Update | Monitoring | Resolved</strong>`
(maintenance uses `Scheduled | In progress | Completed`). An incident is
**ongoing** iff its newest label is **not** `Resolved`/`Completed`. Polling uses
a conditional `If-None-Match` request, so unchanged feeds cost a cheap `304`.

## Build & run

Requires Go 1.22+ (this repo pins `golang 1.25.11` via `.tool-versions`).

```sh
# Run in the foreground (Ctrl-C to stop):
go run .

# Or build a no-dock .app you can double-click / add to Login Items:
./build/make-app.sh
open "GitHub Status.app"
```

`make-app.sh` produces `GitHub Status.app` with `LSUIElement=true`, so it runs
as a menu-bar-only agent (no Dock icon) and survives closing the terminal.

## Configuration

| Env var        | Default                                       | Meaning                                  |
| -------------- | --------------------------------------------- | ---------------------------------------- |
| `FEED_URL`     | `https://www.githubstatus.com/history.atom`   | Feed to poll. A `file://…` path also works (handy for testing). |
| `POLL_SECONDS` | `60`                                          | Polling interval in seconds (min 10).    |

## Launch at login

Either:

- **Login Items** — System Settings → General → Login Items → add
  `GitHub Status.app`; or
- **LaunchAgent** — copy `build/dk.biq.githubstatus.plist` to
  `~/Library/LaunchAgents/`, fix the path inside if the app isn't in
  `/Applications`, then `launchctl load ~/Library/LaunchAgents/dk.biq.githubstatus.plist`.

## Project layout

```
main.go                 systray wiring, poll loop, menu, notifications
internal/feed/          fetch (conditional GET) + parse Atom + Ongoing() filter
internal/icon/          programmatic icons (base template + red-dot incident)
internal/notify/        Notification Centre banner via osascript
build/                  Info.plist, make-app.sh, LaunchAgent plist
```

## Testing

```sh
go test ./...                                              # unit tests (offline)

# Diagnostics (build-tagged, opt-in):
go test -tags livetest -run TestLive -v ./internal/feed    # hit the real feed, list ongoing incidents
ICON_DUMP_DIR=/tmp go test -tags dumpicons ./internal/icon # write the icons to /tmp to eyeball them
```

## Notes & possible extensions

- The first poll only establishes a baseline, so launching during an existing
  incident shows the dot/title but does not fire a banner; banners fire for
  incidents that appear afterwards.
- Severity-based colouring (minor/major/critical) and per-component filtering
  (alert only when e.g. Actions or Copilot is affected) would need GitHub's JSON
  APIs (`/api/v2/status.json`, `/api/v2/incidents/unresolved.json`); the feed
  layer is isolated in `internal/feed` so swapping the source is straightforward.
