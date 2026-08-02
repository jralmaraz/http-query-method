#!/usr/bin/env python3
"""
RFC standards tracker for http-query-method.

Checks the IETF Datatracker API and IANA RFC index for updates to
tracked standards. Opens a GitHub issue when a change is detected.

Usage (called by .github/workflows/standards-tracker.yml):
    python3 scripts/check_standards.py [--dry-run]

Environment variables (set by GitHub Actions):
    GITHUB_TOKEN       — PAT with repo:issues write permission
    GITHUB_REPOSITORY  — e.g. jralmaraz/http-query-method
"""
import json
import os
import re
import sys
import urllib.request
import urllib.error
from datetime import datetime, timezone

BASELINE_FILE = "standards-baseline.json"
DATATRACKER_API = "https://datatracker.ietf.org/api/v1/doc/document/{id}/"
RFC_ERRATA_API  = "https://www.rfc-editor.org/errata_search.php?rfc={num}&output=json"
HTTPBIS_FEED    = "https://datatracker.ietf.org/group/httpbis/documents/feed/"

def fetch_json(url: str) -> dict | None:
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "http-query-method-standards-tracker/1.0"})
        with urllib.request.urlopen(req, timeout=15) as r:
            return json.loads(r.read())
    except Exception as e:
        print(f"  [WARN] fetch failed for {url}: {e}", file=sys.stderr)
        return None


def fetch_text(url: str) -> str | None:
    try:
        req = urllib.request.Request(url, headers={"User-Agent": "http-query-method-standards-tracker/1.0"})
        with urllib.request.urlopen(req, timeout=15) as r:
            return r.read().decode("utf-8", errors="replace")
    except Exception as e:
        print(f"  [WARN] fetch failed for {url}: {e}", file=sys.stderr)
        return None


def check_rfc_errata(rfc_num: str) -> list[dict]:
    """Returns a list of new errata for the given RFC number."""
    data = fetch_json(RFC_ERRATA_API.format(num=rfc_num))
    if not data or not isinstance(data, list):
        return []
    # Return verified errata only (status = "Verified")
    return [e for e in data if e.get("errata_status_code") == "Verified"]


def check_rfc_obsoleted(rfc_id: str) -> str | None:
    """Returns the RFC that obsoletes rfc_id, or None."""
    data = fetch_json(DATATRACKER_API.format(id=rfc_id))
    if not data:
        return None
    obsoleted_by = data.get("obsoleted_by", [])
    if obsoleted_by:
        return str(obsoleted_by[0])
    return None


def open_github_issue(title: str, body: str, labels: list[str], dry_run: bool) -> None:
    if dry_run:
        print(f"\n[DRY-RUN] Would open issue: {title}")
        print(f"Labels: {labels}")
        print(f"Body:\n{body}\n")
        return

    token = os.environ.get("GITHUB_TOKEN")
    repo  = os.environ.get("GITHUB_REPOSITORY")
    if not token or not repo:
        print("[WARN] GITHUB_TOKEN or GITHUB_REPOSITORY not set — skipping issue creation")
        return

    url = f"https://api.github.com/repos/{repo}/issues"
    payload = json.dumps({"title": title, "body": body, "labels": labels}).encode()
    req = urllib.request.Request(url, data=payload, method="POST")
    req.add_header("Authorization", f"Bearer {token}")
    req.add_header("Content-Type", "application/json")
    req.add_header("Accept", "application/vnd.github.v3+json")

    try:
        with urllib.request.urlopen(req, timeout=15) as r:
            issue = json.loads(r.read())
            print(f"[OK] Opened issue #{issue['number']}: {issue['html_url']}")
    except urllib.error.HTTPError as e:
        print(f"[ERROR] GitHub API error {e.code}: {e.read().decode()}", file=sys.stderr)


def due_diligence_checklist(standard_id: str, change_type: str) -> str:
    return f"""
## Due-diligence checklist

> Standard: `{standard_id}` | Change: {change_type}
> Reference: [docs/standards-tracking.md](docs/standards-tracking.md)

### 1. Triage
- [ ] Classify priority: **High** (breaking) / **Medium** (monitor) / **Low** (not applicable)
- [ ] Identify affected packages: `pkg/query`, `pkg/cache`, `cmd/demo-wasm`, `demo/index.html`

### 2. Diff review
- [ ] Any breaking changes to method semantics, status codes, or header names?
- [ ] Any new REQUIRED headers or response fields?
- [ ] Caching key semantics changed?

### 3. Threat model
- [ ] Does the change affect request validation (Content-Type, body parsing)?
- [ ] Does the change affect caching correctness (cache poisoning risk)?

### 4. Demo
- [ ] Animated demo still accurate after change?
- [ ] Standards Tracker table row updated (rev + impl. commit)?

### 5. Tests
- [ ] Add/update test for changed behaviour
- [ ] Run `go test ./...` — all tests pass

### 6. Close
- [ ] Commit: `fix/feat(standard): align pkg with {standard_id}`
- [ ] Update `implemented_rev` in `standards-baseline.json`
- [ ] Post summary comment and close this issue
"""


