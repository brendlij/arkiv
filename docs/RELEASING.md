# Releasing Arkiv

Arkiv uses Semantic Versioning, short-lived release branches, protected pull requests, annotated tags, GitHub Releases, and multi-architecture images in GitHub Container Registry (GHCR).

## Branch model

- `main` is always releasable and should be protected on GitHub.
- Feature and fix branches merge into `main` through pull requests.
- `release/vX.Y.Z` branches contain only release metadata or final release fixes and are deleted after merge.
- Tags use exactly `vX.Y.Z`. Do not move or reuse a published tag.

Configure the `main` branch protection rule to require pull requests, one approval, resolved conversations, and the `Test`, `Container`, and `Release metadata` checks. Block force pushes and branch deletion. Enable automatic deletion of merged branches.

## Prepare a release pull request

Open **Actions → Prepare release → Run workflow** on GitHub and select `patch`, `minor`, or `major`. The workflow calculates the next version, updates `web/package.json` and its lock file, moves the `Unreleased` changelog entries into a dated release section, creates `release/vX.Y.Z`, and opens a pull request.

Create a fine-grained personal access token with repository **Contents: Read and write** and **Pull requests: Read and write** permissions, then save it as the repository Actions secret `RELEASE_TOKEN`. The dedicated token allows the generated branch and pull request to trigger the normal CI workflows.

The equivalent local command is:

```sh
git switch main
git pull --ff-only
version="$(node scripts/prepare-release.mjs minor)"
git switch -c "release/v$version"
```

For local preparation, create the branch matching the version printed by the script, commit the generated files, and open the pull request. Normally the GitHub action performs these steps.

```sh
git add CHANGELOG.md web/package.json web/package-lock.json
git commit -m "release: prepare v0.2.0"
git push -u origin release/v0.2.0
gh pr create --base main --title "release: v0.2.0" --fill
```

The release-PR workflow checks the branch name, package version, dated changelog entry, and retained `Unreleased` section. CI tests Go and Vue and builds the production container.

## Tag and publish

Merging the release PR into `main` starts publication automatically. The workflow verifies the package version and changelog, runs the build and tests, publishes `linux/amd64` and `linux/arm64` images, creates the annotated `vX.Y.Z` tag, and creates a GitHub Release from the curated changelog section. A manually pushed matching tag can still restart publication recovery.

Published image tags are:

```text
ghcr.io/brendlij/arkiv:0.2.0
ghcr.io/brendlij/arkiv:0.2
ghcr.io/brendlij/arkiv:latest
```

Set the GHCR package to public if NAS hosts should pull it without registry authentication. Keep `GITHUB_TOKEN` package and contents write permissions enabled for Actions.

## Hotfixes and failed releases

Prepare hotfixes with the same process and increment the patch version. If publishing fails, fix the workflow or infrastructure and rerun the failed job. Never replace a tag that users may already have pulled; issue a new patch version when released content itself is wrong.
