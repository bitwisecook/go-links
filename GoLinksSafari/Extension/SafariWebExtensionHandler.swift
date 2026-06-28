import SafariServices
import os.log

/// Native entry point for the Safari Web Extension. The extension is driven
/// entirely by `Resources/background.js` (declarativeNetRequest), so this
/// handler only needs to satisfy the extension point and echo native messages.
final class SafariWebExtensionHandler: NSObject, NSExtensionRequestHandling {
    func beginRequest(with context: NSExtensionContext) {
        let request = context.inputItems.first as? NSExtensionItem
        let message = request?.userInfo?[SFExtensionMessageKey]
        os_log(.default, "GoLinks received native message: %@", String(describing: message))

        let response = NSExtensionItem()
        response.userInfo = [SFExtensionMessageKey: ["echo": message ?? NSNull()]]
        context.completeRequest(returningItems: [response], completionHandler: nil)
    }
}