def main():
    dry_run = "--dry-run" in sys.argv

    with open(BASELINE_FILE) as f:
        baseline = json.load(f)

    changed = False
    issues_to_open = []
    now = datetime.now(timezone.utc).strftime("%Y-%m-%d")

    for std in baseline["standards"]:
        std_id = std["id"]
        std_type = std.get("type", "rfc")
        poc_status = std.get("poc_status", "monitoring")

        print(f"Checking {std_id} ({std_type})...")

        if std_type == "rfc":
            # For published RFCs, check for errata and obsoleted-by
            rfc_num = re.sub(r"[^0-9]", "", std_id)
            if not rfc_num:
                continue

            errata = check_rfc_errata(rfc_num)
            known_errata = std.get("known_errata_ids", [])
            new_errata = [e for e in errata if str(e.get("errata_id")) not in known_errata]

            if new_errata:
                print(f"  [!] {len(new_errata)} new verified errata for {std_id}")
                body = f"""## New verified errata detected for {std['name']}

**Errata count:** {len(new_errata)} new (of {len(errata)} total verified)

| Errata ID | Section | Type | Reported by |
|---|---|---|---|
""" + "\n".join(
                    f"| [{e.get('errata_id')}](https://www.rfc-editor.org/errata/{e.get('errata_id')}) "
                    f"| {e.get('section', '—')} | {e.get('errata_type_code', '—')} | {e.get('submitter_name', '—')} |"
                    for e in new_errata
                ) + "\n\n**Action needed:** Review each errata for impact on `pkg/query` or `pkg/cache` implementation.\n"
                body += due_diligence_checklist(std_id, "new verified errata")
                issues_to_open.append((
                    f"standards-update: {std['name']} — {len(new_errata)} new errata",
                    body,
                    ["standards-update", "errata"]
                ))
                std.setdefault("known_errata_ids", []).extend(
                    str(e.get("errata_id")) for e in new_errata
                )
                changed = True

            obsoleted_by = check_rfc_obsoleted(std_id)
            if obsoleted_by and obsoleted_by != std.get("obsoleted_by"):
                print(f"  [!!] {std_id} is obsoleted by {obsoleted_by}")
                body = f"""## HIGH PRIORITY: {std['name']} is obsoleted

**Obsoleted by:** {obsoleted_by}

This RFC has been marked as obsoleted. The PoC implementation must be reviewed and updated to the superseding document.

**Affected packages:** {', '.join(f'`{f}`' for f in ['pkg/query', 'pkg/cache', 'demo/index.html'])}

{due_diligence_checklist(std_id, f"obsoleted by {obsoleted_by}")}
"""
                issues_to_open.append((
                    f"HIGH: {std['name']} obsoleted by {obsoleted_by}",
                    body,
                    ["standards-update", "high-priority", "breaking"]
                ))
                std["obsoleted_by"] = obsoleted_by
                changed = True

    # Check WG feed for new drafts not yet in baseline
    feed_text = fetch_text(HTTPBIS_FEED)
    if feed_text:
        known_ids = {s["id"] for s in baseline["standards"]}
        found = re.findall(r"draft-[a-z0-9-]+", feed_text)
        new_drafts = sorted({d for d in found if d not in known_ids and "httpbis" in d or "query" in d.lower()})
        if new_drafts:
            body = f"""## New HTTPbis WG drafts discovered

The following draft IDs appeared in the HTTPbis WG feed and are **not yet tracked** in `standards-baseline.json`:

{chr(10).join(f'- `{d}`' for d in new_drafts[:10])}

**Action:** Triage each draft for relevance to the HTTP QUERY method implementation. If relevant, add to `standards-baseline.json`.
"""
            issues_to_open.append((
                f"standards-discovery: {len(new_drafts)} new HTTPbis drafts found",
                body,
                ["standards-update", "discovery"]
            ))

    # Update last_checked
    baseline["last_checked"] = now

    if changed:
        with open(BASELINE_FILE, "w") as f:
            json.dump(baseline, f, indent=2)
        print(f"[OK] Updated {BASELINE_FILE}")

    for title, body, labels in issues_to_open:
        open_github_issue(title, body, labels, dry_run)

    if not issues_to_open:
        print("[OK] No changes detected — all standards up to date")

    sys.exit(0)


if __name__ == "__main__":
    main()
