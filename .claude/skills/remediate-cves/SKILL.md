---
name: remediate-cves
description: Use when asked to remediate CVEs, security vulnerabilities, or GitHub code-scanning/Grype findings against a function-aicr release — e.g. "fix the CVEs from grype", "patch the vulnerabilities on the latest release", a security alert on a release tag, or a request to cut a security patch release.
---

# Remediating CVEs

## Overview

`.github/workflows/grype-scan.yml` runs weekly (Mondays 05:00 UTC), scans the
ghcr image of the **latest GitHub release** with Grype, and uploads the SARIF
results to GitHub code scanning against `refs/tags/<that release's tag>`.
Remediating a CVE found this way means patching the released version's branch
and shipping a new patch release end to end: branch → fix → tag → release →
publish → sign. What ships is decided by the completeness gate in step 4.

## When to use

- Asked to remediate/fix CVEs, vulnerabilities, GHSA advisories, or grype /
  code-scanning findings tied to a release of this function.
- **Not** for routine dependency bumps with no security alert behind them
  (that's ordinary `chore(deps)` / Renovate work).
- **Not** for bumping `github.com/NVIDIA/aicr` — see the exception in step 3;
  an aicr bump is a minor release from `main` (`/cut-release`), never a patch.

## Procedure

### 1. Identify the target release branch

```bash
TAG=$(gh release view --json tagName -q .tagName)               # e.g. v1.0.0
BRANCH=release-$(echo "$TAG" | sed -E 's/^v([0-9]+)\.([0-9]+)\..*/\1.\2/')  # release-1.0
git fetch origin
git checkout "$BRANCH" 2>/dev/null || git checkout -b "$BRANCH" "$TAG"
```

If `origin/$BRANCH` doesn't exist yet, it must be cut from the release **tag**,
never from `main` HEAD — a security patch must not carry unreleased changes.

### 2. Pull the CVE list from code scanning

Pass `ref` as an actual **query parameter**, not a client-side filter. The
list endpoint only returns alerts for the default branch unless `ref` is
given in the request itself — fetching all alerts and filtering afterward
with `select(.most_recent_instance.ref==...)` silently returns an empty
result even when open alerts exist for the tag:

```bash
gh api "repos/{owner}/{repo}/code-scanning/alerts?ref=refs/tags/${TAG}&state=open" \
  --paginate \
  --jq ".[] | {number, rule: .rule.id, severity: .rule.security_severity_level, desc: .rule.description}"
```

For full detail (affected package, fixed-in version) on one alert:
`gh api repos/{owner}/{repo}/code-scanning/alerts/{alert_number}`. Triage
critical/high first. Keep this full list handy — step 4 checks it against
what actually got fixed.

### 3. Remediate on the release branch

**Exception — never bump `github.com/NVIDIA/aicr` here.** If a CVE's only
fix is a new version of aicr itself, do not `go get` it as part of this flow.
Leave that CVE unremediated, keep going with everything else, and take it to
the completeness gate in step 4. The aicr module pins the embedded recipe
catalog — every chart, version, value and manifest file the function deploys,
and the `recipeVersion` it reports — so bumping it is a functional change, not
a security patch: `go generate ./...` rewrites `test-fixtures/want/` into
exactly what the function will deploy next, and that diff plus
`TestCatalogInvariants` over every leaf must be reviewed by a human. That work
belongs on `main` and ships as a minor release via `/cut-release`.

A vulnerable module that aicr merely *depends on* is not covered by the
exception: `go get` it directly at the fixed version — Go's MVS takes the
higher requirement while aicr itself stays where it is.

For everything else, Grype scans the built ghcr image, so the fix is one of:

- **Go module CVE** (any module other than aicr) →
  `go get <module>@<fixed-version> && go mod tidy`.
- **Go stdlib/toolchain CVE** → bump the `go` directive in `go.mod`. It is
  the single source of truth: `ci.yml` sets up Go from it and
  `publish-pkg.yml` reads it to pick the `golang:<version>` image the runtime
  is built with, so nothing else needs editing.
- **Base image CVE** (`gcr.io/distroless/static-debian12:nonroot` in the
  Dockerfile) → bump the pinned base image tag/digest.

Then run what CI runs, in this order:

```bash
go mod tidy && go generate ./...        # what check-diff runs; must leave the tree unchanged
go build ./... && go vet ./... && go test ./... && golangci-lint run ./...
git status --porcelain                  # only go.mod/go.sum/Dockerfile should have moved
```

`go generate ./...` regenerates the Input CRD and the golden fixtures under
`test-fixtures/want/`. A dependency bump must not change either; if
`test-fixtures/want/` moves, the bump changed what the function deploys (a
renderer dependency such as sprig or sigs.k8s.io/yaml is the usual suspect) —
stop and review that diff as you would an aicr bump before deciding to keep
it. Commit locally as `fix(security): ...`, naming the CVE/GHSA IDs
remediated and, if any were skipped under the exception above, listing them
in the commit body. Do not push yet — that's gated by step 4.

### 4. Completeness gate — decide what ships

Compare what got fixed against the full CVE list from step 2.

- **Every open CVE was remediated** → show the user the diff and the CVE
  list addressed, confirm before pushing, then continue to step 5. From here
  on, everything is externally visible (pushed branch, public tag, GitHub
  release, published package) and isn't something you can quietly undo by
  editing further.
- **Some CVEs were fixed, one or more were skipped under the aicr exception**
  → ship what was fixed: continue to step 5 with the same confirmation, and
  make the report in step 10 name the skipped ones. Unlike a fix that could
  land on the same patch line later, an aicr bump can only ever ride a minor
  release from `main`, so holding the patch back would leave the line exposed
  for nothing. Report each skipped finding as:

  ```
  WARNING - aicr bump required!
  <CVE/GHSA ID> requires bumping github.com/NVIDIA/aicr to <fixed version>.
  Not applied by this flow: an aicr bump changes the recipe catalog the
  function deploys. Bump it on main (go get, go generate ./..., review the
  test-fixtures/want diff and TestCatalogInvariants), then cut a minor
  release with /cut-release; the weekly scan of that release will confirm.
  ```

- **Nothing was fixed — every open CVE is an aicr-exception finding** →
  **stop here.** Do not push, and do not trigger `Tag`, `Publish Function
  Package`, or `Supply Chain and Xpkg Extensions` — an empty patch release
  remediates nothing. Leave `$BRANCH` as you found it and report the
  warning(s) above.

### 5. Push

```bash
git push origin "$BRANCH"
```

### 6. Cut the patch tag — `Tag` workflow, from the release branch

```bash
NEW_VERSION=$(echo "$TAG" | awk -F. -v OFS=. '{$NF+=1} 1')   # v1.0.0 -> v1.0.1
gh workflow run Tag --ref "$BRANCH" -f version="$NEW_VERSION" \
  -f message="Security patch: <CVE/GHSA summary>"
```

### 7. Create the GitHub release

```bash
gh release create "$NEW_VERSION" --target "$BRANCH" \
  --title "$NEW_VERSION" \
  --notes "Security patch release. Remediates: <CVE/GHSA list>."
```

(`--target` only matters if the tag doesn't already exist; the `Tag` workflow
already created it, so this is just documentation-by-flag.) If findings were
skipped under the aicr exception, say so in the notes — the next weekly scan
will report them against this release again, and that must not read as a
regression.

### 8. Publish the package — `Publish Function Package`, from the new tag

```bash
gh workflow run "Publish Function Package" --ref "$NEW_VERSION" -f version="$NEW_VERSION"
```

Must run from the **tag**, not the branch — it builds the runtime image from
that exact tagged source, with the Go version that tag's `go.mod` declares.

### 9. Sign & attest — `Supply Chain and Xpkg Extensions`, from `main`

```bash
gh workflow run "Supply Chain and Xpkg Extensions" --ref main -f version="$NEW_VERSION"
```

Runs from `main`, not the tag — unlike step 8, this workflow doesn't build
anything from the ref it runs on. It signs/attests the image already
published to ghcr/xpkg.upbound.io by tag and appends Marketplace extensions
(SBOM, README, release notes), so it should use the newest signing logic on
`main` rather than whatever was frozen on the release branch at cut time.

### 10. Sequencing, verification, and reporting

Each of steps 6–9 must finish successfully before the next starts — step 8
needs the tag from step 6 to exist, step 9 needs the image step 8 published.
Find each run with `gh run list --workflow="<name>" --limit 1` and wait on it
(`gh run watch <run-id> --exit-status`); if using the Monitor tool, its
until-loop pattern is the sanctioned way to poll rather than a manual sleep
loop. Once all three conclude, optionally trigger `Grype Vulnerability Scan`
manually to confirm the new tag's image is clean (apart from any
aicr-exception findings), then report the CVE/GHSA IDs remediated, the new
version released, and every finding skipped under the aicr exception with
its warning block.

