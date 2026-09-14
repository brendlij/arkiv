# Changelog

All notable changes to Arkiv are documented here. This project follows [Semantic Versioning](https://semver.org/) and the [Keep a Changelog](https://keepachangelog.com/) format.

## [Unreleased]

### Added

- Added an idempotent `arkiv migrate-uploads` command that copies and verifies legacy managed originals before switching storage roots.

### Changed

- Made `/photos` the writable canonical original store and moved new managed uploads to configurable, sharded paths below `/photos/originals`.
- Clarified account upload quotas separately from physical NAS capacity.

### Fixed

- Prevented scans from failing on unreadable directories or indexing protected NAS folders and managed uploads twice.

## [0.1.1] - 2026-09-14

### Added

- Added GitHub CI, release-PR validation, tag-driven multi-architecture GHCR publishing, and automated GitHub Releases.
- Added automated semantic-version and changelog preparation for release pull requests.

### Changed

- Changed the supplied Compose deployment to pull `ghcr.io/brendlij/arkiv` instead of building locally on the NAS.
- Standardized native, container, health-check, and development API ports on `8090`.
- Updated the application, README, and favicon to use the current Arkiv brand assets.
- Removed the prefilled administrator username from the login form.

## [0.1.0] - 2026-09-14

### Highlights

- Added durable resumable uploads, bounded streaming SHA-256 verification, per-user duplicate checks with keep-copy choice, quota reservations and configurable per-user limits, and startup/hourly partial-transfer cleanup with complete-orphan quarantine.

- Added persistent empty upload folders, atomic recursive rename/move with explicit grants preserved, folder sharing/revocation, and destination-aware uploads. Admin folder assignments now support virtual uploaded folders.

- Clarified My library, Shared with me and Server libraries navigation; added ownership and effective-access panels for folders and individual media, with audience identities restricted to owners/admins.

- Added Ctrl/Strg/Command and Shift media selection, keyboard select-all/Escape/Delete, bulk upload folder moves, and recoverable Trash with owner-only management and shared/public visibility filtering.

- Expanded uploads/scanning to a shared 50-extension photo, RAW and video catalog; added server-provided picker formats, container-signature checks, 250 MiB photo/RAW and 2 GiB video limits, and longer upload timeouts.

- Added stepped photo zoom, mouse-wheel zoom, drag panning and Fit reset; distinguished album adding from zoom controls.

- Redesigned Maps with default street detail, a remembered offline option, a compact place list, photo markers and a selection strip.

- Added authenticated JPEG/PNG/GIF upload with per-user folders, file/quota limits, signature validation, progress, cancellation and preview processing. Upload originals persist beside the database.

- Rebranded the frontend to arkiv using the supplied SVG marks, local Geist fonts, warm light/dark palettes, and an updated favicon.
- Added photo masonry preserving aspect ratios, subtle interaction feedback, reduced-motion overrides and viewer control inactivity handling.

- Added Maps with permission-filtered photo groups, area browsing, place-name search and nearby-place metadata.
- Added offline GeoNames lookup, Natural Earth outlines, optional OpenStreetMap tiles, and migration 006 with GPS backfill and place-name FTS search.

- Added selected-user/public-link sharing with scheduled starts, expiration presets, no-expiry, link rotation/revocation and optional original downloads.
- Added a bounded anonymous album page and migration 005; public tokens are hashed and every public media request checks link validity and source permissions.

- Added database-backed accounts, mandatory temporary-password onboarding, self-service password changes and administrator reset/disable workflows.
- Added recursive personal folder grants, private favorites, owned albums, viewer/contributor sharing and server-side media authorization with immediate revocation.
- Added account, people/access and album-sharing interfaces, plus a Gallery G/photo logo and favicon.
- Added migration 004 with preservation of existing albums/favorites and cross-user security regression tests.

### Added

- Single-process Go server with embedded Vue/TypeScript interface, SQLite WAL database, numbered migrations and graceful shutdown.
- Batched incremental library scans, per-file metadata, durable bounded preview jobs, retry/recovery, read-only source policy and path containment.
- ExifTool metadata and RAW extraction, embedded TIFF thumbnail fallback, libvips WebP variants, ffprobe video metadata and ffmpeg posters.
- Chronological gallery, immersive viewer, light/dark themes, search, favorites, virtual albums, folders and Inbox.
- Argon2id authentication, scoped persistent sessions, CSRF protections, login throttling and explicitly trusted proxy mode.
- Multi-stage Dockerfile, Compose configuration, NAS deployment/backup instructions and reproducible verification tests.

### Fixed

- Stale processing workers cannot overwrite a newer file generation.
- Inaccessible or cancelled scans do not reconcile removals.
- DNG files with only embedded TIFF thumbnails now generate previews.
- Source orientation is preserved when extracting RAW previews.
- Media pagination remains bounded, and scanner writes use batches of 128.
- Small video posters preserve their original dimensions rather than upscaling before thumbnail generation.

### Processing visibility

- Added an administrator Processing page with live scan, readiness and queue details.
- Persisted worker phases and added single/all failed-job retries that leave healthy and running jobs untouched.
- Kept full preview rebuilding in a separate maintenance control.

- Added confirmed permanent deletion for owned uploads in Trash and personal Trash/removal for accessible shared and read-only server media. Deletion cleanup is durable across restarts.

- Viewer: Space toggles video playback outside native controls; videos support zoom/pan and independent zoomed playback controls. Photo zoom progressively loads browser-supported originals instead of enlarging only a 2048-pixel preview.

- Signed-in image viewer opens originals immediately. Added full-size lossless PNG rendering for browser-incompatible images and 16-bit RAW sensor development with supported libvips builds. Low-resolution fallback is no longer silently displayed as the opened image.

### Cache maintenance and compatible videos

- Added hourly and manual cache auditing, missing-preview repairs, conservative obsolete-file cleanup and idle expiry for full-size image/video copies.
- Added an admin storage summary and on-demand persisted H.264/AAC playback conversion with access checks, original preservation, retries and restart recovery (migration 012).

### Mobile browsing

- Compact navigation with More menu, stable three-column gallery and append-based loading.
- Safe-area-aware viewer controls, scroll restoration, photo touch gestures and hold-to-select.

### Configuration and setup naming

- Standardized runtime settings exclusively on `ARKIV_*` with shared healthcheck port resolution.
- Renamed Docker service/image/entrypoint to arkiv; retained gallery executable alias and existing database path. Host mounts and published port are configurable in .env.
- Rewrote README setup, storage, permissions, backup and upgrade guidance; added configuration reference.
