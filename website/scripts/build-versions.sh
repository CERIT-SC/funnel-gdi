#!/usr/bin/env bash
# Builds the versioned documentation site published on GitHub Pages:
#
#   <out>/<version>/   every release tag X.Y.Z.N that contains the website tooling
#   <out>/latest/      the newest of those releases
#   <out>/dev/         the development version (DEV_REF, default HEAD = master-gdi)
#   <out>/versions.json  the list the version switcher reads
#   <out>/index.html     redirects to latest/ (or dev/ before the first release)
#
# Older releases without the website tooling are listed in versions.json with
# a link to their documentation on GitHub.
#
# Usage: website/scripts/build-versions.sh [out-dir]   (default: build/pages)
# Env:   SITE_ROOT (default https://cerit-sc.github.io/funnel-gdi/)
#        DEV_REF   (default HEAD; WORKTREE = the working directory as it is)
#        PAGEFIND  (default 1; 0 skips the search index, which needs npx)
#        DEV_ONLY  (default 0; 1 builds only dev/, e.g. to check a pull request)

set -euo pipefail

root="$(git -C "$(dirname "$0")" rev-parse --show-toplevel)"
out="$(mkdir -p "${1:-$root/build/pages}" && cd "${1:-$root/build/pages}" && pwd)"
site_root="${SITE_ROOT:-https://cerit-sc.github.io/funnel-gdi/}"
dev_ref="${DEV_REF:-HEAD}"
repo_url="https://github.com/CERIT-SC/funnel-gdi"
tooling="website/scripts/sync-gdi-docs.py"
work="$(mktemp -d)"
trap 'git -C "$root" worktree prune; rm -rf "$work"' EXIT

rm -rf "${out:?}"/*

# build <git-ref> <directory> <version label> <ref for GitHub links>
# The ref WORKTREE builds the working directory as it is (local preview).
build() {
  local ref="$1" dir="$2" version="$3" link_ref="$4" tree="$work/$2"
  echo "==> Building $dir ($version) from $ref"
  if [ "$ref" = WORKTREE ]; then
    tree="$root"
  else
    git -C "$root" worktree add --quiet --detach "$tree" "$ref"
  fi
  python3 "$tree/$tooling" --version "$version" --ref "$link_ref"
  HUGO_PARAMS_SITEROOT="$site_root" hugo --source "$tree/website" --minify --quiet \
    --baseURL "$site_root$dir/" --destination "$out/$dir"
  if [ "${PAGEFIND:-1}" = 1 ]; then
    npx -y pagefind@1.5.2 --site "$out/$dir" --silent
  fi
  if [ "$ref" != WORKTREE ]; then
    git -C "$root" worktree remove --force "$tree"
  fi
}

releases=()   # releases with the website tooling, newest first
external=()   # older releases, documented on GitHub only
while read -r tag; do
  [ -n "$tag" ] || continue
  if git -C "$root" cat-file -e "$tag:$tooling" 2>/dev/null; then
    releases+=("$tag")
  else
    external+=("$tag")
  fi
done < <(git -C "$root" tag -l | grep -E '^[0-9]+\.[0-9]+\.[0-9]+\.[0-9]+$' | sort -V -r)
if [ "${DEV_ONLY:-0}" = 1 ]; then
  releases=()
fi

build "$dev_ref" dev dev master-gdi
for tag in ${releases[@]+"${releases[@]}"}; do
  build "$tag" "$tag" "$tag" "$tag"
done
latest="${releases[0]:-}"
if [ -n "$latest" ]; then
  build "$latest" latest "$latest" "$latest"
fi

# versions.json for the version switcher
{
  echo "["
  sep=""
  for tag in ${releases[@]+"${releases[@]}"}; do
    flag=""
    [ "$tag" = "$latest" ] && flag=', "latest": true'
    printf '%s  {"name": "%s", "path": "%s/"%s}' "$sep" "$tag" "$tag" "$flag"
    sep=$',\n'
  done
  printf '%s  {"name": "dev", "path": "dev/"}' "$sep"
  for tag in ${external[@]+"${external[@]}"}; do
    printf ',\n  {"name": "%s", "url": "%s/tree/%s"}' "$tag" "$repo_url" "$tag"
  done
  printf '\n]\n'
} > "$out/versions.json"

# The site root opens the latest release, or dev before the first release.
target="${latest:-dev}"
[ -n "$latest" ] && target="latest"
cat > "$out/index.html" <<EOF
<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <title>Funnel GDI documentation</title>
  <meta http-equiv="refresh" content="0; url=$site_root$target/">
  <link rel="canonical" href="$site_root$target/">
</head>
<body>
  <p>Redirecting to the <a href="$site_root$target/">funnel-gdi documentation</a>.</p>
</body>
</html>
EOF
cp "$out/index.html" "$out/404.html"
touch "$out/.nojekyll"

echo "Built: dev ${releases[*]:-} ${latest:+latest}; linked to GitHub: ${external[*]:-none}"
