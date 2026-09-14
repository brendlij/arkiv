# Verification record

## 2026-09-13 — Windows development host

This records actual checks, not a declaration of completed NAS acceptance.

- Go 1.26.8 native build, automated tests, and `go vet ./...` passed.
- Linux amd64 and arm64 binaries cross-compiled successfully; `docker compose config --quiet` accepted the deployment configuration.
- Vue 3 / TypeScript production build passed. Built JavaScript is approximately 95 kB (36 kB gzip); no CDN or remote frontend service is required.
- Actual HTTP startup, embedded frontend response, protected API, SQLite migrations and context-driven graceful shutdown passed an automated application test.
- SQLite lifecycle tests verify WAL, foreign keys, migrations, favorites and album membership across reopen; scanner tests verify new, unchanged, modified, removed and unavailable/cancelled roots.
- API tests verify cursor pagination, FTS search, folders, Inbox, favorites, albums, invalid IDs, missing originals, traversal rejection, and HTTP range responses.
- Actual ExifTool 13.59, libvips 8.18.6, and ffmpeg processed a generated EXIF JPEG, MP4 and MOV; verified all three WebP sizes, orientation, no upscaling, corrupt-file isolation, job recovery, and unchanged original checksums.
- Actual public-domain RAW fixtures passed embedded extraction plus all three WebP sizes, with original checksums unchanged: Sony A7 III ARW, Canon EOS 40D CR2, Canon EOS R CR3, Nikon D7000 NEF, Fujifilm X-T1 RAF, and a PowerShot SX40 HS DNG with an embedded TIFF thumbnail.
- The final regression run with both real-media flags enabled passed on 2026-09-13. Its local log is `test-results/final-tests.log`; the 100k-row and 10k-file scale tests were run separately as recorded below.
- A subsequent focused real-media regression passed after correcting video-poster upscaling; both image previews and small video posters now have dimension assertions.
- The running application processed these six RAW formats successfully. Native browser checks exercised sign-in, viewer, metadata, favorites, album creation/membership, hierarchical folders, Inbox, and video decoding. Desktop and 390×844 mobile layouts were visually inspected; light theme remained selected after reload. MP4 reported readyState 4, duration 2 seconds and video width 640 in the browser.

## Performance observations

On this development PC, with 100,000 synthetic indexed rows:

| Request | Observed time |
| --- | ---: |
| Timeline, 120 assets | 1.5 ms |
| Broad FTS search, 120 assets | 219 ms |
| Folder grouping | 89 ms |
| Statistics | 57 ms |

With 10,000 small fixture files, discovery/indexing took 3.46 seconds and an unchanged rescan took 0.45 seconds. These scanner checks do not include media decoding. Files are committed in batches of 128, and directory traversal retains entries for the current directory rather than the whole tree.

These are individual test observations, not NAS performance guarantees or a load-test distribution. Actual images, codecs, NAS disks and concurrent users change performance.

## Reproduce

```sh
cd web && npm ci && npm run build && cd ..
go test ./...
go vet ./...
ARKIV_INTEGRATION=1 go test ./internal/jobs -run TestRealMediaPipeline -v
ARKIV_RAW_FIXTURES=/absolute/path/to/raw-fixtures go test ./internal/media -v
ARKIV_SCALE_TEST=1 go test ./internal/api ./internal/scanner -v
docker build --target verify .
```

Real-media tests require `exiftool`, `vipsthumbnail`, `vipsheader`, `ffmpeg`, and `ffprobe` on PATH. RAW fixtures must include uppercase `.ARW`, `.DNG`, `.CR2`, `.CR3`, `.NEF`, and `.RAF` files. Without the environment flags those expensive/external tests explicitly skip. They are not counted as passing format acceptance merely by the default test run.

## Fixture provenance

