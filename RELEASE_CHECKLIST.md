# Release checklist

## One-time setup

- [ ] Add the GitHub Actions secret `RELEASE_TOKEN` with repository **Contents: Read and write** and **Pull requests: Read and write** permissions.
- [ ] Allow GitHub Actions to publish packages.
- [ ] Make the GHCR package public for anonymous NAS pulls.

## Every release

- [ ] Merge completed changes into `main`.
- [ ] Add user-visible changes under `## [Unreleased]` in `CHANGELOG.md`.
- [ ] Open **GitHub → Actions → Prepare release → Run workflow**.
- [ ] Select `patch`, `minor`, or `major`.
- [ ] Review the generated `release/vX.Y.Z` pull request.
- [ ] Wait for `Test`, `Container`, and `Release metadata` checks.
- [ ] Merge the release pull request.
- [ ] Confirm the `vX.Y.Z` tag and GitHub Release exist.
- [ ] Confirm `ghcr.io/brendlij/arkiv:X.Y.Z` and `latest` are available.

## NAS update

```sh
docker compose pull
docker compose up -d
docker compose logs --tail=100 arkiv
```
