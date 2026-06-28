import SwiftUI
import SafariServices
#if os(macOS)
import AppKit
#else
import UIKit
#endif

struct ContentView: View {
    /// Must match `DEFAULTS.serverBase` in the extension's background.js.
    private let serverBase = "https://go.bragi0.com"
    private let extensionBundleID = "com.bragi0.golinks.Extension"

    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                header

                card(title: "Two ways to jump", systemImage: "arrow.turn.down.right") {
                    usage(trigger: "go thing", note: "Searches with your default engine; the extension catches it and redirects.")
                    Divider()
                    usage(trigger: "go/thing", note: "Treated as a URL. Always works, regardless of search engine.")
                    Divider()
                    usage(trigger: "go jira PROJ-42", note: "Extra words become arguments passed to the link.")
                }

                card(title: enableTitle, systemImage: "puzzlepiece.extension") {
                    ForEach(Array(enableSteps.enumerated()), id: \.offset) { idx, step in
                        Label {
                            Text(step)
                        } icon: {
                            Text("\(idx + 1)")
                                .font(.caption.bold())
                                .frame(width: 20, height: 20)
                                .background(Circle().fill(Color.accentColor.opacity(0.18)))
                        }
                    }
                }

                buttons
            }
            .padding(24)
            .frame(maxWidth: 620, alignment: .leading)
            .frame(maxWidth: .infinity)
        }
        .tint(Color(red: 0.086, green: 0.639, blue: 0.290))
        .background(backgroundColor)
    }

    private var header: some View {
        HStack(spacing: 12) {
            Text("go")
                .font(.system(size: 30, weight: .bold, design: .rounded))
                .foregroundStyle(.white)
                .padding(.horizontal, 14).padding(.vertical, 6)
                .background(
                    LinearGradient(colors: [Color(red: 0.20, green: 0.78, blue: 0.42),
                                            Color(red: 0.086, green: 0.639, blue: 0.290)],
                                   startPoint: .top, endPoint: .bottom),
                    in: RoundedRectangle(cornerRadius: 12, style: .continuous))
            VStack(alignment: .leading, spacing: 2) {
                Text("Go Links").font(.largeTitle.bold())
                Text(serverHost).font(.callout).foregroundStyle(.secondary)
            }
            Spacer()
        }
    }

    private var buttons: some View {
        VStack(alignment: .leading, spacing: 12) {
            Button {
                open(serverBase)
            } label: {
                Label("Open the go-links server", systemImage: "safari")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)

            Button {
                openSettings()
            } label: {
                Label(settingsButtonTitle, systemImage: "gearshape")
                    .frame(maxWidth: .infinity)
            }
            .buttonStyle(.bordered)
        }
        .controlSize(.large)
        .padding(.top, 4)
    }

    // MARK: - Building blocks

    private func card(title: String, systemImage: String,
                      @ViewBuilder content: () -> some View) -> some View {
        VStack(alignment: .leading, spacing: 12) {
            Label(title, systemImage: systemImage)
                .font(.headline)
                .foregroundStyle(.secondary)
            content()
        }
        .padding(18)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(cardColor, in: RoundedRectangle(cornerRadius: 16, style: .continuous))
    }

    private func usage(trigger: String, note: String) -> some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(trigger)
                .font(.system(.body, design: .monospaced).weight(.semibold))
                .foregroundStyle(Color.accentColor)
            Text(note).font(.callout).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity, alignment: .leading)
    }

    // MARK: - Platform specifics

    private var serverHost: String { URL(string: serverBase)?.host ?? serverBase }

    #if os(macOS)
    private var backgroundColor: Color { Color(nsColor: .windowBackgroundColor) }
    private var cardColor: Color { Color(nsColor: .controlBackgroundColor) }
    private var enableTitle: String { "Enable it in Safari" }
    private var settingsButtonTitle: String { "Open Safari Extension Settings" }
    private var enableSteps: [String] {
        ["Open Safari ▸ Settings ▸ Extensions.",
         "Turn on “Go Links”.",
         "Set its permissions to “Allow on Every Website”.",
         "Try typing  go/thing  in the address bar."]
    }
    #else
    private var backgroundColor: Color { Color(uiColor: .systemGroupedBackground) }
    private var cardColor: Color { Color(uiColor: .secondarySystemBackground) }
    private var enableTitle: String { "Enable it on this device" }
    private var settingsButtonTitle: String { "Open Settings" }
    private var enableSteps: [String] {
        ["Open Settings ▸ Apps ▸ Safari ▸ Extensions (or Safari ▸ Extensions).",
         "Turn on “Go Links” and allow it on all websites.",
         "In Safari, tap the ‹ⒶＡ› / puzzle-piece menu to confirm it’s on.",
         "Try typing  go/thing  in the address bar."]
    }
    #endif

    private func open(_ urlString: String) {
        guard let url = URL(string: urlString) else { return }
        #if os(macOS)
        NSWorkspace.shared.open(url)
        #else
        UIApplication.shared.open(url)
        #endif
    }

    private func openSettings() {
        #if os(macOS)
        SFSafariApplication.showPreferencesForExtension(withIdentifier: extensionBundleID) { _ in }
        #else
        if let url = URL(string: UIApplication.openSettingsURLString) {
            UIApplication.shared.open(url)
        }
        #endif
    }
}

#Preview {
    ContentView()
}
