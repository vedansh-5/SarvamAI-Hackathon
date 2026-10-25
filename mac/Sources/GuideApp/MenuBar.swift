import AppKit
import Carbon.HIToolbox

@MainActor
final class AppController: NSObject, NSApplicationDelegate, NSMenuDelegate {
    private let model = GuideModel()
    private var panel: PanelController?
    private var statusItem: NSStatusItem?
    private var talkKeyProblem: String?

    private enum KeyID {
        static let talk: UInt32 = 1
        static let escape: UInt32 = 2
    }

    func applicationDidFinishLaunching(_ notification: Notification) {
        panel = PanelController(model: model)
        model.onVisible = { [weak self] visible in self?.panelVisible(visible) }
        model.onPhase = { [weak self] phase in self?.updateIcon(phase) }

        let item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        let menu = NSMenu()
        menu.delegate = self
        item.menu = menu
        statusItem = item
        updateIcon(.hidden)

        registerTalkKey()
        Task { await model.refresh() }
    }

    private func registerTalkKey() {
        let press = { [weak self] in self?.model.pressTalk() }
        let release = { [weak self] in self?.model.releaseTalk() }
        let status = Hotkeys.shared.register(id: KeyID.talk, keyCode: TalkKey.code, modifiers: optionKey, onPress: { press() }, onRelease: { release() })
        if status == noErr {
            model.talkKeyLabel = TalkKey.label
            return
        }
        let backup = Hotkeys.shared.register(id: KeyID.talk, keyCode: TalkKey.code, modifiers: optionKey | controlKey, onPress: { press() }, onRelease: { release() })
        if backup == noErr {
            model.talkKeyLabel = TalkKey.backupLabel
            talkKeyProblem = "\(TalkKey.label) was taken (error \(status)), so \(TalkKey.backupLabel) is used."
        } else {
            talkKeyProblem = "No talk key could be set (error \(status))."
        }
    }

    private func panelVisible(_ visible: Bool) {
        panel?.setVisible(visible)
        if visible {
            Hotkeys.shared.register(id: KeyID.escape, keyCode: kVK_Escape, modifiers: 0, onPress: { [weak self] in self?.model.close() })
        } else {
            Hotkeys.shared.unregister(id: KeyID.escape)
        }
    }

    private func updateIcon(_ phase: GuideModel.Phase) {
        let name: String
        switch phase {
        case .listening: name = "mic.fill"
        case .thinking: name = "ellipsis.circle"
        default: name = "waveform"
        }
        let image = NSImage(systemSymbolName: name, accessibilityDescription: model.brand.name)
        image?.isTemplate = true
        statusItem?.button?.image = image
        statusItem?.button?.toolTip = "\(model.brand.name): hold \(model.talkKeyLabel) and talk"
    }

    func menuNeedsUpdate(_ menu: NSMenu) {
        menu.removeAllItems()
        let title = [model.brand.emoji, model.brand.name].compactMap { $0 }.filter { !$0.isEmpty }.joined(separator: " ")
        menu.addItem(info(title, bold: true))
        if let tagline = model.brand.tagline, !tagline.isEmpty { menu.addItem(info(tagline)) }
        menu.addItem(.separator())

        menu.addItem(info("Hold \(model.talkKeyLabel), ask, then let go"))
        if let talkKeyProblem { menu.addItem(info(talkKeyProblem)) }
        menu.addItem(info("Esc stops speaking and closes the panel"))
        menu.addItem(.separator())

        if model.commands.isEmpty {
            menu.addItem(info("Voice commands are off: no word list from the server"))
            menu.addItem(info("The Back, Again, Next and Stop buttons still work"))
        } else {
            menu.addItem(info("During a guide, hold \(model.talkKeyLabel) and say:"))
            for command in GuideCommand.allCases {
                let words = (model.commands.words[command] ?? []).prefix(4).joined(separator: ", ")
                menu.addItem(info("   \(command.rawValue.capitalized): \(words)"))
            }
        }
        menu.addItem(.separator())

        let show = NSMenuItem(title: "Show Last Guide", action: model.hasGuide ? #selector(showGuide) : nil, keyEquivalent: "")
        show.target = self
        menu.addItem(show)
        let stop = NSMenuItem(title: "Stop and Close", action: model.phase == .hidden ? nil : #selector(stopGuide), keyEquivalent: "")
        stop.target = self
        menu.addItem(stop)
        menu.addItem(.separator())

        if let timings = model.lastTimings { menu.addItem(info("Last answer: \(timings.summary)")) }
        let server = model.client.base.absoluteString
        menu.addItem(info(model.serverReachable ? "Server: \(server)" : "Server not reachable: \(server)"))
        let retry = NSMenuItem(title: "Reconnect", action: #selector(reconnect), keyEquivalent: "")
        retry.target = self
        menu.addItem(retry)
        menu.addItem(.separator())

        let quit = NSMenuItem(title: "Quit \(model.brand.name)", action: #selector(quit), keyEquivalent: "q")
        quit.target = self
        menu.addItem(quit)
        Task { await model.refresh() }
    }

    private func info(_ text: String, bold: Bool = false) -> NSMenuItem {
        let item = NSMenuItem(title: text, action: nil, keyEquivalent: "")
        item.isEnabled = false
        if bold {
            item.attributedTitle = NSAttributedString(string: text, attributes: [.font: NSFont.boldSystemFont(ofSize: NSFont.systemFontSize)])
        }
        return item
    }

    @objc private func showGuide() { model.reopen() }
    @objc private func stopGuide() { model.close() }
    @objc private func reconnect() { Task { await model.refresh() } }
    @objc private func quit() {
        model.close()
        NSApp.terminate(nil)
    }
}
