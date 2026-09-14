# Configuration

All application settings use the `ARKIV_*` prefix. `.env.example` contains the settings used by the supplied Compose file. Docker Compose reads `.env`; the native binary reads process environment variables only.

## Docker host settings

These variables are interpreted by Compose, not by the server:

| Variable           | Default                         | Purpose                                                                       |
| ------------------ | ------------------------------- | ----------------------------------------------------------------------------- |
| `ARKIV_IMAGE`      | `ghcr.io/brendlij/arkiv:latest` | Published container image; pin a version tag for reproducible NAS deployments |
| `ARKIV_PHOTOS_DIR` | `./photos`                      | Canonical original storage on the Docker host; mounted writable at `/photos`  |
| `ARKIV_DATA_DIR`   | `./data`                        | Persistent database and application state; mounted at `/data`                 |
| `ARKIV_CACHE_DIR`  | `./cache`                       | Rebuildable derivatives; mounted at `/cache`                                  |
| `ARKIV_HOST_PORT`  | `8090`                          | Port published on the Docker host; mapped to container port 8090              |
| `TZ`               | `Europe/Berlin`                 | Container timezone, also used for image dates without an EXIF offset          |

Relative host paths resolve relative to the Compose project. All three directories must already exist. The container uses UID/GID 10001:10001 and requires write access to all three default mounts. NAS ACLs can override ordinary Unix mode bits. Preserve the existing photo and data paths when upgrading.

## Server settings

| Variable                   | Default                     | Purpose                                                                                          |
| -------------------------- | --------------------------- | ------------------------------------------------------------------------------------------------ |
| `ARKIV_DATABASE`           | `/data/gallery.db`          | Absolute SQLite database path; `/data` contains application state only                           |
| `ARKIV_CACHE`              | `/cache`                    | Absolute cache directory, separate from original libraries                                       |
| `ARKIV_LIBRARY`            | `/photos`                   | Absolute root for the primary original-photo library                                             |
| `ARKIV_LIBRARIES`          | unset                       | JSON library array; overrides the single external root                                           |
| `ARKIV_PORT`               | `8090`                      | Server listen port, 1–65535; also used by the health check                                       |
| `ARKIV_USERNAME`           | `admin`                     | First administrator username; bootstrap only                                                     |
| `ARKIV_PASSWORD_HASH`      | required for password login | Argon2id hash from `arkiv hash-password`; bootstrap only, but must remain configured for startup |
| `ARKIV_SCAN_ON_START`      | `true`                      | Scan external libraries at startup                                                               |
| `ARKIV_SCAN_INTERVAL`      | `30m`                       | External-library scan interval, minimum `1m`                                                     |
| `ARKIV_WORKERS`            | `2`                         | Metadata/thumbnail workers, 1–16                                                                 |
| `ARKIV_UPLOAD_DIR`         | `/photos/originals`         | Managed original and resumable-transfer storage; may be nested below the primary library         |
| `ARKIV_UPLOAD_QUOTA_BYTES` | `5368709120`                | Default Arkiv account quota (5 GiB), independent of NAS free space                               |
| `ARKIV_UPLOAD_RETENTION`   | `168h`                      | Incomplete upload retention, `1h`–`2160h`                                                        |
| `ARKIV_SECURE_COOKIES`     | `false`                     | Use `true` when serving through HTTPS                                                            |
| `ARKIV_TRUST_PROXY_AUTH`   | `false`                     | Enable trusted-proxy authentication; requires explicit trusted CIDRs                             |
| `ARKIV_TRUSTED_PROXIES`    | unset                       | JSON array of proxy CIDRs, required when proxy authentication is enabled                         |

The default Compose file exposes the ordinary user settings and fixes database, cache and primary-library paths to match its mounts. `ARKIV_UPLOAD_DIR` is a container path, not a host path. Advanced settings must be added to the service's `environment` block; putting an arbitrary variable into `.env` alone does not pass it to the container. `VIPS_CONCURRENCY=1` is a native libvips setting supplied by the image, not an Arkiv variable.

## Multiple external libraries

Add a read-only mount for each library and pass its **container** path:

```yaml
services:
  arkiv:
    environment:
      ARKIV_LIBRARIES: '[{"id":"photos","name":"Photos","root":"/photos","enabled":true},{"id":"archive","name":"Archive","root":"/archive","enabled":true}]'
    volumes:
      - type: bind
        source: /volume1/archive
        target: /archive
        read_only: true
        bind:
          create_host_path: false
```

Merge this example into the existing service; retain its data, cache and photos mounts. Additional source mounts need only read and traverse access and should remain read-only. IDs must be stable and unique, and roots must not overlap. The managed upload library is added automatically and is the sole allowed nested library root. Changing a root under an existing ID is rejected to protect associations. Disabling a library hides it without discarding its metadata.

## Trusted proxy authentication

Leave this disabled for ordinary password login. If enabled, configure `ARKIV_TRUSTED_PROXIES`, ensure only the trusted proxy can reach the application, and configure that proxy to authenticate requests and overwrite identity headers. Consult [Accounts](ACCOUNTS.md) and the implemented authentication routes before enabling it. This mode is not configured by the default Compose file.

## Test settings

Optional verification flags use the same namespace: `ARKIV_RAW_FIXTURES`, `ARKIV_MEDIA_FIXTURE_ROOT`, `ARKIV_SCALE_TEST` and `ARKIV_INTEGRATION`.
