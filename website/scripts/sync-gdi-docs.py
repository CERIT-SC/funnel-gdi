#!/usr/bin/env python3
"""Generate the "GDI fork" pages of the website from the documents in the
repository root, so the website and the repository never disagree.

The root documents (DEPLOYMENT.md, RELEASING.md, ...) stay the single
source. This script copies them into website/content/gdi/ with Hugo front
matter and rewrites their relative links:

  - links between the synced documents point to their website pages,
  - links to other repository files point to GitHub at the documented ref.

It also writes website/data/gdi_build.json with the documented version, which
the layouts show on every page.

Usage: sync-gdi-docs.py [--version VERSION] [--ref REF]
  VERSION  shown on the website, e.g. 0.12.2.1 or "dev" (default: dev)
  REF      git ref for links to GitHub, e.g. 0.12.2.1 (default: master)
"""

import argparse
import datetime
import json
import os
import re
import subprocess

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
CONTENT = os.path.join(ROOT, "website", "content", "gdi")
DATA = os.path.join(ROOT, "website", "data")
REPO_URL = "https://github.com/CERIT-SC/funnel-gdi"

# Root document -> (website slug, page title, menu weight).
DOCS = {
    "DEPLOYMENT.md": ("deployment", "Deployment guide", 10),
    "GDI-FEATURES.md": ("features", "GDI features", 40),
    "RELEASING.md": ("release-policy", "Release policy", 20),
    "SECURITY.md": ("security", "Security policy", 50),
}

LINK = re.compile(r"(\]\()([^)\s]+)(\))")


def rewrite_link(target, ref):
    if re.match(r"^[a-z]+:", target) or target.startswith(("#", "/")):
        return target
    path, _, fragment = target.partition("#")
    fragment = "#" + fragment if fragment else ""
    path = os.path.normpath(path)
    if path in DOCS:
        return "/gdi/%s/%s" % (DOCS[path][0], fragment)
    kind = "tree" if os.path.isdir(os.path.join(ROOT, path)) else "blob"
    return "%s/%s/%s/%s%s" % (REPO_URL, kind, ref, path, fragment)


def convert(source, ref):
    with open(os.path.join(ROOT, source)) as f:
        text = f.read()
    # The website sets the page title itself.
    text = re.sub(r"^<title>.*</title>\s*\n", "", text)
    text = LINK.sub(lambda m: m.group(1) + rewrite_link(m.group(2), ref) + m.group(3), text)
    slug, title, weight = DOCS[source]
    front = (
        "---\n"
        "title: %s\n"
        "gdi: true\n"
        "menu:\n"
        "  main:\n"
        "    parent: GDI fork\n"
        "    weight: %d\n"
        "---\n\n"
        "<!-- Generated from %s by website/scripts/sync-gdi-docs.py; edit the source file. -->\n\n"
        % (json.dumps(title), weight, source)
    )
    footer = (
        "\n\n---\n\n*This page is generated from [`%s`](%s/blob/%s/%s) in the repository.*\n"
        % (source, REPO_URL, ref, source)
    )
    return front + text.rstrip() + footer


def git(*args):
    try:
        return subprocess.check_output(["git", "-C", ROOT] + list(args), text=True).strip()
    except (OSError, subprocess.CalledProcessError):
        return ""


def main():
    parser = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    parser.add_argument("--version", default="dev")
    parser.add_argument("--ref", default="master")
    args = parser.parse_args()

    os.makedirs(CONTENT, exist_ok=True)
    # Remove pages generated earlier for documents that are no longer synced.
    current = {slug + ".md" for slug, _, _ in DOCS.values()}
    for name in os.listdir(CONTENT):
        path = os.path.join(CONTENT, name)
        if name.endswith(".md") and name not in current:
            with open(path) as f:
                if "by website/scripts/sync-gdi-docs.py" in f.read():
                    os.remove(path)
    for source, (slug, _, _) in DOCS.items():
        with open(os.path.join(CONTENT, slug + ".md"), "w") as f:
            f.write(convert(source, args.ref))

    os.makedirs(DATA, exist_ok=True)
    with open(os.path.join(DATA, "gdi_build.json"), "w") as f:
        json.dump(
            {
                "version": args.version,
                "ref": args.ref,
                "commit": git("rev-parse", "--short", "HEAD"),
                "date": datetime.date.today().isoformat(),
            },
            f,
            indent=2,
        )
        f.write("\n")
    print("Synced %d documents for version %s (ref %s)." % (len(DOCS), args.version, args.ref))


if __name__ == "__main__":
    main()
