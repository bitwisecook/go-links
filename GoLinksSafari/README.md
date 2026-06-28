# Go Links — Safari Web Extension

A Safari extension for **iPhone, iPad, and macOS** that turns the address bar into a
go-links launcher. Type either form:

| You type | What happens |
|----------|--------------|
| `go thing` | Your default search engine is asked to search for *"go thing"*; the extension catches that and redirects to `https://go.bragi0.com/thing`. |
| `go/thing` | Safari treats this as the URL `http://go/thing`; the extension rewrites it to `https://go.bragi0.com/thing`. **Works with any search engine — recommended.** |
| `go jira PROJ-42` | Everything after the link name is passed to the link as arguments (`/jira+PROJ-42`). |

It also adds a toolbar **popup** (quick launcher with live suggestions from the
server's `/api/suggestions`) and an **options page** to configure the server URL,
trigger word, and which engines to intercept.

## Why two forms? (the Safari constraint)

Chrome has an `omnibox` keyword API; **Safari does not**, and Safari can't install
OpenSearch engines either. So an extension cannot see what you type in the address
bar *before* it becomes a navigation. The only hooks available are navigation
redirects (`declarativeNetRequest`), which is exactly what this extension uses:

- **`go thing`** only works if your **default search engine** is one we intercept
  (Google, DuckDuckGo, Bing, Yahoo, Ecosia, Brave, Startpage — toggleable in
  settings). It relies on Safari routing the query to that engine.
- **`go/thing`** is bullet-proof: it never depends on the search engine, because
  Safari resolves `go/…` as a hostname. If in doubt, use the slash form.

The redirect target is the existing go-links server in this repo, which already
understands `name`, `name args`, and `name+arg1+arg2` and 302-redirects
accordingly (see `handlers.go`).

## Build

Requires Xcode 16+ and [XcodeGen](https://github.com/yonaskolb/XcodeGen)
(`brew install xcodegen`).

```bash
cd GoLinksSafari
xcodegen generate          # produces GoLinks.xcodeproj from project.yml
open GoLinks.xcodeproj
```

There are **two schemes** (native targets, no Mac Catalyst):

- **GoLinks** — iPhone / iPad (iOS Simulator or device).
- **GoLinks-mac** — a native macOS app + macOS Safari extension (destination **My Mac**).

To run on a real device or in macOS Safari you must set a **signing team**:
select each target → *Signing & Capabilities* → choose your team (or set
`DEVELOPMENT_TEAM` in `project.yml` and re-run `xcodegen generate`). Signing for
TestFlight/App Store is documented in `RELEASING.md`.

Command-line sanity build (no signing, Simulator):

```bash
xcodebuild -project GoLinks.xcodeproj -scheme GoLinks \
  -sdk iphonesimulator -destination 'platform=iOS Simulator,name=iPhone 17 Pro' \
  CODE_SIGNING_ALLOWED=NO build
```

## Enable the extension

**macOS:** Safari ▸ Settings ▸ Extensions ▸ enable **Go Links** ▸ set its website
permission to *Allow on Every Website*.

**iOS / iPadOS:** run the app once, then Settings ▸ Apps ▸ Safari ▸ Extensions ▸
**Go Links** ▸ turn on and *Allow* on all websites. (Or tap the page-menu /
puzzle-piece button in Safari ▸ Manage Extensions.)

The extension needs broad host access (`<all_urls>`) for two reasons: search
engines live on many domains (e.g. every Google ccTLD), and it must be able to
redirect to whatever server you configure. It only ever **redirects navigations**
and **fetches suggestions from your server** — it does not read page content.

## Configure

Open the extension's **options page** (popup ▸ *Settings*, or the Extensions
settings panel) to change:

- **Server URL** — defaults to `https://go.bragi0.com`.
- **Trigger word** — defaults to `go`.
- **Slash redirect** — the `go/thing` form (on by default).
- **Engines** — which default-search-engine queries to intercept.

Settings are stored in `browser.storage.local`; the background worker rebuilds its
`declarativeNetRequest` rules immediately on save.

## Project layout

```
GoLinksSafari/
├── project.yml                     # XcodeGen spec (native iOS + native macOS, 4 targets)
├── App/                            # SwiftUI container app (Swift 6, cross-platform)
│   ├── GoLinksApp.swift
│   ├── ContentView.swift           # onboarding; #if os(macOS) AppKit / else UIKit
│   ├── Assets.xcassets/            # iOS 1024 + macOS idiom icons (opaque)
│   ├── Info.plist                  # iOS app
│   ├── Info-mac.plist              # macOS app (LSApplicationCategoryType, etc.)
│   └── GoLinksMac.entitlements     # macOS sandbox + network.client
└── Extension/                      # Safari Web Extension (MV3), shared by both OSes
    ├── SafariWebExtensionHandler.swift
    ├── Info.plist
    ├── GoLinksMacExtension.entitlements
    └── Resources/
        ├── manifest.json
        ├── background.js           # builds dynamic DNR redirect rules from settings
        ├── popup.{html,css,js}     # quick launcher + live suggestions
        ├── options.{html,css,js}   # settings UI
        └── images/                 # generated icons
```

## Changing the default server

The default is set in **two** places (keep them in sync):

- `Extension/Resources/background.js` → `DEFAULTS.serverBase`
- `App/ContentView.swift` → `serverBase`

Per-install overrides are done in the options page and don't require a rebuild.
