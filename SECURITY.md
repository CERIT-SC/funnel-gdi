# Security Policy

## Supported Versions

Which releases get fixes is defined by the [release policy](RELEASING.md#support):
the latest release gets bug and security fixes; the previous release, if it
has a different upstream base, gets security fixes for 3 months after the
latest release is published. See [CHANGELOG-GDI.md](CHANGELOG-GDI.md) for the
list of releases.

| Version     | Supported                                                                 |
| ----------- | ------------------------------------------------------------------------- |
| 0.12.2.x    | Yes, bug and security fixes, once 0.12.2.1 is released                    |
| 0.11.3.x    | Yes until 0.12.2.1 is released; then security fixes only, for 3 months   |
| < 0.11.3.1  | No                                                                        |

## Reporting a Vulnerability

To report a vulnerability in the GDI fork, submit a private Security Advisory:

> https://github.com/CERIT-SC/funnel-gdi/security/advisories/new

If the vulnerability is also present in upstream Funnel, report it upstream as
well:

> https://github.com/calypr/funnel/security/advisories/new
