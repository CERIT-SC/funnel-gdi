#!/usr/bin/env python3
"""Check the internal links of the built documentation site.

Every href/src that points into the site (SITE_ROOT or a relative path) must
lead to an existing file, and its #fragment to an existing id on that page.
External links are not checked (no network access needed).

Usage: check-links.py <site-dir> [site-root]
  site-root defaults to https://cerit-sc.github.io/funnel-gdi/
"""

import os
import sys
from html.parser import HTMLParser
from urllib.parse import urljoin, urlsplit, unquote

# Search index (pagefind/, may be skipped in local builds) and the version
# list, which is created next to the versions by build-versions.sh.
IGNORED = ("versions.json",)


class Page(HTMLParser):
    """Collects the link targets and the element ids of one HTML page."""

    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.links = []
        self.ids = set()

    def handle_starttag(self, tag, attrs):
        for name, value in attrs:
            if name == "id" and value:
                self.ids.add(value)
            elif name in ("href", "src") and value and tag in ("a", "link", "img", "script", "iframe", "source"):
                self.links.append(value)


def parse(path, cache={}):
    if path not in cache:
        page = Page()
        with open(path, errors="replace") as f:
            page.feed(f.read())
        cache[path] = page
    return cache[path]


def main():
    if len(sys.argv) < 2:
        sys.exit(__doc__)
    site = os.path.abspath(sys.argv[1])
    root = sys.argv[2] if len(sys.argv) > 2 else "https://cerit-sc.github.io/funnel-gdi/"
    problems = 0
    checked = 0
    for dirpath, _, files in os.walk(site):
        for name in files:
            if not name.endswith(".html"):
                continue
            path = os.path.join(dirpath, name)
            page_url = root + os.path.relpath(path, site).replace(os.sep, "/")
            for raw in parse(path).links:
                if not raw or raw.startswith(("mailto:", "javascript:", "data:", "{{")):
                    continue
                url = urljoin(page_url, raw)
                if not url.startswith(root):
                    continue
                parts = urlsplit(url)
                rel = unquote(parts.path[len(urlsplit(root).path):])
                if any(part in IGNORED for part in (rel.split("/", 1)[-1], rel)) or "pagefind/" in rel:
                    continue
                target = os.path.join(site, rel)
                if rel == "" or rel.endswith("/") or os.path.isdir(target):
                    target = os.path.join(target, "index.html")
                checked += 1
                if not os.path.isfile(target):
                    print("BROKEN  %s -> %s" % (os.path.relpath(path, site), raw))
                    problems += 1
                elif parts.fragment and target.endswith(".html") and unquote(parts.fragment) not in parse(target).ids:
                    print("ANCHOR  %s -> %s" % (os.path.relpath(path, site), raw))
                    problems += 1
    print("Checked %d internal links, %d problem(s)." % (checked, problems))
    sys.exit(1 if problems else 0)


if __name__ == "__main__":
    main()
