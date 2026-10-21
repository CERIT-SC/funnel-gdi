# Changelog (GDI fork)

All notable changes to the GDI fork of Funnel (`CERIT-SC/funnel-gdi`) are
documented here. `CHANGELOG.md` is the upstream changelog of
[calypr/funnel](https://github.com/calypr/funnel) and is not maintained by
this fork; the current list of GDI additions is in
[GDI-FEATURES.md](GDI-FEATURES.md).

The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).
Versions use the scheme `X.Y.Z.N`: `X.Y.Z` is the upstream release the fork
is based on, `N` is the GDI revision on top of it.

## [0.12.2.1] - Unreleased

### Added

- CI reports the statement coverage of the unit and component tests per package (job summary and the `unit-coverage` artifact with an HTML report) and fails when it drops below the minimum `COVERAGE_MIN`. Run `make test-coverage` for the same report locally.
