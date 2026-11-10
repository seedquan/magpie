# Data export policy in this fork

This fork removes setup synchronization over WebDAV, S3 and GitHub, manual
and scheduled backup/restore, OTLP collectors and session tracing, request
archives (local and S3), session Markdown export and usage CSV export.
Their GUI controls, HTTP routes, CLI commands and background jobs are absent.

Legacy configuration files and environment variables cannot restore the
removed exporters. Existing user data and previously saved backups are
preserved; this change does not erase files or remote objects.

Daily usage telemetry and partner-event collection are permanently disabled
in `internal/stats` and `internal/provider/partner_events.go`. Compatibility
settings remain readable but cannot enable transmission. Fork builds use a
non-release version by default to avoid replacement by upstream auto-updates.

Normal provider/model requests, sign-in, user-selected community plugins,
local usage views and session viewing remain available. Internal recovery
copies used for safe configuration edits are not user backup/export features.
Generated plugin images and client certificates remain normal output files;
they do not export session, usage or setup data.

## Verification

- `TestLegacyArchiveSettingsCannotExport` exercises a gateway request with
  both old archive switches enabled: no bucket calls, archive header or folder.
- `TestRemovedDataExportRoutes` calls the real GUI handler with old settings:
  cloud sync, backup and data export routes return 404 or 405.
- `TestUsageRejectsRemovedExportFlags` rejects the former CLI CSV options.
- The privacy GUI regression runs both browser engines, five languages and
  desktop/phone widths, including old sync and observability deep links.

Repository synchronization and PR destinations follow the personal-fork
boundaries in [AGENTS.md](../../AGENTS.md).
