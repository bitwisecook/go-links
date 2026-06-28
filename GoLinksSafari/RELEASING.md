# Releasing Go Links to TestFlight

The app is on App Store Connect and TestFlight.

| Item | Value |
|------|-------|
| App name | Go Links |
| App Store Connect app ID | `6785280484` |
| App bundle ID | `com.bragi0.golinks` |
| Extension bundle ID | `com.bragi0.golinks.Extension` |
| **Team (App Store / ASC API key)** | **`QVUFB5676H`** (James Deucker) |
| Signing | Manual, `Apple Distribution` + App Store profiles |
| Profiles | `GoLinks AppStore`, `GoLinks Ext AppStore` (App Store) |

> ⚠️ Two teams exist on this Mac. The **ASC API key, bundle IDs, app record, cert,
> and profiles all live under `QVUFB5676H`** — *not* the local Apple Development
> team `L7TJYE37QU`. The project's `DEVELOPMENT_TEAM` is set to `QVUFB5676H`.
> App **records cannot be created via the API** (the `apps` resource is read-only
> for CREATE) — new apps must be made once in the App Store Connect web UI.

## Credentials (not in git)

- ASC API key: `/tmp/AuthKey_D9CM6S6D44.p8` · key id `/tmp/asc_admin_keyid.txt` · issuer `/tmp/asc_issuer.txt`
- Distribution identity (persisted): `/tmp/golinks_dist.p12` (password `golinks`)
- App Store profiles installed in `~/Library/MobileDevice/Provisioning Profiles/`

## Ship a new build

1. Bump the build number in `project.yml` (`CURRENT_PROJECT_VERSION`) and/or
   `MARKETING_VERSION`, then `xcodegen generate`.
2. Make sure a keychain holds the distribution identity and is unlocked, e.g.:
   ```bash
   security create-keychain -p golinks build.keychain
   security unlock-keychain -p golinks build.keychain
   security import /tmp/golinks_dist.p12 -k build.keychain -P golinks -T /usr/bin/codesign -A
   security list-keychains -d user -s build.keychain "$HOME/Library/Keychains/login.keychain-db"
   security set-key-partition-list -S apple-tool:,apple:,codesign: -s -k golinks build.keychain
   ```
3. Archive → export → upload:
   ```bash
   xcodebuild archive -project GoLinks.xcodeproj -scheme GoLinks \
     -destination 'generic/platform=iOS' -archivePath build/GoLinks.xcarchive \
     CODE_SIGN_IDENTITY="Apple Distribution"
   xcodebuild -exportArchive -archivePath build/GoLinks.xcarchive \
     -exportOptionsPlist ExportOptions.plist -exportPath build/export
   xcrun altool --upload-app -f build/export/GoLinks.ipa -t ios \
     --apiKey D9CM6S6D44 --apiIssuer "$(cat /tmp/asc_issuer.txt)"
   ```
   (`ExportOptions.plist` = manual signing, team `QVUFB5676H`, the two profiles above.)
4. After processing, set export compliance and attach to the Internal group
   (the helper used during setup PATCHes `usesNonExemptEncryption=false` and
   POSTs the build to the internal `betaGroups` relationship).

## macOS (native, Mac App Store / TestFlight)

Same app record `6785280484`, same bundle IDs, **native macOS targets** (`GoLinks-mac`,
`GoLinks-mac Extension`) — no Mac Catalyst. The macOS platform is enabled on the record.

Extra signing material beyond the iOS set:
- **Mac Installer Distribution** cert (`/tmp/golinks_installer.p12`, pw `golinks`) —
  signs the `.pkg`. The app itself signs with the same **Apple Distribution** cert.
- **MAC_APP_STORE** profiles `GoLinks Mac AppStore` / `GoLinks Mac Ext AppStore`,
  installed as `.provisionprofile` in `~/Library/MobileDevice/Provisioning Profiles/`.

Ship a new macOS build (after importing both `.p12`s into an unlocked keychain):
```bash
xcodebuild archive -project GoLinks.xcodeproj -scheme GoLinks-mac \
  -destination 'generic/platform=macOS' -archivePath build/GoLinks-mac.xcarchive \
  CODE_SIGN_IDENTITY="Apple Distribution"
xcodebuild -exportArchive -archivePath build/GoLinks-mac.xcarchive \
  -exportOptionsPlist macExportOptions.plist -exportPath build/macexport   # -> GoLinks.pkg
xcrun altool --upload-app -f build/macexport/GoLinks.pkg -t macos \
  --apiKey D9CM6S6D44 --apiIssuer "$(cat /tmp/asc_issuer.txt)"
```
`macExportOptions.plist` = `app-store-connect`, manual, `signingCertificate=Apple Distribution`,
`installerSigningCertificate=3rd Party Mac Developer Installer`, the two Mac profiles.
Build numbers are **per-platform**, so iOS and macOS can both be build 1.

## Gotchas hit during first upload

- **App icon alpha:** the 1024 marketing icon must be **opaque** (no alpha). The
  asset catalog icon is generated opaque; the extension toolbar icons keep alpha.
- **`CODE_SIGN_IDENTITY`:** xcodegen injects `[sdk=iphoneos*] = iPhone Developer`,
  which overrides project settings — pass `CODE_SIGN_IDENTITY="Apple Distribution"`
  on the `xcodebuild` command line.
- **PKCS#12 import:** OpenSSL 3 needs `-legacy` to produce a `.p12` that Apple's
  `security import` accepts.
- **macOS `LSApplicationCategoryType`:** the Mac app's Info.plist must set a category
  (e.g. `public.app-category.productivity`) or the pkg upload is rejected (90242).
- **`/v1/builds?filter[app]=` is flaky** — it can return empty even for VALID builds.
  Query `/v1/apps/{id}/preReleaseVersions` then `/v1/preReleaseVersions/{id}/builds`
  to reliably find a build + its `processingState`.
