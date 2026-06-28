import SwiftUI

@main
struct GoLinksApp: App {
    var body: some Scene {
        WindowGroup {
            ContentView()
                #if os(macOS)
                .frame(minWidth: 520, minHeight: 640)
                #endif
        }
        #if os(macOS)
        .defaultSize(width: 560, height: 720)
        .windowResizability(.contentMinSize)
        #endif
    }
}
