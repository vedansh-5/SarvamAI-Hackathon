import AppKit

final class QuietText: NSTextView {
    override func accessibilitySelectedText() -> String? { nil }
    override func accessibilitySelectedTextRange() -> NSRange { NSRange(location: 0, length: 0) }
}

let args = CommandLine.arguments
let quiet = args.count > 1 && args[1] == "quiet"
let word = args.count > 2 ? args[2] : "bedazzled"
let text = "VANS reads this: \(word), and nothing else."

let app = NSApplication.shared
app.setActivationPolicy(.regular)

let menu = NSMenu()
let appItem = NSMenuItem()
appItem.submenu = NSMenu()
appItem.submenu?.addItem(withTitle: "Quit", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
menu.addItem(appItem)
let editItem = NSMenuItem()
let edit = NSMenu(title: "Edit")
edit.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
editItem.submenu = edit
menu.addItem(editItem)
app.mainMenu = menu

let window = NSWindow(contentRect: NSRect(x: 200, y: 200, width: 420, height: 120),
                      styleMask: [.titled], backing: .buffered, defer: false)
window.title = "VANS basics"
let view: NSTextView = quiet ? QuietText(frame: window.contentView!.bounds) : NSTextView(frame: window.contentView!.bounds)
view.string = text
view.isEditable = false
window.contentView?.addSubview(view)

NotificationCenter.default.addObserver(forName: NSApplication.didFinishLaunchingNotification, object: nil, queue: .main) { _ in
    window.makeKeyAndOrderFront(nil)
    app.activate(ignoringOtherApps: true)
    window.makeFirstResponder(view)
    view.setSelectedRange((text as NSString).range(of: word))
    print("ready")
    fflush(stdout)
}
app.run()
