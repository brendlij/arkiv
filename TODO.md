# arkiv Development Progress

## Managed folders
- [x] Persistent empty folders with migration/backfill
- [x] Owner-only create/rename/move; recursive asset, folder and grant updates
- [x] Direct upload destination and inherited access explanation
- [x] Existing-user folder sharing and revocation; admin virtual-folder assignment
- [x] Cross-user, collision, cycle, empty folder, Trash, quota and sharing regression checks

## Photo upload — native implementation
- [x] Separate managed storage, per-user access and existing processing queue
- [x] File selection, drop target, per-file progress and cancellation
- [x] Format/filename/quota and cross-user isolation regression tests
- [x] Browser upload through to a ready preview in My uploads
- [x] Shared format catalog, photo/RAW/video uploads and per-kind size limits
- [x] Configurable per-user quotas, resumable chunks, duplicate detection and stale upload cleanup/quarantine

## arkiv visual identity — verified native implementation
- [x] Supplied SVG marks, favicon, local Geist and light/dark brand palette
- [x] Responsive masonry preserving photo aspect ratios
- [x] Short interaction transitions and reduced-motion overrides
- [x] Viewer inactivity handling with keyboard-safe controls
- [x] Production build, formatting and desktop/mobile browser checks

## Maps and places — verified native implementation
- [x] Local place-name index, migration, GPS backfill and processing integration
- [x] Permission-filtered clustered map, place list and location/area search
- [x] Offline world map, optional street tiles, place-photo browsing and metadata labels
- [x] Location/permission tests, frontend verification and data attribution

## Public and timed sharing — verified native implementation
- [x] Migration for scheduled/expiring invitations and hashed public-link tokens
- [x] Anonymous album-only browsing with optional original downloads
- [x] Start/end time controls, copy/replace/revoke link and user access editing
- [x] Anonymous isolation, expiry, revocation, schedule and permission regression tests
- [x] Frontend build, browser verification and documentation

## Multi-user workflows and identity — verified native implementation
- [x] Numbered migration for accounts, per-user favorites, folder grants and album ownership
- [x] Admin user provisioning, disabling, password reset, and self-service password changes
- [x] Enforce folder/album access on every asset, preview, original, search and statistics endpoint
- [x] Personal folders, private albums, shared albums with viewer/contributor roles and revocation
- [x] Account/admin/sharing interfaces with clear permission states
- [x] Cross-user authorization, migration and workflow regression tests
- [x] Gallery logo generation, repository assets, favicon and UI integration
- [x] Update documentation and verify browser workflows

Completion means implemented, compiled, tested, and verified. Unchecked entries may contain partial implementation; deployment and real format acceptance remain separate gates.

## Phase 0 – Foundation
- [x] Repository layout, Go bootstrap, configuration validation, structured logs, graceful shutdown
- [x] Vue 3, TypeScript, Vite build and embedded frontend
- [x] SQLite connection, WAL, foreign keys and numbered migrations
- [x] Health endpoint and persistent directories
- [ ] Multi-stage Docker build and Compose deployment
- [x] README native startup instructions (container execution remains unverified)

## Phase 1 – Media Database
- [x] Media schema, indexes, duplicate path prevention and persistence tests

## Phase 2 – Library Management
- [x] Multiple configured library roots and enabled state; source read-only behavior verified by checksums

## Phase 3 – File Scanner
- [x] Recursive discovery, incremental updates, safe reconciliation and scanner tests

## Phase 4 – Metadata
- [x] EXIF extraction through actual ExifTool integration
  - [x] EXIF JSON fields, deterministic dates and malformed metadata parser tests

## Phase 5 – RAW Support
- [x] Embedded JPEG extraction through isolated RAW interface, with TIFF thumbnail fallback
- [x] Verify real ARW, DNG, CR2, CR3, NEF, RAF fixtures (one public-domain sample per format; not all camera variants)

## Phase 6 – Thumbnail Pipeline
- [x] Bounded persistent jobs, atomic WebP variants and restart recovery

## Phase 7 – Video
- [x] ffprobe metadata, ffmpeg posters and direct range playback
- [x] Verify MP4 and MOV fixtures

## Phase 8 – API
- [x] Paginated timeline, asset detail, previews, originals, folders, search and stats
- [x] Persisted favorites and virtual albums

## Phase 9 – Frontend
- [x] Responsive timeline, search, folders, favorites, albums and Inbox
- [x] Viewer with keyboard navigation, zoom, metadata and video
- [ ] Fullscreen browser acceptance (implemented; in-app browser did not confirm fullscreen state)
- [x] Persisted light/dark theme and dialog focus restoration
- [ ] Reduced-motion OS preference acceptance (stylesheet implemented)
- [x] Desktop and 390×844 mobile browser verification
- [ ] Broader screen-reader and touch-swipe/device acceptance

## Phase 10 – Authentication
- [x] Single-user password hashing, sessions, CSRF protection and login rate limiting
- [x] Optional explicitly trusted proxy authentication; peer/header boundary tests

## Phase 11 – Configuration
- [x] Validated environment variables and documented defaults

## Phase 12 – Docker
- [ ] Non-root runtime with processing tools and read-only source mounts
- [ ] Verify Compose on Docker and document UGREEN deployment

## Phase 13 – Performance
- [x] Bounded workers, paginated queries and frontend rendering (120 assets per page)
- [x] Batch scanner transactions and verify 10,000-file discovery / unchanged rescan
- [x] Measure timeline/search/folder/stats queries with 100,000 indexed rows on development host
- [ ] Measure full scan throughput and memory on a UGREEN NAS

## Phase 14 – Reliability
- [x] Per-asset failures, retry/recovery and unavailable-root safeguards

## Phase 15 – Security
- [x] Path containment, OS-rooted original reads, headers, MIME allowlist and request-body limits
- [ ] Symlink-escape runtime test on Linux (implemented test skips on this Windows host without symlink privileges)

## Phase 16 – Testing
- [x] Automated database, scanner, API, auth and path-security tests
- [x] End-to-end JPEG/MP4/MOV processing, restart persistence and originals integrity

## Phase 17 – Observability
- [x] Structured processing/scan logs and queue/library statistics, including scan errors

## Documentation and release acceptance
- [x] README architecture, configuration, backup, upgrade, troubleshooting and development
- [x] CHANGELOG maintained
- [ ] All 22 requested MVP acceptance criteria verified; do not declare MVP complete before this

## Remaining compatibility / operational work
- [ ] Broader HEIC/HEIF and image/video camera-fixture matrix in the Debian runtime
- [x] Hourly cache audit, obsolete derivative pruning, 30-day idle full-image/video eviction, missing-preview repair and admin storage dashboard
- [ ] Full RAW development fallback for files without any embedded representation (unsupported case documented)

Last verified session: 2026-09-13. See docs/VERIFICATION.md for exact native tests, sample provenance, performance observations, and environment blockers.

- [x] Administrator processing dashboard: live discovery count, metadata/preview readiness, worker phases, paginated queue/errors and targeted failure retries

- [x] Confirmed permanent deletion of selected owned uploads, personal removal of borrowed/external media, and interrupted-deletion cleanup
