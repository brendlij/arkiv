import { readFile, writeFile } from "node:fs/promises";

const bump = process.argv[2];
const dryRun = process.argv.includes("--dry-run");
const allowedBumps = new Set(["patch", "minor", "major"]);

if (!allowedBumps.has(bump)) {
  throw new Error(
    "usage: node scripts/prepare-release.mjs patch|minor|major [--dry-run]",
  );
}

const packagePath = "web/package.json";
const lockPath = "web/package-lock.json";
const changelogPath = "CHANGELOG.md";
const packageJson = JSON.parse(await readFile(packagePath, "utf8"));
const packageLock = JSON.parse(await readFile(lockPath, "utf8"));
const match = /^(\d+)\.(\d+)\.(\d+)$/.exec(packageJson.version);

if (!match) {
  throw new Error(`invalid current version: ${packageJson.version}`);
}

let [, major, minor, patch] = match.map(Number);
if (bump === "major") {
  major += 1;
  minor = 0;
  patch = 0;
} else if (bump === "minor") {
  minor += 1;
  patch = 0;
} else {
  patch += 1;
}

const version = `${major}.${minor}.${patch}`;
const changelog = await readFile(changelogPath, "utf8");
const unreleased = "## [Unreleased]";
const unreleasedStart = changelog.indexOf(unreleased);
const unreleasedEnd = unreleasedStart + unreleased.length;
const nextRelease = changelog
  .slice(unreleasedEnd)
  .search(/^## \[\d+\.\d+\.\d+\] - \d{4}-\d{2}-\d{2}\s*$/m);

if (unreleasedStart < 0 || nextRelease < 0) {
  throw new Error(
    "CHANGELOG.md must contain Unreleased followed by a dated release",
  );
}

const releaseStart = unreleasedEnd + nextRelease;
const notes = changelog.slice(unreleasedEnd, releaseStart).trim();
if (!/^### /m.test(notes) || !/^- /m.test(notes)) {
  throw new Error("Unreleased must contain a section and at least one entry");
}

const date = new Date().toISOString().slice(0, 10);
const updatedChangelog = `${changelog.slice(0, unreleasedEnd)}\n\n## [${version}] - ${date}\n\n${notes}\n\n${changelog.slice(releaseStart)}`;

if (!dryRun) {
  packageJson.version = version;
  packageLock.version = version;
  packageLock.packages[""].version = version;
  await Promise.all([
    writeFile(packagePath, `${JSON.stringify(packageJson, null, 2)}\n`),
    writeFile(lockPath, `${JSON.stringify(packageLock, null, 2)}\n`),
    writeFile(changelogPath, updatedChangelog),
  ]);
}

process.stdout.write(version);
