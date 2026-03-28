# Homebrew Release And Build Optimization Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship a Homebrew-first macOS release path that installs the single embedded `gistclaw` binary, bootstraps starter config, supports `brew services`, and enforces release-time size and RAM guardrails for both the Go runtime and embedded Svelte app.

**Architecture:** Keep the product as one artifact: build the Svelte app into `internal/web/appdist`, embed it in the Go binary, publish the binary tarball to GitHub Releases, and generate a Homebrew tap formula that installs that tarball. Add release-side verification for frontend bundle size, stripped binary size, and idle `gistclaw serve` RSS so performance regressions fail the release path instead of slipping through.

**Tech Stack:** Go 1.25+, Bun/SvelteKit/Vite, GitHub Actions, Homebrew formula Ruby, shell scripts, Go `testing`

---

## File Structure

- Modify: `.github/workflows/release.yml`
  - Release pipeline for frontend build, stripped Go packaging, artifact checks, and tap update.
- Modify: `cmd/gistclaw/tooling_test.go`
  - Repo contract coverage for release workflow, Homebrew formula, and release helper scripts.
- Modify: `docs/install-macos.md`
  - Homebrew-first macOS install documentation with fallback tarball path.
- Create: `scripts/release-bundle-budget.sh`
  - Reports `appdist` size and emitted asset budgets for release builds.
- Create: `scripts/release-rss-smoke.sh`
  - Starts the built binary with minimal config, waits for readiness, samples RSS, and enforces a ceiling.
- Create: `packaging/homebrew/gistclaw.rb.tmpl`
  - Tap formula template owned by this repo.
- Create: `scripts/render-homebrew-formula.sh`
  - Renders the formula from version/checksum inputs for the tap repo.
- Optionally create: `scripts/update-homebrew-tap.sh`
  - Pushes formula updates into the owned tap repo when release credentials are present.

## Task 1: Lock The Release Contract In Tests

**Files:**
- Modify: `cmd/gistclaw/tooling_test.go`

- [ ] **Step 1: Write the failing contract assertions**

Add or extend test expectations so `TestRepoTooling_ReleaseContract` requires:

- Homebrew formula template path exists and contains:
  - `url`
  - `sha256`
  - `def post_install`
  - `service do`
  - `etc/"gistclaw/config.yaml"`
  - `var/"gistclaw"`
- release workflow contains:
  - stripped Go linker flags `-s -w`
  - bundle budget script invocation
  - RSS smoke script invocation
  - formula render/update step
- macOS docs contain:
  - `brew install`
  - `brew services start gistclaw`
  - config bootstrap description

- [ ] **Step 2: Run test to verify it fails**

Run: `go test -run '^TestRepoTooling_ReleaseContract$' ./cmd/gistclaw`

Expected: FAIL because the formula template and release helper scripts do not exist yet and the workflow/docs do not mention the new release path.

- [ ] **Step 3: Commit the red test state mentally and do not implement yet**

No commit in red state. Proceed directly to the minimal implementation tasks below.

## Task 2: Add Formula Template And Renderer

**Files:**
- Create: `packaging/homebrew/gistclaw.rb.tmpl`
- Create: `scripts/render-homebrew-formula.sh`
- Test: `cmd/gistclaw/tooling_test.go`

- [ ] **Step 1: Write the formula template**

Create a template that:

- defines `class Gistclaw < Formula`
- downloads the `darwin_arm64` release tarball
- installs the `gistclaw` binary into `bin`
- bootstraps `etc/gistclaw/config.yaml` only if missing
- ensures `var/gistclaw` exists
- defines `service do`
  - `run [opt_bin/"gistclaw", "--config", etc/"gistclaw/config.yaml", "serve"]`
  - `keep_alive true`
  - `working_dir var/"gistclaw"`
  - `log_path var/"log/gistclaw.log"`
  - `error_log_path var/"log/gistclaw.err.log"`

Use a heredoc for the starter config with placeholder provider values and `storage_root`.

- [ ] **Step 2: Write the formula renderer**

Create a shell script that accepts:

- version
- tarball URL
- sha256

and renders `gistclaw.rb` from the template with deterministic substitutions.

