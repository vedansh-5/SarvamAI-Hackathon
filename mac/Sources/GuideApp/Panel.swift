import AppKit
import SwiftUI

final class FloatingPanel: NSPanel {
    override var canBecomeKey: Bool { true }
    override var canBecomeMain: Bool { false }
}

final class ClickThroughHostingView<Content: View>: NSHostingView<Content> {
    override func acceptsFirstMouse(for event: NSEvent?) -> Bool { true }
}

@MainActor
final class PanelController {
    static let size = NSSize(width: 500, height: 460)

    private let panel: FloatingPanel
    private let model: GuideModel
    private var hideWork: DispatchWorkItem?

    init(model: GuideModel) {
        self.model = model
        panel = FloatingPanel(
            contentRect: NSRect(origin: .zero, size: Self.size),
            styleMask: [.borderless, .nonactivatingPanel],
            backing: .buffered,
            defer: true
        )
        panel.isFloatingPanel = true
        panel.level = .statusBar
        panel.collectionBehavior = [.canJoinAllSpaces, .fullScreenAuxiliary, .stationary, .ignoresCycle]
        panel.backgroundColor = .clear
        panel.isOpaque = false
        panel.hasShadow = false
        panel.hidesOnDeactivate = false
        panel.becomesKeyOnlyIfNeeded = true
        panel.isMovable = false
        panel.isReleasedWhenClosed = false
        let host = ClickThroughHostingView(rootView: GuideView(model: model))
        host.frame = NSRect(origin: .zero, size: Self.size)
        panel.contentView = host
    }

    func setVisible(_ visible: Bool) {
        hideWork?.cancel()
        if visible {
            place()
            panel.orderFrontRegardless()
        } else {
            let work = DispatchWorkItem { [weak self] in
                MainActor.assumeIsolated { self?.panel.orderOut(nil) }
            }
            hideWork = work
            DispatchQueue.main.asyncAfter(deadline: .now() + 0.3, execute: work)
        }
    }

    private func place() {
        let mouse = NSEvent.mouseLocation
        let screen = NSScreen.screens.first { NSMouseInRect(mouse, $0.frame, false) } ?? NSScreen.main
        guard let area = screen?.visibleFrame else { return }
        let origin = NSPoint(x: area.midX - Self.size.width / 2, y: area.maxY - Self.size.height - 6)
        panel.setFrame(NSRect(origin: origin, size: Self.size), display: true)
    }
}

private let accent = Color(red: 0.55, green: 0.80, blue: 0.24)
private let ink = Color(red: 0.96, green: 0.96, blue: 0.97)
private let red = Color(red: 1.0, green: 0.27, blue: 0.23)
private let amber = Color(red: 1.0, green: 0.70, blue: 0.25)

struct GuideView: View {
    let model: GuideModel
    @Environment(\.accessibilityReduceMotion) private var reduceMotion

    var body: some View {
        VStack(spacing: 0) {
            island
                .scaleEffect(model.phase == .hidden ? 0.6 : 1, anchor: .top)
                .opacity(model.phase == .hidden ? 0 : 1)
                .animation(reduceMotion ? nil : .spring(response: 0.42, dampingFraction: 0.72), value: model.phase)
            Spacer(minLength: 0)
        }
        .frame(width: PanelController.size.width, height: PanelController.size.height, alignment: .top)
    }

    private var island: some View {
        VStack(alignment: .leading, spacing: 12) {
            header
            if model.phase == .guiding || (model.phase == .thinking && model.hasGuide) {
                if !model.steps.isEmpty { stepList }
                controls
            }
        }
        .padding(.horizontal, 18)
        .padding(.vertical, 12)
        .frame(width: islandWidth, alignment: .leading)
        .background(
            RoundedRectangle(cornerRadius: 22, style: .continuous)
                .fill(Color.black)
                .shadow(color: .black.opacity(0.35), radius: 14, y: 6)
                .shadow(color: accent.opacity(glow), radius: 4 + 22 * model.level)
        )
        .foregroundStyle(ink)
        .padding(.top, 4)
        .animation(reduceMotion ? nil : .spring(response: 0.38, dampingFraction: 0.8), value: model.steps)
        .animation(reduceMotion ? nil : .easeOut(duration: 0.18), value: model.index)
    }

    private var islandWidth: CGFloat {
        switch model.phase {
        case .guiding: 460
        case .thinking where model.hasGuide: 460
        case .note: 420
        default: 320
        }
    }

    private var glow: Double {
        model.phase == .listening || model.speaking ? 0.5 * model.level : 0
    }

    private var header: some View {
        HStack(spacing: 12) {
            indicator
            Text(headline)
                .font(.system(size: 14, weight: .medium))
                .foregroundStyle(headlineColor)
                .lineLimit(model.phase == .guiding ? 3 : 2)
                .fixedSize(horizontal: false, vertical: true)
                .frame(maxWidth: .infinity, alignment: .leading)
            Button(action: { model.close() }) {
                Image(systemName: "xmark")
                    .font(.system(size: 10, weight: .bold))
                    .frame(width: 20, height: 20)
                    .background(Circle().fill(Color.white.opacity(0.12)))
            }
            .buttonStyle(.plain)
            .help("Close (Esc)")
        }
        .frame(minHeight: 24)
    }

    private var headline: String {
        switch model.phase {
        case .hidden: ""
        case .listening: model.heard.isEmpty ? "Listening…" : model.heard
        case .thinking: model.heard.isEmpty ? "Working it out…" : "“\(model.heard)”"
        case .guiding: model.say.isEmpty ? model.heard : model.say
        case .note: model.note
        }
    }

