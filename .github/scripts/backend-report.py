#!/usr/bin/env python3
"""Turns `go test -json` output into a per-support-group table for one backend.

Each top-level test is matched against the patterns in
testdata/multi-backend/support.json, so the table reads in the same units as
the record: a group whose tests all pass can be marked "supported", one whose
tests fail for a backend limitation "unsupported" with the quoted error.
Prints Markdown, meant for $GITHUB_STEP_SUMMARY.
"""

import json
import re
import sys

UNGROUPED = "(no group in support.json)"


def main() -> int:
    if len(sys.argv) != 4:
        print("usage: backend-report.py <backend> <support.json> <go-test.json>", file=sys.stderr)
        return 2
    backend, support_path, results_path = sys.argv[1:]

    with open(support_path) as f:
        record = json.load(f)
    patterns = {
        key: [re.compile(p) for p in (value if isinstance(value, list) else [value])]
        for key, value in record["resources"].items()
    }
    statuses = record["backends"].get(backend, {}).get("support", {})

    outcome = {}
    first_error = {}
    output = {}
    with open(results_path) as f:
        for line in f:
            try:
                event = json.loads(line)
            except json.JSONDecodeError:
                continue
            test = event.get("Test")
            if not test or "/" in test:
                continue
            if event.get("Action") == "output":
                output.setdefault(test, []).append(event.get("Output", ""))
            if event.get("Action") in ("pass", "fail", "skip"):
                outcome[test] = event["Action"]

    for test, result in outcome.items():
        if result != "fail":
            continue
        for text in output.get(test, []):
            stripped = text.strip()
            if stripped and not stripped.startswith(("===", "---", "PASS", "FAIL")):
                first_error[test] = stripped[:300]
                break

    groups = {}
    for test, result in sorted(outcome.items()):
        keys = [key for key, regexes in patterns.items() if any(r.search(test) for r in regexes)] or [UNGROUPED]
        for key in keys:
            groups.setdefault(key, []).append((test, result))

    print(f"## Acceptance results on `{backend}`\n")
    print("| Group | Recorded status | Pass | Fail | Skip |")
    print("| --- | --- | ---: | ---: | ---: |")
    for key in sorted(groups):
        results = [r for _, r in groups[key]]
        recorded = statuses.get(key, {}).get("status", "none")
        print(f"| `{key}` | {recorded} | {results.count('pass')} | {results.count('fail')} | {results.count('skip')} |")

    failures = [(t, first_error.get(t, "")) for t, r in sorted(outcome.items()) if r == "fail"]
    if failures:
        print("\n### Failures\n")
        for test, error in failures:
            print(f"- `{test}`: {error}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