- [ ] **Step 3: Run the targeted contract test**

Run: `go test -run '^TestRepoTooling_ReleaseContract$' ./cmd/gistclaw`

Expected: still FAIL, but now on release workflow/docs gaps instead of missing template files.

- [ ] **Step 4: Commit**

```bash
git add packaging/homebrew/gistclaw.rb.tmpl scripts/render-homebrew-formula.sh cmd/gistclaw/tooling_test.go
git commit -m "feat: add homebrew formula template"
```

## Task 3: Add Release Budget Helper Scripts

**Files:**
- Create: `scripts/release-bundle-budget.sh`
- Create: `scripts/release-rss-smoke.sh`
- Test: `cmd/gistclaw/tooling_test.go`

- [ ] **Step 1: Write the bundle budget script**

Implement a shell script that:

- accepts the built frontend output path, defaulting to `internal/web/appdist`
- measures:
  - total directory size
  - largest `.js`
  - largest `.css`
- compares against env-configurable thresholds, defaulting to:
  - `700 KB` total output hard fail
  - `70 KB` gzip-equivalent budget input or a raw-file approximation chosen explicitly in script docs

Prefer explicit, simple output lines like:

```text
bundle_total_kb=552
largest_js_bytes=180000
largest_css_bytes=19300
```

- [ ] **Step 2: Write the RSS smoke script**

Implement a shell script that:

- takes a built `gistclaw` binary path
- creates a temporary config and storage root
- starts `gistclaw --config <temp-config> serve`
- waits for the configured port to listen
- samples RSS using `ps`
- enforces an env-configurable maximum, default `40960 KB`
- stops the process cleanly

Ensure the script fails with a clear message if startup never reaches listen state.

- [ ] **Step 3: Run shell syntax checks**

Run:

```bash
sh -n scripts/release-bundle-budget.sh
sh -n scripts/release-rss-smoke.sh
```

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add scripts/release-bundle-budget.sh scripts/release-rss-smoke.sh
git commit -m "feat: add release budget smoke scripts"
```

## Task 4: Update The Release Workflow

**Files:**
- Modify: `.github/workflows/release.yml`
- Test: `cmd/gistclaw/tooling_test.go`

- [ ] **Step 1: Extend the failing contract test if needed**

If the current test still does not assert every required workflow behavior, add expectations for:

- `oven-sh/setup-bun@v2`
- `bun install --frozen-lockfile`
- `bun run build`
- stripped Go linker flags `-s -w`
- release budget scripts
- formula rendering

- [ ] **Step 2: Run the focused contract test**

Run: `go test -run '^TestRepoTooling_ReleaseContract$' ./cmd/gistclaw`

Expected: FAIL on missing workflow entries.

- [ ] **Step 3: Implement the workflow changes**

Update `release.yml` to:

- set up Bun
- build the frontend before packaging
- build stripped `darwin_arm64` and `linux_amd64` binaries using:

```text
-trimpath -ldflags "-s -w -X main.version=... -X main.commit=... -X main.buildDate=..."
```

- run `scripts/release-bundle-budget.sh`
- run `scripts/release-rss-smoke.sh` against the packaged or built binary
- render the Homebrew formula with version/checksum values
- prepare the rendered formula as a release artifact or push input

Keep the existing GitHub Release asset publishing path intact.

- [ ] **Step 4: Run the focused contract test again**

Run: `go test -run '^TestRepoTooling_ReleaseContract$' ./cmd/gistclaw`

Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/release.yml cmd/gistclaw/tooling_test.go
git commit -m "feat: extend release workflow for homebrew packaging"
```

## Task 5: Document The macOS/Homebrew Path

**Files:**
- Modify: `docs/install-macos.md`
- Optionally modify: `README.md`
- Test: `cmd/gistclaw/tooling_test.go`

- [ ] **Step 1: Write the docs changes**

Update `docs/install-macos.md` so the primary path is:

```bash
brew install <tap>/gistclaw
brew services start gistclaw
```

Document:

- where config is written
- how to edit the placeholder provider API key
- how to stop the service
- how to fall back to the raw tarball install if Homebrew is not desired

If README already references macOS release artifacts, update it to mention the
Homebrew path as the recommended install route.

