# Gallery accounts and sharing

## First run and upgrade

The configured `ARKIV_USERNAME` and `ARKIV_PASSWORD_HASH` seed the first administrator only when the users table is empty. Passwords and subsequent accounts live in SQLite. Changing the environment does not reset existing credentials. Keep the database and its backups private. Migration 004 signs out old sessions and assigns existing albums and favorites to the bootstrap administrator. Back up `/data` before upgrading; an older binary is not a supported rollback after migration.

## Administrator workflow

1. Sign in and open **People & access → Add person**.
2. Supply a unique username, display name, member/admin role, and a temporary password of at least 12 characters. Share that password privately outside Gallery.
3. Select the person and assign an existing configured library folder, for example `Family/Julia`, with a friendly personal label. An empty relative path grants the entire library. Folder names are case-sensitive database paths; use the spelling shown by the folder browser. Subfolders are included, but similarly prefixed siblings are not.
4. The person signs in, replaces their temporary password, signs in again, and opens **Shared with me**. Their timeline and search contain only accessible photos.
5. To remove access, revoke a folder or disable the account. Resetting a password requires a new temporary password and revokes all sessions. Another administrator can reset a forgotten password. Administrators cannot disable/demote themselves, and at least one active administrator is required.

Administrators can read all configured libraries and manage all albums. Their normal Albums screen lists their own albums; Shared albums lists albums explicitly shared with them. Members cannot administer users or trigger scans/rebuilds. Accounts can be disabled rather than deleted, preserving ownership history. The self-hosted application supports up to 1,000 accounts.

Assigned folders are read-only views of existing source directories. Gallery does not create, upload into, move, rename or delete source folders or originals. Overlapping grants intentionally allow multiple users to see the same originals; each user's favorites remain private.

## Album workflow

Create a private album from **Albums → New album**, then open a photo from an assigned folder and use **+** to add it. Open the album's **Share & manage** panel to set its description and share it with an exact existing username. Enter that username again with a different role to update their permission.

| Role | Permissions |
| --- | --- |
| Owner | View/download, add/remove items, rename/delete the virtual album, grant/revoke members |
| Viewer | View included photos and download originals; keep personal favorites |
| Contributor | Viewer permissions plus add photos from their own assigned folders and remove their own additions |
| Administrator | Full management access, including albums belonging to other users |

Recipients find albums under **Shared albums**. Sharing exposes the included originals and their metadata, not the containing folder or other files. A borrowed photo cannot be added to another album without direct folder access. An album shares a curated set of items; assigning a folder shares its current and future descendants.

Revocation applies to subsequent API requests, including cached-thumbnail URLs, search, statistics and original downloads. Thumbnails use `Cache-Control: no-store`. Previously downloaded files and content already displayed in an open page cannot be recalled. The album owner's account and the person who added each photo must remain enabled, and that contributor must retain source-folder access. Another independent folder grant or album share can still provide access.

Deleting an album deletes only virtual membership and sharing records. Originals remain untouched. Email invitations, registration and automated email password recovery are not included. Public links and timed sharing are described below.

## Account security

**My account** changes a password after verifying the current password and signs out every session. Local temporary-password accounts cannot access library APIs until they complete this step. With trusted-proxy authentication, `Remote-User` must match an enabled provisioned account; credentials and password changes are managed by the identity provider instead. Only explicitly configured trusted proxy peer addresses can supply that identity.

The database is authoritative for enabled state and role on every request. Auth checks and server-side SQL permission filters protect metadata, originals, previews, favorites, folders, search, counts and album membership. Sensitive operational error text is restricted to administrators. Original source mounts must remain read-only in deployment.

## Additional API routes

All mutation routes retain the CSRF header requirement. Album lists support `scope=owned|shared` (omitted means all albums accessible to the caller). Assets include `canAdd` and album-context `canRemove` UI hints; the server independently checks each mutation.

| Route | Purpose |
| --- | --- |
| POST `/api/auth/password` | `currentPassword`, `newPassword`; revokes sessions |
| GET/POST `/api/admin/users` | List accounts / create with `username`, `displayName`, `role`, `password` |
| PATCH `/api/admin/users/{id}` | `displayName`, `role`, `enabled` |
| POST `/api/admin/users/{id}/password` | Set temporary `password` |
| GET `/api/admin/libraries` | Enabled library identifiers and names |
| GET/POST `/api/admin/users/{id}/folders` | List / assign `libraryId`, `folder`, `label` |
| DELETE `/api/admin/users/{id}/folders/{grantId}` | Revoke a folder grant |
| GET `/api/me/folders` | Personal labeled folder roots |
| PATCH/DELETE `/api/albums/{id}` | Update `name`, `description` / delete virtual album |
| GET/POST `/api/albums/{id}/members` | List / upsert `username`, `role` (viewer or contributor) |
| DELETE `/api/albums/{id}/members/{userId}` | Revoke a recipient |