RAW fixtures are CC0 samples from [raw.pixls.us](https://raw.pixls.us/), downloaded into ignored `test-results/`, not distributed or seeded into the application. Public sources and SHA-256 digests:

| Format | Source ID | SHA-256 |
| --- | --- | --- |
| ARW | [2414](https://raw.pixls.us/getfile.php/2414/nice/Sony%20-%20ILCE-7M3%20-%2014bit%2014bit%20compressed%20(3:2).ARW) | `250784580ea527442c09004417bb0eead484f2bf3ee8f9121a776ac65bb50d0f` |
| CR2 | 2101 | `775c806358fedddec7622113a5e399a3330bd5a65e35e37ae1108d8bf58067be` |
| CR3 | 4515 | `89bd55532b0cceb5efa360e97827ba35c2076bce0de4675f3a4413a883dac880` |
| NEF | 958 | `639945753240005d80488c0462c01e73a7ab82c96d3d1cdb6661f02e1b3c1319` |
| RAF | 2421 | `e994a1fd6e87e392432fe146a35b0b88584dc2bd50bee2c8c7e886ac2b59fcde` |
| DNG | 748 | `dab360e60b94a3f011dd154577f3e7305febd3d497d47379cc4a64b383c48e1f` |

## Open acceptance gates

- Docker image build and Compose runtime could not be executed: local Docker Desktop failed while creating its `dockerInference` socket and exited. This is not a successful container test. The runtime image uses Debian-packaged tools, so the native tool tests do not prove identical container codec coverage.
- UGREEN NAS deployment, permissions, architecture, shutdown, and performance need verification on the actual device.
- One fixture per priority RAW format does not prove every camera variant. HEIC/HEIF and the other advertised detection formats still need representative real-file acceptance; animated GIF uses a still preview.
- Fullscreen is implemented but its state could not be confirmed in the in-app browser. Native browser/device fullscreen, touch swipes and broader screen-reader testing remain open.
- The explicit symlink-escape test skipped because Windows did not grant symlink creation privileges; traversal and OS-rooted file reads passed. Run the same test suite in the Linux verification target before release.
- Cache pruning for removed/changed originals and full RAW development are not implemented. See README for recovery and cache policy.
## Multi-user and identity verification — 2026-09-13

Implemented migration 004, persisted accounts, recursive folder grants, private favorites, owned/shared albums, viewer/contributor permissions and admin/account/sharing UI. See `docs/ACCOUNTS.md` for the complete workflow and `docs/BRAND.md` for the logo prompt and generation details.

Verified on the native Windows build:

- `go test ./...` and `go vet ./...` pass.
- `npm run build` (Vue type checking + Vite) and `npm run format:check` pass.
- Cross-user regression tests cover folder descendants and sibling isolation, traversal rejection, search/folder/statistics filtering, inaccessible asset IDs, original and ready-thumbnail authorization, private favorites, private album discovery, sharing and role updates, no resharing borrowed assets, contributor removal ownership, membership/folder revocation, admin-only operations, and original preservation after album deletion.
- Account tests cover temporary-password API gating, password change, old session invalidation, reset/disable, denied member administration, self-demotion protection, preserved database credentials after restart, and bootstrap adoption of legacy albums/favorites.
- 100,000-row admin query check: timeline 0.59 ms, FTS search 223 ms, folders 80 ms, statistics 67 ms on this local machine. These are fixture timings, not NAS capacity guarantees or multi-user load benchmarks.
- Browser checks: native test admin sign-in; People & access; shared-album discovery and management panel; test member sign-in with only 7 assigned-folder assets rather than all 8; personal folder display; private album listing; account form at 390×844; fixed mobile navigation; logo and desktop layouts. Password mutations were exercised through backend/API tests, not browser password entry.
- Local preview retains only test data: existing admin fixture and `maya-demo` with an assigned Travel folder and a shared Weekend stories album. These are not deployment credentials or seeded production users.

Docker/NAS execution remains unverified because the previously recorded Docker Desktop engine startup failure persists. No claim of container acceptance is made. Public links and uploads are outside this implementation; shared albums require provisioned signed-in users and originals remain read-only.
## Public links and access windows — 2026-09-13

This extends and supersedes the earlier signed-in-only sharing limitation. Migration 005 adds scheduled start/expiry to user invitations and a hashed public-link table. Public sharing remains off until an album owner/admin creates a link.

Verified:

- `go test ./...`, `go vet ./...`, frontend production build, and Prettier check pass.
- Public API tests exercise anonymous access through the real authentication middleware; private API routes remain authenticated. Token hashes are stored instead of raw URLs, and settings do not reveal either secret representation.
- Public listing and media are scoped to the album even when the visitor has a broader signed-in session. Unrelated asset IDs fail. Previews work while originals are blocked by default; explicit original-download permission works.
- Rotation, revocation, expiry boundaries, future start times, invalid timestamps (including epoch zero), disabled owners, and revoked source-folder grants deny access. Timed user invitations deny listing, originals, previews and contribution outside their window.
- Browser: selected-user/public mode controls, date inputs, duration presets, link creation, no-login public page, RAW preview, and immediate unavailable state after revocation. Responsive sharing form and public viewer checked at 390×844; the native dialog receives focus and makes background controls inert. Public verification links were revoked afterward.
- Original source paths and detailed camera/location metadata are omitted from anonymous listing responses. Optional original downloads can contain embedded metadata; this is explained in the sharing UI and documentation.

The final local preview is rebuilt and running. No external hosting/network exposure was created; Docker/NAS runtime acceptance remains unchanged and unverified. Existing downloads and in-progress transfers cannot be recalled by expiry or revocation.

## Maps and places — 2026-09-13

Migration 006 adds locally derived nearby-place labels and place-name FTS search. The signed-in map and place APIs enforce the existing asset permissions. Anonymous public albums do not gain map or detailed location endpoints.

Verified:

- `go test ./...`, `go vet ./...`, `npm run build`, and `npm run format:check` pass.
- Tests cover local place lookup, missing/invalid/remote GPS, backfill and FTS, private-folder isolation, shared albums and expired access, disabled libraries, place filters, bounded queries and date-line crossing.
- The 100,000-row GPS fixture returned the world map in about 754 ms and a restricted viewport in 81 ms on this host. These are isolated development measurements, not NAS or concurrent-user capacity guarantees.
- Browser checks cover offline country outlines, place search and photo selection, the optional OpenStreetMap street layer, nearby-place metadata in the viewer, and Maps at 390×844. A global SVG sizing conflict and street-layer stacking were corrected during visual verification.
- The original eight preview files had no GPS. Three new synthetic test-pattern JPEGs, named `map-demo-Berlin.jpg`, `map-demo-Paris.jpg`, and `map-demo-Sydney.jpg`, were added under the test library's `Travel/Maps` folder with known fixture coordinates. Existing original files were not modified. The preview now reports three GPS photos and eight without GPS.

Place names are approximate nearby populated places, not exact administrative boundaries or street addresses. Default map and lookup data are bundled locally. Optional street tiles contact OpenStreetMap only after the checkbox is enabled; it is left off in the final preview. Sources, licenses and rebuild instructions are in [MAPS.md](MAPS.md). Docker/NAS runtime acceptance remains unverified as recorded above.

## arkiv visual identity — 2026-09-13

- Frontend production build (including Vue/TypeScript checking), Go binary build and Prettier check pass.
- Verified the supplied full SVG wordmark at sign-in, icon-only navigation, arkiv browser title, light and dark palettes, and locally bundled Geist font assets.
- Verified photo masonry at desktop size and 390×844, with original aspect ratios and reserved layout space.
- Verified the viewer's inactive state visually and via its rendered class, then immediate Right-arrow navigation and control restoration. Keyboard use keeps controls available. Existing swipe, zoom and fullscreen handlers remain in place; no device touch-gesture or frame-rate benchmark was performed in this pass.
- Reduced-motion CSS disables decorative animations and transforms. No automated OS-preference emulation was performed.
- Existing eight preview originals and three synthetic GPS fixtures remain unchanged. The native preview runs the rebuilt frontend. Docker/NAS acceptance remains open.

## Photo uploads — 2026-09-13

All Go tests and vet passed; frontend production/type checks passed. Upload regressions cover authenticated ownership, other-member isolation, repeated filenames without overwriting, quota exhaustion, filename paths, invalid bytes and mismatched formats. The browser successfully uploaded the synthetic map-demo-Berlin.jpg fixture and showed its ready thumbnail in My uploads, with no queued/failed jobs. An additional user-uploaded photo was present and left untouched. Managed upload assets survive outside the external-library scanner. A final rebuild adds a friendly My uploads heading. No actual mobile picker, large-file throughput or Docker runtime test was performed. Upload originals live beside the database and require backup; see README for format limits and crash-orphan limitations.

### Upload interaction correction — 2026-09-13

Successful batches now close and reset the upload dialog, navigate to My uploads and show an inline confirmation. Failed/cancelled batches retain their file statuses. Upload moved from the search toolbar to the page heading. Visible pending photos refresh their metadata/previews every two seconds without reloading the grid or resetting viewer order. Polling stops on unmount and ignores responses across account changes. Production/type build passed; browser upload confirmed automatic dismissal, destination navigation and immediate pending-photo display.
The pending test photo changed to its ready image automatically with no refresh action. Mobile placement was visually checked at 390×844; viewport override was reset afterward.

### Detailed Maps redesign — 2026-09-13

Replaced the default outline with standard OpenStreetMap street tiles, retained a browser-persisted Offline map choice, and kept local place lookup. The page now uses a unified place-list/map layout, thumbnail markers, an active place state, area search over the map and a horizontal selection strip. On mobile the map precedes the place list. Provider attribution and a concise data-sharing explanation remain visible. Desktop browser checks verified Berlin street labels, selection and opening a photo from the strip; 390×844 layout and offline switching were checked. Frontend production/type build, formatting check and Go binary build passed. Existing map authorization APIs were unchanged. This supersedes earlier notes saying the default is offline. Docker/NAS acceptance remains unchanged.

### Maps integration and sidebar mark — 2026-09-13

Place search now applies one focus outline to the entire icon/input wrapper and suppresses the nested input outline. Removed the outer Maps card border/background; the place list shares the page surface. The sidebar uses a simplified compact SVG with an adjacent lowercase arkiv wordmark. The supplied full sign-in mark remains available. Production/type and Go builds passed; final browser screenshot verified the focused search, integrated map layout and readable wordmark.

### Photo zoom controls — 2026-09-13

Replaced the native-size toggle with bounded 1×–8× zoom relative to fitted size, separate minus/Fit/plus controls, pointer-anchored wheel zoom, double-click 2×/fit, and captured drag panning. +/− and 0 keyboard controls are available. Photo changes, details-layout changes, fullscreen and window resize reset zoom. Album adding uses a distinct album-plus icon and tooltip. Video and unavailable-preview zoom controls are disabled.

Production/type and Go builds passed. Browser checks verified plus to 150%, wheel zoom without modifier keys to about 215%, pan translation, and 0 restoring the fitted image. The toolbar was checked at 390×844 and the viewport reset. Touch pinch is not implemented; existing fitted-photo swipes remain.

### Consistent collection controls — 2026-09-13

Folder cards and navigation controls share a 12px corner token. Sidebar and folder keyboard-focus outlines are inset, preventing clipping inside scrollable navigation; active navigation keeps its selected fill on hover. Folder hover/press feedback follows the same palette. Production/type and Go builds passed. Browser keyboard navigation reproduced the focused Folders state and verified its complete outline alongside rounded Photos/Uploads cards.

## Expanded media uploads — 2026-09-13

Upload and scanner recognition share a 50-extension media catalog. The authenticated capabilities endpoint drives the picker and client validation. Content signatures distinguish common image, RAW and video container families; JPEG/PNG/GIF retain header-dimension checks. Full codec/preview validation remains in processing jobs. Limits are 250 MiB photo/RAW, 2 GiB video and 5 GiB per user, with a one-hour upload read deadline and client timeout. Default API limits are unchanged.

Verified: all Go tests and vet; production frontend/type build. Tests cover signature matches/mismatches, empty files, mixed video/RAW classifications, account isolation, quota and filename protections. Optional validation against the 11 existing local fixtures passed, including MP4, JPEG, CR2, CR3, DNG, NEF, RAF and ARW. Native authenticated multipart HTTP uploads of test-video.mp4 and sample.CR3 (31,606,200 bytes) succeeded and finished preview generation. Browser verified the format list, video poster, MP4 player readyState 4 with two-second duration and 640px width, and the RAW entry in My uploads. The processing queue returned to zero with no failed jobs. The two uploaded fixtures are additional test copies; existing sources were untouched.

No claim is made that every listed camera/codec combination was exercised. HEIC/JXL/AVIF and legacy video decoder availability depends on the installed tools; 2 GiB throughput, hour-long transfers and Docker/NAS execution were not tested. README documents these boundaries and backups.


## Multi-selection and managed media organization (2026-09-13)

- Go tests and vet passed; Vue typecheck and production build passed.
- Added atomic bulk-action tests: other-owner and mounted-library selections rejected without partial updates; invalid destination rejected; virtual move preserves storage path; trash hides from admin/member/public visibility; restore retains the destination and re-enables album visibility.
- Live browser: Control-click plus Shift-click selected four consecutive items, move dialog closed on success, new folder appeared in My uploads, Trash immediately showed the test item, restore emptied Trash without refresh. Test photo returned to its original folder; personal uploads untouched.
- Visual check: checked corner controls, selected image treatment, and responsive wrapping toolbar. Mounted libraries remain read-only; no permanent deletion.


## Ownership and access clarity (2026-09-13)

- Go tests and vet, Vue typecheck and production build passed. Access-details tests cover owner audience, viewer identity redaction, inaccessible/anonymous denial, active public links, and owner-only Trash visibility.
- Browser verified My library ownership labels, folder access expansion, navigation highlight preservation, and ownership inside video Details. No grants or sharing settings were changed.


## Managed folders (2026-09-13)

- Full Go suite and vet passed; production Vue typecheck/build passed.
- Integration tests verify empty folder persistence, parent creation, recursive rename/move of empty descendants and trashed media, unchanged original paths, grant migration, collision/self-cycle rejection, cross-user isolation, share/revoke, empty-folder access and admin virtual-folder assignments. Upload tests verify nested destinations and immutable storage/quota behavior.
- Browser verified empty-folder creation, rename, moving into a new parent, the upload destination preselection, and sharing an empty test folder with the demo account. No personal files were moved or shared. Browser removal of this temporary share was blocked by automatic approval review pending explicit authorization; revocation is covered by API tests.


## Reliable uploads (2026-09-13)

- Go suite and vet passed. Regression tests simulate unacknowledged bytes followed by DB restart, wrong offsets, idempotent initiation/completion, crash after final rename, full-file corruption, per-user duplicate isolation, duplicates in Trash, quota reservations and overrides, expiry, cleanup and quarantine preservation. Auth integration verifies the quota route and 8 MiB chunk limits without widening ordinary request limits.
- Incremental browser SHA-256 matched independent Node crypto digests across padding/chunk boundaries and beyond 512 MiB using bounded buffers. Vue production build and typecheck passed.
- Live browser: generated PNG uploaded through the new protocol, dialog closed, gallery updated immediately, and preview became ready. A 32-byte partial transfer created through the authenticated API appeared in Paused uploads and was discarded in the UI; the already-uploaded original remained. Admin per-user storage controls rendered correctly. Browser file selection was unusually slow in the automation environment; no second file-picker test was attempted.
- Docker/NAS performance and deployment acceptance remain pending. The legacy multipart compatibility endpoint remains non-resumable.

## Processing dashboard — 2026-09-14

- `go test ./...` and `go vet ./...` passed. New regression coverage verifies admin-only reads/retries, separate readiness/queue counts, live scan examined count, 50-item pagination, input validation, disabled-library exclusion, and targeted/all failed retries without touching healthy or running jobs. Worker tests verify metadata and preview phases during tool execution. Migration persistence test updated for migration 010.
- Vue typecheck and production build passed; Go executable rebuilt and local fixture server restarted successfully with migration 010.
- Browser verification: Processing opens from the sidebar, reports 19 indexed originals / 19 metadata-ready / 19 previews-ready, no failed jobs, and a disabled retry action when no failures exist. Queue filtering displays the correct empty state. Manual Scan library updates the last successful scan timestamp without changing asset counts. Desktop layout inspected visually.
- NAS-scale processing, disk-cache integrity auditing, cache pruning, and real mobile-device acceptance remain unverified/out of scope.

## Trash and personal removal — 2026-09-14

- `go test ./...`, `go vet ./...`, Vue typecheck and production build passed with migration 011.
- New regression test covers borrowed-photo personal Trash/restore/permanent hiding, unchanged external rescans, preservation of other owners' originals, denial after access revocation, confirmation enforcement, atomic invalid-selection rejection, and rejection of permanent deletion outside Trash.
- Owned-original deletion verified using isolated temporary fixtures: removes current cached derivatives, upload receipts, favorites and album links. An unavailable upload root preserves durable deletion intent and hides pending deletion from media/file routes; maintenance completes deletion after root recovery and is idempotent.
- Browser preview verified: Trash displays the new permanent-delete button, confirmation names the selected file and explains original deletion, and Cancel closes it without changing the existing media. No existing user originals were deleted during UI verification.
- Managed originals are physically deleted only for their uploader. Shared/external originals are personally hidden, not removed from external storage. Current preview derivatives are cleaned; obsolete prior-generation cache pruning remains separate. NAS filesystem-failure and Windows locked-file acceptance still require deployment testing.

## Viewer playback and zoom — 2026-09-14

- Vue typecheck and production build passed; embedded Go preview rebuilt.
- Browser verified Space plays and pauses a video while focus remains on Close viewer (outside the video). Video Zoom in produces scale(1.5), disables native controls while zoomed, and exposes separate play/pause, seeking and Fit controls.
- Browser verified zooming DSC05240.jpg replaces the 2048 derivative with the authenticated original, natural dimensions 4671 x 4672, while retaining the 150% zoom transform.
- Full-resolution upgrade applies to browser-decodable JPEG, PNG, WebP, AVIF, GIF and BMP originals. RAW and other formats retain generated previews. Original decode failure falls back to the preview; stale image loads are ignored on navigation. Real device/native video fullscreen behavior remains unverified.

## Original-first image quality — 2026-09-14

- Go suite/vet and frontend typecheck/build passed. Cached full-image access test covers authorized access, missing grants, revocation and Trash exclusion.
- Browser: JPEG opened at scale 1 directly from `/original`, natural dimensions 4671 x 4672, without waiting for zoom.
- Local actual libvips 8.18 RAW sensor development yielded a 6024 x 4024, 3-band, 16-bit PNG from sony-a7iii.ARW; no resizing or embedded JPEG extraction.
- Full-size rendered PNGs use serialized on-demand conversion and permission/generation recheck. Original uploads are unchanged. Current full-size cached derivatives join permanent-deletion cleanup.
- Broader HEIC/TIFF/RAW camera variants, Docker loaders, HDR/wide-gamut visual parity and full-memory stress testing remain unverified. No promise of pixel-identical RAW rendering across different developers or monitors.

## Cache maintenance and video conversions - 2026-09-14
- Go tests/vet and Vue typecheck/production build passed. Isolated cache tests cover orphan removal, protection of current/fresh/unknown files, idle copy expiry, idempotent repair and admin-only maintenance.
- Actual FFmpeg fixture conversion produced H.264 at the input dimensions; SHA-256 of the original remained identical. Tests verify HTTP byte ranges, denial without access and rejection of stale source generations.
- Browser storage panel and explicit compatible-playback queue verified. HDR conversion is unsupported; public shares, large-file/NAS resource stress and a broad codec matrix remain outside this verification.

## Mobile navigation, gallery and viewer - 2026-09-14
- Added five-slot bottom navigation with an accessible full-menu dialog, safe-area spacing, 16px form inputs, three-column stable thumbnails, touch-sized controls and lazy decoding.
- Cursor loading appends without clearing the grid; mobile intersection loading stops automatically at 600 loaded assets, with explicit Load more available thereafter. The loading test exercises deduplication, duplicate-request prevention, stale navigation responses, errors and retry.
- Viewer restores the gallery scroll position, supports touch pinch/pan and double-tap zoom, and distinguishes horizontal photo swipes from vertical gestures. Native video controls remain available. Long press activates selection; moving the finger cancels the hold.
- Browser checked at 390x844: navigation menu, photo grid, playback state and return to a scrolled gallery. Real iPhone Safari multi-touch, native fullscreen, large-library memory and device performance still require physical-device testing.

## Mobile hold-and-drag selection - 2026-09-14
- Per-photo checkbox buttons are hidden on touch/mobile layouts; the main photo button exposes selected state for accessibility. A 400 ms hold starts additive range selection, drag extends/retracts that range, and dragging near viewport edges scrolls the gallery. Movement before activation retains normal scrolling.
- `npm run test:touch` runs the actual compiled GalleryGrid setup with controlled hit-testing: verifies swipe cancellation, hold activation, range extension/retraction, preservation of earlier selections, edge scrolling, multi-touch cancellation, click suppression and short-tap opening. Typecheck and production build passed. Real iPhone Safari touch dispatch remains a physical-device acceptance check.

## Integrated mobile selection - 2026-09-14
- Replaced the expanding in-flow action panel on mobile with a constant-height selection strip and fixed bottom actions occupying the navigation area. Exiting selection is also available in the bottom dock while scrolled.
- Browser at 390px: selecting an image preserves scrollY=0 and its top position; the long permissions explanation no longer expands the gallery. Touch interaction regression test and frontend build passed.

## Immersive mobile viewer - 2026-09-14
- Full-viewport media with overlay controls, horizontal touch navigation for photos and videos, downward swipe to close, delayed single-tap control toggle and double-tap/pinch zoom. Zoomed movement pans rather than navigating. Mobile video playback has a separate seek bar.
- `npm run test:viewer` checks the compiled component gesture logic for sideways photo/video navigation, close gesture, zoom protection, upward-swipe rejection and cancellation. Typecheck/build passed; 390px browser screenshot and video playback verified. Actual iPhone Safari gesture delivery and browser chrome/fullscreen behavior remain device acceptance checks.

## Configuration naming and Docker docs - 2026-09-14
- Go tests/vet passed, including legacy settings, canonical precedence, explicit-empty hash handling and shared healthcheck port selection.
- Docker Compose config validated using a non-secret placeholder: canonical hash precedence, legacy hash fallback, host-port override and resolved bind mounts.
- Docker CLI is installed but the Docker Desktop Linux engine is unavailable; image build/runtime validation remains unverified. No container deployment was performed.