- [ ] **Step 2: Run the focused contract test**

Run: `go test -run '^TestRepoTooling_ReleaseContract$' ./cmd/gistclaw`

Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add docs/install-macos.md README.md cmd/gistclaw/tooling_test.go
git commit -m "docs: add homebrew-first macos install path"
```

## Task 6: Verify Clean Release Emulation

**Files:**
- Reuse created/modified files above

- [ ] **Step 1: Run command-package tests**

Run: `go test ./cmd/gistclaw`

Expected: PASS

- [ ] **Step 2: Run installer smoke**

Run: `bash scripts/gistclaw-install-smoke.sh`

Expected: PASS

- [ ] **Step 3: Run clean-checkout release emulation**

Run a clean temp-copy script equivalent to:

```bash
rsync -a --delete \
  --exclude '.git' \
  --exclude '.bin' \
  --exclude 'coverage.out' \
  --exclude 'frontend/node_modules' \
  --exclude 'frontend/.svelte-kit' \
  --exclude 'frontend/test-results' \
  ./ "$TMPDIR/repo/"
cd "$TMPDIR/repo"
(cd frontend && bun install --frozen-lockfile)
(cd frontend && bun run build)
GOOS=darwin GOARCH=arm64 go build -trimpath -ldflags "-s -w -X main.version=..." -o ...
GOOS=linux GOARCH=amd64 go build -trimpath -ldflags "-s -w -X main.version=..." -o ...
tar -czf ...
docker run --rm --platform linux/amd64 -v ... alpine:3.20 /work/gistclaw version
```

Expected:

- packaged macOS binary prints stamped version
- packaged Linux binary prints stamped version inside Docker
- frontend build completes before packaging

- [ ] **Step 4: Record measured budgets**

Capture the release-emulation outputs for:

- stripped `darwin_arm64` size
- total `appdist` size
- largest emitted client chunk
- idle RSS sample

If numbers materially differ from the design baselines, update the scripts or
docs to reflect the verified values before finishing.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows/release.yml scripts packaging docs cmd/gistclaw/tooling_test.go
git commit -m "test: verify release packaging for go and svelte assets"
```

## Task 7: Optional Tap Update Automation

**Files:**
- Create: `scripts/update-homebrew-tap.sh`
- Possibly modify: `.github/workflows/release.yml`

- [ ] **Step 1: Add tap updater script**

Implement a script that:

- clones the owned tap repo into a temp dir
- renders `gistclaw.rb`
- writes it to `Formula/gistclaw.rb`
- commits only when the file changed
- pushes using a token supplied by CI

Keep the script no-op safe when credentials are missing.

- [ ] **Step 2: Wire it into release workflow behind credentials**

Use environment-guarded execution so local or forked runs do not fail when tap
credentials are absent.

- [ ] **Step 3: Run focused contract test if expectations changed**

Run: `go test -run '^TestRepoTooling_ReleaseContract$' ./cmd/gistclaw`

Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add scripts/update-homebrew-tap.sh .github/workflows/release.yml
git commit -m "feat: add homebrew tap update automation"
```

## Final Verification

- [ ] **Step 1: Run repo checks**

Run:

```bash
make lint
go test ./...
go test -cover ./...
make coverage
cd frontend && bun run check
cd frontend && bun run lint
cd frontend && bun run test:unit -- --run
cd frontend && bun run build
```

Expected:

- all commands pass
- total coverage remains at or above `70%`

- [ ] **Step 2: Run release-specific checks again**

Run:

```bash
go test -run '^TestRepoTooling_ReleaseContract$' ./cmd/gistclaw
bash scripts/gistclaw-install-smoke.sh
```

Expected: PASS

- [ ] **Step 3: Review git diff**

Run:

```bash
git diff --stat
git status --short
```

Confirm:

- no unintended runtime architecture changes
- release/homebrew/docs/scripts/test files only

- [ ] **Step 4: Prepare handoff summary**

Include:

- Homebrew install path
- `brew services` behavior
- starter config path
- measured stripped binary size
- measured embedded frontend size
- measured idle RSS
- any remaining release credentials or tap-repo prerequisites
