## API and architecture

`cmd/gallery` owns process lifecycle. `internal/config`, `database`, `library`, `scanner`, `metadata`, `media`, `thumbnails`, `video`, `jobs`, `auth`, and `api` separate configuration, indexing, processing, security and transport. Numbered SQL migrations live beside the database package and are embedded into the executable. Vue components and typed API calls live under `web/src`; gallery state lives in a composable.

| Endpoint | Behavior |
| --- | --- |
| GET /api/health | Public database liveness |
| POST /api/auth/login | JSON `username`, `password`; seven-day session cookie |
| GET /api/auth/session, POST /api/auth/logout | Session status / revocation |
| GET /api/assets, /api/timeline, /api/search | `limit` 1–240, opaque `cursor`, `q`, `favorite=true`, `library`, exact `folder`, `album`, `inbox=true`; returns `items`, `nextCursor` |
| GET /api/assets/{id} | Asset metadata and processing status |
| GET /api/assets/{id}/thumbnail/{size} | Authenticated WebP, size 256/1024/2048 |
| GET /api/assets/{id}/original | Original bytes; `download=1` requests attachment |
| POST /api/assets/{id}/favorite | JSON `favorite` boolean; idempotent assignment |
| GET /api/folders | `library`, `parent`, `offset`; direct children with recursive counts |
| GET /api/albums | Permission-filtered collections, `scope=owned|shared`, `offset`; returns `items`, `hasMore` |
| POST /api/albums | JSON `name` |
| GET /api/albums/{id} | Album name and asset count |
| POST /api/albums/{id}/assets | JSON `assetId`; idempotent membership |
| DELETE /api/albums/{id}/assets/{assetId} | Remove virtual membership only |
| POST /api/scan, POST /api/retry | Queue reconciliation / rebuild previews |
| GET /api/stats | Counts, queue, failed assets, scan state, last scan error |

Every API except health/login and the scoped public-link read routes requires authentication. Mutating requests require `X-Gallery-Request: 1`; browser Origin must match the request host. No permissive CORS is enabled. Configure your reverse proxy to preserve the original Host. Passwords use fixed-cost Argon2id (64 MiB, three iterations, two lanes). Tokens are random, only their hashes are stored, and sessions stop working after username/password-hash changes. Login limits use direct peer IP (five attempts per five minutes), including failed attempts; behind a single proxy this limit is shared.

Use Docker Engine with Compose v2. This repository includes a `verify` Docker target to run native Linux real-media tests: `docker build --target verify .`. Linux amd64/arm64 binaries cross-compile; actual containers and NAS architecture still require runtime acceptance. For a NAS without build resources, build the image on another computer for the NAS architecture and transfer it with `docker save` / `docker load`.


## Additional media workflows

- `GET /api/assets/{id}/full-image`: authorized full-resolution rendered PNG when required.
- `GET` / `POST /api/assets/{id}/playback`: inspect or request a compatible video job.
- `GET /api/assets/{id}/playback/file`: authorized, seekable compatible MP4.
- `POST /api/assets/bulk`: selected IDs and action `move`, `trash`, `restore` or `delete`; permanent deletion requires `confirm: true` and items already in Trash.
- `GET` / `POST /api/cache`: administrator cache status and maintenance.
- `GET /api/processing`, `POST /api/processing/retry`: administrator queue inspection/retry.
- `/api/uploads/sessions`: resumable upload creation/listing; per-session PATCH chunks, POST complete and DELETE cancellation.

For precise payloads and permission checks, the handlers in `internal/api` and their tests are authoritative.
