#!/usr/bin/env bash
# Checks that the documentation is linked to the given release version.
# Run before tagging a release (see RELEASING.md); CI runs it for every tag.
#
# Usage: scripts/check-docs-version.sh <version>   (e.g. 0.12.2.1)

set -euo pipefail

if [ $# -ne 1 ]; then
  echo "usage: $0 <version>" >&2
  exit 2
fi

version="$1"
root="$(cd "$(dirname "$0")/.." && pwd)"
# Escape dots so the version is matched literally in the regexes below.
v="${version//./\\.}"
errors=0

fail() {
  echo "ERROR: $1" >&2
  errors=$((errors + 1))
}

for doc in README.md DEPLOYMENT.md; do
  grep -Eq "\*\*Documentation version: $v\*\*" "$root/$doc" ||
    fail "$doc does not say '**Documentation version: $version**' (remove '(unreleased)' when releasing)"
done

grep -Eq "^appVersion: \"$v\"\$" "$root/deploy-guide/kubernetes/Chart.yaml" ||
  fail "deploy-guide/kubernetes/Chart.yaml appVersion is not \"$version\""

if [ "$errors" -gt 0 ]; then
  echo "Documentation is not ready for release $version ($errors problem(s))." >&2
  exit 1
fi

echo "Documentation is linked to version $version."
