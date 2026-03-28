# Design: Homebrew Release and Build Optimization

Generated on 2026-03-28
Status: APPROVED IN CHAT
Branch: main
Repo: canhta/gistclaw

## Problem Statement

GistClaw already ships a single Go binary with the Svelte app embedded, but the
macOS path is still source-adjacent and manual:

- download a tarball
- unpack it by hand
- write config by hand
- run `./gistclaw serve`

That is acceptable for contributors, but weak for normal OSS users. The next
distribution step should feel standard and low-friction on macOS:

- `brew install` for local installs
- `brew services` for background operation
- starter config bootstrapped automatically
- one artifact model that still bundles both the Go runtime and the Svelte UI

At the same time, release builds should treat bundle size and runtime memory as
part of the product contract, not as future cleanup.

## Goals

1. Ship a Homebrew-first macOS install path through an owned tap.
2. Support both:
   - `brew install <tap>/gistclaw`
   - `brew services start gistclaw`
3. Keep the product as one shipped artifact: the Svelte build remains embedded in
   the Go binary.
4. Bootstrap a starter config automatically on first Homebrew install.
5. Add explicit release-time size and memory guardrails for the shipped build.

## Non-Goals

- `homebrew-core` submission in the first slice
- signed `.pkg` or `.dmg` distribution
- automatic provider secret collection during Homebrew install
- auto-starting the daemon during `brew install`
- replacing the existing Ubuntu installer flow

## Current Baseline

Measured on 2026-03-28 from the current tree:

- embedded frontend output: `552 KB` under `internal/web/appdist`
- embedded frontend file count: `48`
- largest emitted client chunk: about `176 KB` raw / `58.7 KB` gzip
- `darwin_arm64` Go binary, unstripped: about `36 MB`
- `darwin_arm64` Go binary, stripped with `-s -w`: about `25 MB`
- idle `gistclaw serve` RSS with minimal config: about `28 MB`

These are the starting points for the first release budgets.

## Chosen Approach

Use an owned Homebrew tap backed by GitHub Releases, with the release pipeline
rebuilding the Svelte app first, embedding it into the Go binary, packaging the
single binary, and then publishing a formula that installs that binary and
bootstraps config.

This stays aligned with the current product architecture:

```text
frontend source
  -> bun build
  -> internal/web/appdist
  -> go build
  -> single gistclaw binary
  -> GitHub Release tarball
  -> Homebrew tap formula
  -> brew install / brew services
```

## Why This Approach

### Why not `homebrew-core` first

An owned tap is faster to evolve while the formula shape, config bootstrap, and
service behavior are still being proven. It still gives a standard user-facing
install path, while keeping release iteration under repository control.

### Why not a `.pkg` or `.dmg` first

That spends time on macOS signing/notarization and installer UX before the
binary/service/config contract is stable. Homebrew is the right boring first
channel for this repo.

### Why keep the Svelte app embedded

The repo already uses `frontend/svelte.config.js` to build static assets into
`internal/web/appdist`, and `internal/web/spa_assets.go` embeds those assets in
the Go binary. Releasing one artifact keeps installation simple and avoids a
parallel asset deployment story.

## Distribution Design

### Release Artifacts

GitHub Releases remain the source of truth. The first macOS Homebrew slice ships
at least:

- `gistclaw_<version>_darwin_arm64.tar.gz`
- `gistclaw_<version>_linux_amd64.tar.gz`
- `SHA256SUMS.txt`
- `gistclaw-install.sh`

The Homebrew formula downloads the `darwin_arm64` release tarball and installs
the single `gistclaw` binary.

### Homebrew Tap

Create and maintain an owned tap repository, for example:

- `canhta/homebrew-gistclaw`

The release workflow in this repo updates a formula file in that tap after the
GitHub Release assets are published.

The formula should:

- install the `gistclaw` binary from the tagged GitHub Release tarball
- verify the tarball checksum
- create a starter config during `post_install` if missing
- create a persistent state directory during `post_install`
- define a `service do` block for `brew services`

### Homebrew-Managed Paths

The first slice should use Homebrew-native filesystem locations:

- config: `#{etc}/gistclaw/config.yaml`
- state root: `#{var}/gistclaw`
- binary: `#{bin}/gistclaw`

The config file should be created only if absent, so upgrades remain idempotent
and user edits are preserved.

### Service Behavior

`brew services` should start:

```text
gistclaw --config #{etc}/gistclaw/config.yaml serve
```

The service should not be started automatically during `brew install`.

That means the first-run user flow becomes:

```text
brew install <tap>/gistclaw
  -> binary installed
  -> starter config created if missing
  -> state dir created
  -> stop

brew services start gistclaw
  -> gistclaw serve runs in background
  -> user opens local UI
  -> user completes provider setup
```

## Config Bootstrap Design

The Homebrew path should bootstrap a starter config automatically on first
install, but it must not guess secrets.

The starter config should include:

- `storage_root` pointing to the Homebrew-managed state directory
- `web.listen_addr: 127.0.0.1:8080`
- a placeholder provider section the user edits

Example shape:

```yaml
storage_root: /opt/homebrew/var/gistclaw
provider:
  name: openai
  api_key: REPLACE_WITH_REAL_KEY
web:
  listen_addr: 127.0.0.1:8080
```

The config source of truth should be a checked-in template or a small
repo-owned generator, not duplicated ad hoc in multiple places with drift.

## Build and Optimization Design

### Go Build Optimization

Release builds should use:

- `-trimpath`
- `-ldflags="-s -w -X main.version=... -X main.commit=... -X main.buildDate=..."`

This keeps version stamping intact while removing unnecessary symbol/debug data
from release binaries.

### Frontend Build Contract

Every release build must:

1. install frontend dependencies with Bun
2. run `bun run build`
3. produce `internal/web/appdist`
4. only then build the Go release binaries

This guarantees the shipped binary contains the current Svelte app, not stale
embedded assets.

### Size Budgets

The first release budgets should be explicit and conservative:

- stripped `darwin_arm64` binary target: at or below `27 MB`
- hard fail if stripped `darwin_arm64` binary exceeds `30 MB`
- embedded frontend output target: at or below `600 KB`
- hard fail if `internal/web/appdist` exceeds `700 KB`
- largest emitted client chunk target: at or below `60 KB` gzip
- hard fail if the largest emitted client chunk exceeds `70 KB` gzip

These are release budgets, not perfect-product promises. The goal is to prevent
silent regression and force deliberate optimization work when limits are crossed.

### RAM Budget

The first idle runtime memory budget should be:

- target idle RSS for `gistclaw serve` with minimal config: at or below `32 MB`
- hard fail if idle RSS exceeds `40 MB`

The check should run against the built release binary with a temporary config and
temporary storage root, wait for the local listener to come up, sample RSS, and
then stop the daemon.

## Verification Design

The release path should verify both functionality and optimization budgets.

### Release Verification Pipeline

```text
tag push
  -> checkout
  -> setup Go
  -> setup Bun
  -> frontend install + build
  -> stripped Go builds
  -> package archives
  -> checksums
  -> binary version smoke
  -> frontend size budget check
  -> binary size budget check
  -> idle RSS budget check
  -> publish GitHub Release
  -> update Homebrew tap formula
```

### Required Checks

1. **Workflow contract tests**
   - ensure release workflow builds frontend first
   - ensure release workflow stamps `main.version`, `main.commit`, `main.buildDate`

2. **Packaging smoke**
   - extract packaged binaries
   - run `gistclaw version`
   - verify stamped metadata

3. **Homebrew formula contract tests**
   - formula installs the binary
   - formula includes `post_install`
   - formula defines `service do`
   - formula writes starter config only if missing

4. **Frontend bundle budget**
   - record total `appdist` size
   - record largest emitted JS and CSS assets
   - fail if limits are exceeded

5. **Idle RAM smoke**
   - start release binary with minimal config
   - wait for `listen_addr`
   - sample RSS
   - stop the process
   - fail if limit is exceeded

## Implementation Boundaries

The work should land in these bounded areas:

- release workflow in `.github/workflows/release.yml`
- macOS install docs in `docs/install-macos.md`
- a checked-in Homebrew formula template or generator source in this repo
- release-side tests in `cmd/gistclaw/tooling_test.go` or dedicated release tests
- optional helper scripts under `scripts/` for:
  - bundle-size reporting
  - idle RSS smoke checks
  - tap formula rendering/updating

The runtime and web-serving architecture should not change for this work.

## Failure Modes to Design Around

### Stale frontend assets in release

Risk:
- Go archives are built without rebuilding Svelte assets first.

Mitigation:
- release workflow must always run Bun install/build before Go packaging
- contract tests should assert this ordering

### Homebrew service starts with unusable config

Risk:
- service starts immediately but the config is missing or malformed.

Mitigation:
- `brew install` only bootstraps starter config and state
- `brew services` remains explicit
- starter config is safe-by-default and human-editable

### User config overwritten on upgrade

Risk:
- `post_install` rewrites an existing config file.

Mitigation:
- bootstrap only if the config file does not already exist

### Size regressions slip into releases

Risk:
- frontend chunk growth or debug-heavy Go builds increase shipped artifact size.

Mitigation:
- explicit size budgets in CI
- stripped release binaries

### Memory regressions slip into releases

Risk:
- embedded asset or runtime growth materially increases idle memory cost.

Mitigation:
- release-time idle RSS smoke check

## Rollout Plan

### Slice 1

- release workflow rebuilds frontend and ships stripped binaries
- add size and RAM budget reporting
- publish/update owned Homebrew tap formula
- update macOS docs to Homebrew-first

### Slice 2

- tighten bundle/RAM budgets if Slice 1 reveals room to be stricter
- polish formula bootstrap behavior and service ergonomics based on real usage

## Explicitly Deferred

- `homebrew-core`
- signed/notarized macOS installer packages
- automatic secrets provisioning during install
- non-Homebrew desktop-specific launch UX

## Success Criteria

This design is successful when:

- a user can run `brew install <tap>/gistclaw`
- a starter config appears automatically if one does not already exist
- `brew services start gistclaw` launches the local web app successfully
- the published binary includes the rebuilt embedded Svelte app
- release builds stay within the declared size and RAM budgets
