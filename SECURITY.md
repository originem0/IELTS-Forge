# Security Policy

## Supported version

Only the latest release is supported with security fixes.

## Reporting a vulnerability

Please do not publish exploitable details in a public issue. Use GitHub's private vulnerability reporting feature for this repository when available, or contact the repository owner privately.

Never attach real API keys, private essays, recordings, purchased question-bank content, or complete local backup files to an issue.

## Local security model

The Windows portable launcher binds only to `127.0.0.1`. Saved API credentials are encrypted with current-user Windows DPAPI in the private `runtime-data` directory and restored on startup. Disconnecting and deleting the saved configuration removes them. Credentials are excluded from exported study-data backups and distribution packages.

