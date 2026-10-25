import Carbon.HIToolbox
import Foundation

@MainActor
final class Hotkeys {
    static let shared = Hotkeys()

    struct Key {
        var ref: EventHotKeyRef
        var onPress: () -> Void
        var onRelease: () -> Void
    }

    private var keys: [UInt32: Key] = [:]
    private var handler: EventHandlerRef?
    private let signature: OSType = 0x4744_4150

    @discardableResult
    func register(id: UInt32, keyCode: Int, modifiers: Int, onPress: @escaping () -> Void, onRelease: @escaping () -> Void = {}) -> OSStatus {
        installHandler()
        unregister(id: id)
        var ref: EventHotKeyRef?
        let hotKeyID = EventHotKeyID(signature: signature, id: id)
        let status = RegisterEventHotKey(UInt32(keyCode), UInt32(modifiers), hotKeyID, GetApplicationEventTarget(), 0, &ref)
        if status == noErr, let ref {
            keys[id] = Key(ref: ref, onPress: onPress, onRelease: onRelease)
        }
        return status
    }

    func unregister(id: UInt32) {
        guard let key = keys.removeValue(forKey: id) else { return }
        UnregisterEventHotKey(key.ref)
    }

    func isRegistered(id: UInt32) -> Bool { keys[id] != nil }

    fileprivate func handle(id: UInt32, kind: UInt32) {
        guard let key = keys[id] else { return }
        if kind == UInt32(kEventHotKeyPressed) { key.onPress() } else { key.onRelease() }
    }

    private func installHandler() {
        guard handler == nil else { return }
        var specs = [
            EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyPressed)),
            EventTypeSpec(eventClass: OSType(kEventClassKeyboard), eventKind: UInt32(kEventHotKeyReleased)),
        ]
        InstallEventHandler(GetApplicationEventTarget(), hotkeyCallback, specs.count, &specs, nil, &handler)
    }
}

private func hotkeyCallback(_ next: EventHandlerCallRef?, _ event: EventRef?, _ data: UnsafeMutableRawPointer?) -> OSStatus {
    guard let event else { return OSStatus(eventNotHandledErr) }
    var hotKeyID = EventHotKeyID()
    let status = GetEventParameter(event, EventParamName(kEventParamDirectObject), EventParamType(typeEventHotKeyID), nil, MemoryLayout<EventHotKeyID>.size, nil, &hotKeyID)
    guard status == noErr else { return status }
    let kind = GetEventKind(event)
    let id = hotKeyID.id
    MainActor.assumeIsolated {
        Hotkeys.shared.handle(id: id, kind: kind)
    }
    return noErr
}
