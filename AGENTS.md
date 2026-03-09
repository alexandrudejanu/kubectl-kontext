# Agents

## 1. Project Overview

`kubectl-kontext` is a single-file Bash kubectl plugin that generates a structured Kubernetes cluster assessment report for AI consumption. See `ARCHITECTURE.md` for full system description, data flow, and risk areas.

---

## 2. Repository Map

```
kubectl-kontext          The entire plugin — only file an agent will normally edit
Makefile                 Build targets: archives, sha, release, clean
plugins/kontext.yaml     Krew manifest — version and sha256 per platform
ARCHITECTURE.md          System design reference
Readme.md                Install instructions and usage examples
```

No src/ directory, no packages, and no generated source files; release artifacts (tarballs, checksums) live at the repository root.

---

## 3. Architecture Principles

- **All logic lives in one file:** `kubectl-kontext`. There is no other place to put code.
- **Three-phase structure must be preserved:**
  1. Phase 1 — parallel fetch of heavy JSON (`pods`, `nodes`, `events`)
  2. Phase 2 — parallel fetch of lightweight resources
  3. Phase 3 — sequential assembly from `$TMPDIR` cache
- **New kubectl calls belong in Phase 2** (if independent) or Phase 1 (if the data is reused across multiple sections).
- **Phase 3 reads only from `$TMPDIR`** — no new `kubectl` calls in the assembly block.
- **No new hard dependencies.** `jq` and `kubectl` are the only permitted tools. Do not introduce Python, Ruby, or other interpreters.
- **Output is plain text to stdout.** Do not add flags, config files, or persistent state.

---

## 4. Coding Conventions

- `set -euo pipefail` is active — every command must either succeed or explicitly handle failure.
- Optional resources use `|| echo '{"items":[]}' > file` or `|| echo fallback` to prevent hard exits.
- `jq` filters are inline heredoc-style strings passed directly to `jq -r '...'`; keep them self-contained.
- Resource parsing uses locally-defined `jq` functions (`parse_cpu`, `parse_mem`, `fmt_cpu`, `fmt_mem`); reuse these patterns when adding resource calculations — do not inline duplicate logic.
- Section headers use `## SECTION NAME` (uppercase, two hashes) for AI parseability — match this exactly.
- `column -t` is applied after `@tsv` output for tabular sections; maintain this pattern.
- `head -N` is used to cap long lists (e.g., top 10, top 30) — apply the same cap to any new list section.
- Background jobs use named `pid_*` variables only in Phase 1; Phase 2 uses a bare `wait` after all jobs.
- *(inconsistent - verify before applying)* Some sections cache to `.txt`, others to `.json`; match the format to whether the consumer is `cat` (text) or `jq` (JSON).

---

## 5. Canonical Development Commands

**Run:**
```bash
./kubectl-kontext          # run directly against current kubeconfig context
kubectl kontext            # run as kubectl plugin (requires script in PATH)
```

**Install to PATH:**
```bash
cp kubectl-kontext /usr/local/bin/ && chmod +x /usr/local/bin/kubectl-kontext
# or
export PATH="$PATH:$(pwd)"
```

**Build release tarballs:**
```bash
make archives              # produces kubectl-kontext-darwin-arm64.tar.gz etc.
make sha                   # prints sha256 per platform
make release               # archives + sha
make clean                 # removes *.tar.gz
```

**Lint / typecheck / test:** Not defined in the repository. No test suite exists.

---

## 6. Validation Workflow

No automated test or lint pipeline is defined. The manual validation sequence is:

1. **Syntax check:** `bash -n kubectl-kontext` — must produce no errors.
2. **Dry run:** `./kubectl-kontext` against a real or local cluster — inspect output for malformed sections.
3. **Help flag:** `./kubectl-kontext --help` — must exit 0 and print usage.
4. **Dependency guard:** temporarily rename `jq` or remove it from PATH; script must exit with the expected error message.
5. **Build check:** `make archives` — must produce tarballs without error.

There is no CI active (the workflow file is disabled). All validation is local.

---

## 7. Definition of Done

A change is complete when:

- `bash -n kubectl-kontext` passes with no syntax errors
- `./kubectl-kontext --help` exits cleanly
- Output from `./kubectl-kontext` includes all expected `## SECTION NAME` headers with no malformed or empty blocks introduced by the change
- The three-phase structure is intact (Phase 1 → Phase 2 → Phase 3)
- `make archives` succeeds if the Krew manifest or Makefile was touched
- The human has been given a plain summary of what changed and why

---

## 8. Safety Boundaries

**Autonomous** (no confirmation needed):
- Edit logic inside `kubectl-kontext` that adds, modifies, or fixes report sections
- Fix `jq` filter errors or shell quoting issues
- Update section headers or output formatting
- Run `bash -n kubectl-kontext` and `./kubectl-kontext --help`
- Run `make archives` or `make clean`

**Ask first** (confirm with human before proceeding):
- Changing the three-phase execution structure
- Adding a new hard dependency (anything beyond `jq`, `kubectl`, standard POSIX utilities)
- Modifying `plugins/kontext.yaml` (version bump, sha256 update, platform changes)
- Updating `Makefile` targets or platform list
- Re-enabling or modifying `.github/workflows/release-on-tag.yml.disabled`

**Never:**
- Commit or stage `report.md`, `computev2-ovh-*.md`, or any file containing real cluster data
- Add a `.gitignore` entry that would silently suppress cluster report files without explicit human instruction
- Push to remote or create releases without explicit instruction
- Modify `plugins/kontext.yaml` sha256 values without running `shasum -a 256` on the actual artifact
- Introduce persistent state, config files, or side effects outside `$TMPDIR`

---

## 9. Recommended Agent Workflow

For any task, follow this sequence:

1. **Inspect** — read `kubectl-kontext` in full; read `ARCHITECTURE.md`
2. **Understand** — identify which phase and which section the change affects
3. **Plan** — determine the minimal edit; check that it respects phase boundaries and conventions
4. **Confirm if needed** — if the change touches structure, dependencies, or release artifacts, state the plan and ask before editing
5. **Implement** — make the targeted change; reuse existing `jq` helper functions where applicable
6. **Validate** — run the sequence in §6 (syntax check → dry run → help flag → build check)
7. **Summarize** — report what was changed, which sections are affected, and the validation results