    private var headlineColor: Color {
        switch model.phase {
        case .listening where model.heard.isEmpty: ink.opacity(0.55)
        case .thinking: ink.opacity(0.7)
        case .note: amber
        default: ink
        }
    }

    @ViewBuilder
    private var indicator: some View {
        switch model.phase {
        case .listening:
            HStack(spacing: 8) {
                LiveDot()
                Wave(level: model.level, moving: true)
            }
        case .thinking:
            HopDots()
        case .guiding where model.speaking:
            Wave(level: max(model.level, 0.35), moving: true)
        case .note:
            Image(systemName: "exclamationmark.circle.fill").foregroundStyle(amber)
        default:
            Text(model.brand.emoji ?? "").font(.system(size: 15))
        }
    }

    private var stepList: some View {
        VStack(alignment: .leading, spacing: 6) {
            ForEach(Array(model.steps.enumerated()), id: \.offset) { item in
                let current = item.offset == model.index
                HStack(alignment: .firstTextBaseline, spacing: 10) {
                    Text("\(item.offset + 1)")
                        .font(.system(size: 11, weight: .bold, design: .rounded))
                        .foregroundStyle(current ? Color.black : ink.opacity(0.6))
                        .frame(width: 18, height: 18)
                        .background(Circle().fill(current ? accent : Color.white.opacity(0.1)))
                    Text(item.element)
                        .font(.system(size: 13, weight: current ? .semibold : .regular))
                        .foregroundStyle(current ? ink : ink.opacity(item.offset < model.index ? 0.4 : 0.6))
                        .fixedSize(horizontal: false, vertical: true)
                }
            }
        }
    }

    private var controls: some View {
        VStack(alignment: .leading, spacing: 6) {
            HStack(spacing: 8) {
                if !model.steps.isEmpty {
                    Text("Step \(model.index + 1) of \(model.steps.count)")
                        .font(.system(size: 11, weight: .medium))
                        .foregroundStyle(ink.opacity(0.5))
                }
                Spacer()
                ControlButton(symbol: "chevron.left", label: "Back", enabled: model.index > 0) { model.perform(.back) }
                ControlButton(symbol: "arrow.clockwise", label: "Again", enabled: true) { model.perform(.again) }
                ControlButton(symbol: "chevron.right", label: "Next", enabled: model.index + 1 < model.steps.count) { model.perform(.next) }
                ControlButton(symbol: "stop.fill", label: "Stop", enabled: true) { model.perform(.stop) }
            }
            Text(hint)
                .font(.system(size: 11))
                .foregroundStyle(ink.opacity(0.45))
                .fixedSize(horizontal: false, vertical: true)
        }
    }

    private var hint: String {
        let names = [GuideCommand.next, .again, .back, .stop].compactMap { model.commands.first($0) }
        guard names.count == 4 else { return "Hold \(model.talkKeyLabel) to ask something else. Esc closes." }
        let list = names.map { "“\($0)”" }
        return "Hold \(model.talkKeyLabel) and say \(list[0]), \(list[1]), \(list[2]) or \(list[3]). Esc closes."
    }
}

private struct ControlButton: View {
    let symbol: String
    let label: String
    let enabled: Bool
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            Image(systemName: symbol)
                .font(.system(size: 11, weight: .bold))
                .frame(width: 30, height: 24)
                .background(Capsule().fill(Color.white.opacity(enabled ? 0.14 : 0.05)))
                .foregroundStyle(ink.opacity(enabled ? 1 : 0.35))
        }
        .buttonStyle(.plain)
        .disabled(!enabled)
        .help(label)
        .accessibilityLabel(label)
    }
}

private struct LiveDot: View {
    var body: some View {
        TimelineView(.animation) { context in
            let pulse = 0.5 + 0.5 * cos(context.date.timeIntervalSinceReferenceDate * 2 * .pi / 1.3)
            Circle()
                .fill(red)
                .frame(width: 8, height: 8)
                .opacity(0.4 + 0.6 * pulse)
                .scaleEffect(0.8 + 0.2 * pulse)
        }
    }
}

private struct HopDots: View {
    var body: some View {
        TimelineView(.animation) { context in
            let t = context.date.timeIntervalSinceReferenceDate
            HStack(spacing: 4) {
                ForEach(0..<3, id: \.self) { i in
                    let phase = (t / 0.95 - Double(i) * 0.15).truncatingRemainder(dividingBy: 1)
                    let lift = phase < 0.45 ? sin(phase / 0.45 * .pi) : 0
                    Circle()
                        .fill(accent)
                        .frame(width: 5, height: 5)
                        .offset(y: -4 * lift)
                        .opacity(1 - 0.45 * lift)
                }
            }
            .frame(height: 18)
        }
    }
}

private struct Wave: View {
    let level: Double
    let moving: Bool

    var body: some View {
        TimelineView(.animation(paused: !moving)) { context in
            let t = context.date.timeIntervalSinceReferenceDate
            HStack(spacing: 2.5) {
                ForEach(0..<5, id: \.self) { i in
                    let swing = 0.5 + 0.5 * sin(t * 7.5 + Double(i) * 1.3)
                    let height = 0.16 + 0.84 * min(1, level * (0.55 + 0.45 * swing))
                    Capsule()
                        .fill(accent)
                        .frame(width: 2.5, height: 18 * height)
                }
            }
            .frame(height: 18)
        }
    }
}