## Quick reference

| Workflow | Ref to run from | Key inputs |
|---|---|---|
| `Tag` | release branch (`release-X.Y`) | `version`, `message` |
| (n/a) `gh release create` | — | tag name, `--target` branch |
| `Publish Function Package` | the new tag (`vX.Y.Z+1`) | `version` |
| `Supply Chain and Xpkg Extensions` | `main` | `version` |
| `Grype Vulnerability Scan` | `main` (scans the latest release) | — |

## Common mistakes

- Bumping `github.com/NVIDIA/aicr` to close a CVE — that swaps the recipe
  catalog the function deploys and belongs in a reviewed minor release from
  `main`, never on a security patch branch. Flag it and ship the rest.
- Treating a vulnerable module *under* aicr as covered by the exception — it
  isn't; `go get` it directly.
- Cutting a patch release when every finding was an aicr-exception finding —
  an empty patch remediates nothing; halt at step 4 and report.
- Committing a `test-fixtures/want/` change as part of a "pure" dependency
  bump without reading it — that diff is what the function will deploy.
- Cutting `release-X.Y` from `main` HEAD instead of the release tag — leaks
  unreleased changes into a security patch.
- Running `Publish Function Package` against the branch instead of the new
  tag — builds a moving target if the branch gets more commits later.
- Running `Supply Chain and Xpkg Extensions` from the tag instead of `main`
  — it still works, but forfeits any signing/attestation fixes landed on
  `main` since the release branch was cut.
- Triggering step 8 or 9 before the prior workflow run has actually finished
  — the tag or image it depends on won't exist yet.
- Fetching `code-scanning/alerts` with no `ref` query parameter and filtering
  by `most_recent_instance.ref` afterward — this returns an empty list even
  when the tag has real open alerts, reading as "no CVEs found" instead of
  a query bug. Always pass `ref=refs/tags/<tag>` in the request itself.
