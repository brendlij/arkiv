# Releasing Arkiv

Arkiv uses Semantic Versioning, short-lived release branches, protected pull requests, annotated tags, GitHub Releases, and multi-architecture images in GitHub Container Registry (GHCR).

## Branch model

- `main` is always releasable and should be protected on GitHub.
- Feature and fix branches merge into `main` through pull requests.
- `release/vX.Y.Z` branches contain only release metadata or final release fixes and are deleted after merge.
- Tags use exactly `vX.Y.Z`. Do not move or reuse a published tag.

Configure the `main` branch protection rule to require pull requests, one approval, resolved conversations, and the `Test`, `Container`, and `Release metadata` checks. Block force pushes and branch deletion. Enable automatic deletion of merged branches.

## Prepare a release pull request

Start from an up-to-date `main` branch:

```sh
git switch main
git pull --ff-only
git switch -c release/v0.2.0
npm --prefix web version 0.2.0 --no-git-tag-version
```

Move the relevant entries from `Unreleased` into a dated `## [0.2.0] - YYYY-MM-DD` section in `CHANGELOG.md`. Keep an empty `Unreleased` section for future work. Group entries under `Added`, `Changed`, `Deprecated`, `Removed`, `Fixed`, or `Security`; describe user impact rather than commit history.

Commit and open the release PR:

```sh
git add CHANGELOG.md web/package.json web/package-lock.json
git commit -m "release: prepare v0.2.0"
git push -u origin release/v0.2.0
gh pr create --base main --title "release: v0.2.0" --fill
```

The release-PR workflow checks the branch name, package version, dated changelog entry, and retained `Unreleased` section. CI tests Go and Vue and builds the production container.

## Tag and publish

After the release PR is approved and merged, tag the merge commit from an updated `main` branch:

```sh
git switch main
git pull --ff-only
git tag -a v0.2.0 -m "Arkiv v0.2.0"
git push origin v0.2.0
```

The tag workflow verifies that the tag is on `main` and matches both `web/package.json` and `CHANGELOG.md`. It runs the build and tests, publishes `linux/amd64` and `linux/arm64` images, and creates a GitHub Release using the curated changelog section.

Published image tags are:

```text
ghcr.io/brendlij/arkiv:0.2.0
ghcr.io/brendlij/arkiv:0.2
ghcr.io/brendlij/arkiv:latest
```

Set the GHCR package to public if NAS hosts should pull it without registry authentication. Keep `GITHUB_TOKEN` package and contents write permissions enabled for Actions.

## Hotfixes and failed releases

Prepare hotfixes with the same process and increment the patch version. If publishing fails, fix the workflow or infrastructure and rerun the failed job. Never replace a tag that users may already have pulled; issue a new patch version when released content itself is wrong.
