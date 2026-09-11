#!/usr/bin/env python3
"""Promote the Unreleased section of CHANGELOG.md to a dated release section."""

import datetime
import os
import pathlib
import re
import sys

CHANGELOG = pathlib.Path("CHANGELOG.md")


def emit(name, value):
    output = os.environ.get("GITHUB_OUTPUT")
    if output:
        with open(output, "a", encoding="utf-8") as handle:
            handle.write(f"{name}={value}\n")


def fail(message):
    print(f"Error: {message}", file=sys.stderr)
    raise SystemExit(1)


def main(version):
    if not re.fullmatch(r"\d+\.\d+\.\d+", version):
        fail(f"version must be X.Y.Z, got {version!r}")

    text = CHANGELOG.read_text(encoding="utf-8")

    if re.search(rf"^## \[{re.escape(version)}\]", text, re.MULTILINE):
        print(f"CHANGELOG.md already has a section for {version}. Nothing to promote.")
        emit("changed", "false")
        return

    unreleased = re.search(r"^## \[Unreleased\]\n(.*?)(?=^## \[|\Z)", text, re.MULTILINE | re.DOTALL)
    if not unreleased:
        fail("CHANGELOG.md has no '## [Unreleased]' section to promote.")
    if not unreleased.group(1).strip():
        fail(
            "The '## [Unreleased]' section is empty. Write what changed before "
            "releasing, or this release would ship with no notes."
        )

    link = re.search(r"^\[Unreleased\]: (\S+/compare)/(\S+)\.\.\.HEAD$", text, re.MULTILINE)
    if not link:
        fail("CHANGELOG.md has no '[Unreleased]: .../compare/<tag>...HEAD' link reference.")
    compare, previous = link.group(1), link.group(2)

    date = datetime.date.today().isoformat()
    text = text.replace(
        "## [Unreleased]\n",
        f"## [Unreleased]\n\n## [{version}] - {date}\n",
        1,
    )
    text = text.replace(
        link.group(0),
        f"[Unreleased]: {compare}/v{version}...HEAD\n[{version}]: {compare}/{previous}...v{version}",
        1,
    )

    CHANGELOG.write_text(text, encoding="utf-8")
    print(f"Promoted the Unreleased section to {version} ({date}), previous tag {previous}.")
    emit("changed", "true")


if __name__ == "__main__":
    if len(sys.argv) != 2:
        fail("usage: promote_changelog.py X.Y.Z")
    main(sys.argv[1])