## Public links and timed access

Inside an album, open **Share & manage** and choose **Selected users** or **Public link**. These are independent sharing channels: a public link can coexist with invitations. Switching the editor tab does not revoke existing access; the status at the top shows any public link, and each channel has explicit revoke controls.

Both channels support an optional start date/time and an expiration. The form defaults to seven days, offers 1/7/30-day presets, and supports **No expiry**. Empty start means immediately. Times are entered in your browser's local timezone, sent as RFC3339 timestamps with offsets, stored in UTC, and enforced by the server. An access window includes its start and excludes its expiration. Existing invitations migrate without an expiration. Use **Edit** beside a user to change their role or access window; expired invitations cannot grant access or contribution rights.

Public links grant anonymous, read-only access to the album's current items. Anyone holding or receiving the URL can use it during its window. Previews are enabled by default; **Allow original downloads and video playback** explicitly enables source files. Source files may contain EXIF/location metadata. Without this setting, videos display their still preview. Public listing responses omit source-folder paths, location, camera details and user identity. Shared previews themselves remain downloadable by browsers; this setting controls original-file access, not copy protection.

**Create public link** returns a cryptographically random 256-bit URL. Copy and retain that URL; only its SHA-256 hash is stored in SQLite, so reopening the editor cannot retrieve it. **Replace link with these settings** rotates the secret and invalidates the old URL immediately. There is one public link per album. **Revoke public link** deletes that link's access while leaving user invitations intact. If source grants are removed, contributors are disabled, or the owner is disabled, the affected photos stop being served through public links as well.

Expiration and revocation are checked on each public request, including thumbnails, originals and video range requests. Content already displayed or downloaded cannot be recalled; an already-started transfer may finish. The public page clears itself at the advertised expiry and shows an unavailable state for expired, scheduled, invalid and revoked links. Pages/API responses are marked no-store/noindex with no-referrer policy; these are browser/crawler directives, not a guarantee against someone reposting the content. Treat public URLs as secrets and avoid retaining full URL paths in reverse-proxy access logs.

A generated URL uses the browser's current origin. Create/copy it from your externally reachable HTTPS Gallery hostname for outside recipients; a localhost URL only works on the same computer. This feature does not publish your server or alter your reverse proxy.

Migration 005 adds timing to invitations and the public-link table. New routes:

| Route | Purpose |
| --- | --- |
| GET `/api/albums/{id}/public-link` | Owner/admin: retrieve settings, never the token |
| POST `/api/albums/{id}/public-link` | Owner/admin: create/rotate with optional `startsAt`, `expiresAt`, `allowDownloads` |
| DELETE `/api/albums/{id}/public-link` | Owner/admin: revoke the public link |
| GET `/api/public/{token}` | Anonymous paginated album summary; opaque `cursor` from `nextCursor` |
| GET `/api/public/{token}/assets/{id}/thumbnail/{size}` | Anonymous scoped preview; 256, 1024 or 2048 |
| GET `/api/public/{token}/assets/{id}/original` | Scoped original, only when enabled |

The existing member POST accepts optional `startsAt` and `expiresAt` RFC3339 strings; empty strings mean immediate/no-expiry. These replace the recipient's previous window. Member GET returns Unix-second timing values, with zero meaning unbounded. Anonymous public routes never adopt a signed-in visitor's broader permissions.

## Finding ownership and access

My library lists your assigned upload folders. Shared with me combines albums shared with you and assigned folders belonging to other users or server libraries. Server libraries browses accessible configured storage, including the managed Uploads library. Opening a folder preserves its originating navigation section.

Folder pages and the viewer Details panel show ownership, storage behavior, and a Who has access? disclosure. Owners and administrators can see enabled users who currently have access to items in that scope, plus currently active public albums. A folder audience may include people who can access only some of its items; it does not imply full-folder access. Scheduled future shares are not presented as current access. Other viewers receive an explanation without the audience list. Admin visibility is explicitly disclosed. These are read-only explanations; existing permissions are unchanged.

## Uploaded folders

Uploaded folders now have persistent records, including empty folders. Owners can create, rename/move subtrees and grant/revoke access to existing users from Share folder. These grants use the same inherited rules as assigned server folders. Administrators can assign uploaded folders, including virtual empty folders, in People & access. Explicit grants follow renamed/moved folders; inherited grants are evaluated at the destination. Storage ownership does not transfer. The upload dialog shows the chosen destination and warns that folder access applies to new uploads.
