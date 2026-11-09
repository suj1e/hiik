# Runbook: release

hiik ships unsigned packages. There is no Developer ID, no notarization, no
Authenticode signature and no package signature, so no signing secrets are
needed. macOS builds are ad-hoc signed, which runs on the Mac that built them
but is quarantined elsewhere; Windows shows SmartScreen's warning; Linux
package managers do not verify an origin.

## Prepare

1. `docs/release.md` has a `## X.Y.Z` section for the release.
2. `apps/desktop/mygo.json` has the same `version`.
3. `just check` is green on macOS. It vets and tests `apps/desktop` and
   `scripts`, runs the three checks, and cross-builds darwin/arm64,
   linux/amd64 and windows/amd64.
4. `apps/desktop/resources/icon.png` is current; `just icon` regenerates it
   from `scripts/gen-icon`.
5. Commit, tag and push:

   ```sh
   git tag vX.Y.Z
   git push origin vX.Y.Z
   ```

## Build

`.github/workflows/release.yml` runs on a `v*` tag on `macos-14`:

1. Checks the tag matches `mygo.json` (`vX.Y.Z`).
2. Installs NSIS with `brew install nsis`, for the Windows installer.
3. Builds, unsigned:

   ```sh
   cd apps/desktop
   go tool mygo build -platform darwin/universal,windows/amd64,linux/amd64
   ```

   This is what `just release` runs locally.
4. Fails unless a `.dmg`, a `hiik-setup-*.exe` and a `.deb` exist.
5. Uploads `apps/desktop/dist/**` as the `hiik-vX.Y.Z` workflow artifact.
6. Creates the GitHub release and attaches every `.dmg`, `.exe`, `.deb` and
   `.tar.gz` under `dist/`:

   ```sh
   gh release create "$GITHUB_REF_NAME" <files> --generate-notes
   ```

No step reads a signing or notarization secret.

## Artifacts

`X.Y.Z` is the version in `apps/desktop/mygo.json`. Each platform builds into
its own directory under `apps/desktop/dist/`.

| Platform | Path | What it is |
| --- | --- | --- |
| macOS | `dist/darwin-universal/hiik.app` | universal (arm64 + amd64) app bundle |
| macOS | `dist/darwin-universal/hiik-X.Y.Z.dmg` | disk image, no arch suffix for a universal build |
| Windows | `dist/windows-amd64/hiik.exe` | the app |
| Windows | `dist/windows-amd64/hiik-setup-X.Y.Z-amd64.exe` | NSIS installer |
| Linux | `dist/linux-amd64/hiik` | the app |
| Linux | `dist/linux-amd64/hiik.desktop`, `hiik.png` | desktop entry and icon (from `resources/icon.png`) |
| Linux | `dist/linux-amd64/hiik-X.Y.Z-linux-amd64.tar.gz` | the app and its desktop files |
| Linux | `dist/linux-amd64/hiik_X.Y.Z_amd64.deb` | Debian package, installed under `/opt/hiik` |
| Linux | `dist/linux-amd64/PKGBUILD` | builds the Arch package from the archive |
| Linux | `dist/linux-amd64/install.sh` | installs the app for the user under `~/.local` |

`hiik.app` and the loose Linux files are kept in the workflow artifact, but
the release attaches only the compressed packages listed above.

## Verify

Download each artifact from the GitHub release, not from the build directory.

### macOS

```sh
hdiutil verify hiik-X.Y.Z.dmg
hdiutil attach hiik-X.Y.Z.dmg
codesign -dv --verbose=4 /Volumes/hiik/hiik.app # Signature=adhoc
spctl -a -vv /Volumes/hiik/hiik.app             # rejected: no Developer ID
```

`spctl` rejecting the app is expected. Copy it to Applications, then
right-click → Open once, or allow it under System Settings → Privacy &
Security. Confirm macOS 13.0 or later.

### Windows

```powershell
Get-AuthenticodeSignature .\hiik-setup-X.Y.Z-amd64.exe   # Status NotSigned
.\hiik-setup-X.Y.Z-amd64.exe
Get-AuthenticodeSignature .\hiik.exe                      # Status NotSigned
```

Run the installer as a user, launch hiik from the Start menu, and confirm the
app starts from `hiik.exe` too. SmartScreen's "more info → run anyway" is
expected.

### Linux

```sh
dpkg-deb -I hiik_X.Y.Z_amd64.deb
dpkg-deb -c hiik_X.Y.Z_amd64.deb
sudo apt install ./hiik_X.Y.Z_amd64.deb
hiik

tar -tzf hiik-X.Y.Z-linux-amd64.tar.gz
mkdir -p /tmp/hiik-portable && tar -xzf hiik-X.Y.Z-linux-amd64.tar.gz -C /tmp/hiik-portable
/tmp/hiik-portable/hiik
```

The Debian package needs the dependencies its control file lists (the
terminal is native, so there is no cgo). The archive is the same app and
should run after extraction.

### On every platform

Create a space, run a shell, split, type, resize, close and reopen the
window. Attach to a remote machine if one is configured. Check that Settings
→ Updates does not claim "up to date" when its update check fails.

### Runtime acceptance script

`scripts/accept-runtime` checks that the Windows and Linux artifacts exist
under `dist/` and prints the launch steps above. It never signs, installs or
launches anything:

```sh
just accept
# or, for one platform:
cd scripts && go run ./accept-runtime -platform windows
cd scripts && go run ./accept-runtime -platform linux
```

Run it after `just release`. The release workflow runs the same check on
`windows-latest` and `ubuntu-latest` against the uploaded artifacts, so a
release with a missing or misnamed package fails before publication. The
actual window launch stays manual because it needs a desktop session.

## Roll back

Delete the GitHub release and the pushed tag; users keep the version they
have installed. If an update feed is enabled, publish the previous version's
manifest again before removing the tag.

## If a build fails

- **`hiik-setup-*.exe` missing**: NSIS was not installed, and mygo warns
  instead of failing. `brew install nsis` and rerun.
- **Tag check fails**: bump `version` in `apps/desktop/mygo.json` and
  `docs/release.md`, or tag the matching version.
- **libghostty-vt missing**: check mygo's native resources step; the app
  requires it at run time.
- **DMG step fails**: disk images need macOS; the workflow must run on a Mac.
- **`hc` missing**: only the opt-in E2E tests need a core, and CI does not
  run them.
- **macOS version**: the app requires 13.0, from `macos.minimumSystemVersion`
  in `mygo.json`.
