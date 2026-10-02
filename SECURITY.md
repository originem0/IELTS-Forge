# Security Policy

## Supported version

Only the latest release is supported with security fixes.

## Reporting a vulnerability

Please do not publish exploitable details in a public issue. Use GitHub's private vulnerability reporting feature for this repository when available, or contact the repository owner privately.

Never attach real API keys, private essays, recordings, purchased question-bank content, or complete local backup files to an issue.

## Local security model

The Windows portable launcher binds atomically to an available `127.0.0.1` port. Local APIs reject foreign hosts and cross-site browser requests. Saved text and optional image API credentials are encrypted independently with current-user Windows DPAPI in the private `runtime-data` directory and restored on startup. Deleting one connection preserves the other. Credentials are excluded from learning backups, question-bank archives and distribution packages.

Data-directory locks and revision checks protect concurrent writes. Question-bank imports validate paths, content hashes, media signatures and references before publication. Uploads and expanded ZIP contents are each limited to 1 GiB, with a 128 MiB per-media limit. Pack deletion refuses references from current records and managed recovery points; hiding an old question preserves its immutable source for history.

