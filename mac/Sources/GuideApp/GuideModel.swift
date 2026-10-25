import AppKit
import Foundation
import Observation

struct Timings: Sendable {
    var listen: Double
    var ask: Double
    var speak: Double
    var firstSound: Double

    var summary: String {
        String(format: "heard %.1fs, thought %.1fs, voice %.1fs, first sound %.1fs", listen, ask, speak, firstSound)
    }
}

@MainActor
@Observable
final class GuideModel {
    enum Phase { case hidden, listening, thinking, guiding, note }

    var phase: Phase = .hidden
    var heard = ""
    var say = ""
    var steps: [String] = []
    var index = 0
    var speaking = false
    var level = 0.0
    var note = ""
    var brand = Brand(name: GuideModel.fallbackName)
    var serverReachable = true
    var commands = CommandWords()
    var lastTimings: Timings?
    var talkKeyLabel = "⌥K"

    @ObservationIgnored let client = ServerClient()
    @ObservationIgnored var onVisible: (Bool) -> Void = { _ in }
    @ObservationIgnored var onPhase: (Phase) -> Void = { _ in }
    @ObservationIgnored private let recorder = Recorder()
    @ObservationIgnored private let player = Player()
    @ObservationIgnored private var language = "en-IN"
    @ObservationIgnored private var turns: [Turn] = []
    @ObservationIgnored private var round = 0
    @ObservationIgnored private var holding = false
    @ObservationIgnored private var guideOpen = false
    @ObservationIgnored private var sayAudio: Task<Data?, Never>?
    @ObservationIgnored private var stepAudio: [Int: Task<Data?, Never>] = [:]
    @ObservationIgnored private var meter: Timer?
    @ObservationIgnored private var visible = false

    static var fallbackName: String {
        let info = Bundle.main.infoDictionary
        let name = (info?["CFBundleDisplayName"] as? String) ?? (info?["CFBundleName"] as? String)
        return name ?? ProcessInfo.processInfo.processName
    }

    var hasGuide: Bool { !say.isEmpty || !steps.isEmpty }

    func refresh() async {
        async let brandReply = try? client.brand()
        async let wordsReply = try? client.commands()
        let (b, w) = await (brandReply, wordsReply)
        if let b, !b.name.isEmpty { brand = b }
        if let w { commands = CommandWords(w) }
        serverReachable = b != nil || w != nil
    }

    func pressTalk() {
        holding = true
        guideOpen = hasGuide && (phase == .guiding || phase == .thinking)
        round += 1
        let mine = round
        player.stop()
        speaking = false
        Task {
            let allowed = await Recorder.askPermission()
            guard mine == round, holding else { return }
            guard allowed else {
                show(note: "Microphone is off. Turn it on in System Settings, Privacy & Security, Microphone.")
                return
            }
            do {
                try recorder.start()
            } catch {
                show(note: "The microphone did not start.")
                return
            }
            heard = ""
            setPhase(.listening)
            startMeter()
        }
        if commands.isEmpty || !serverReachable {
            Task { await refresh() }
        }
    }

    func releaseTalk() {
        holding = false
        guard recorder.isRecording, let take = recorder.finish() else { return }
        let inGuide = guideOpen
        if take.seconds < 0.35 {
            show(note: "Hold \(talkKeyLabel) while you talk, then let go.")
            return
        }
        round += 1
        let mine = round
        setPhase(.thinking)
        Task { await handle(take.data, inGuide: inGuide, round: mine) }
    }

    private func handle(_ wav: Data, inGuide: Bool, round mine: Int) async {
        let start = Date()
        let words: Heard
        do {
            words = try await client.listen(wav: wav)
        } catch {
            if mine == round { show(note: error.localizedDescription) }
            return
        }
        let listenTime = Date().timeIntervalSince(start)
        guard mine == round else { return }
        let text = words.text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !text.isEmpty else {
            show(note: "I didn't catch that. Hold \(talkKeyLabel) and try again.")
            return
        }
        heard = text
        if inGuide, let command = commands.match(text) {
            perform(command)
            return
        }
        if !inGuide { turns = [] }
        let askStart = Date()
        let answer: Answer
        do {
            answer = try await client.ask(text: text, language: words.language, turns: Array(turns.suffix(4)))
        } catch {
            if mine == round { show(note: error.localizedDescription) }
            return
        }
        let askTime = Date().timeIntervalSince(askStart)
        guard mine == round else { return }
        language = answer.language ?? words.language
        say = answer.say
        steps = answer.steps
        index = 0
        turns.append(Turn(question: text, say: answer.say, steps: answer.steps))
        stepAudio = [:]
        let speakStart = Date()
        sayAudio = voice(answer.say)
        if !steps.isEmpty { stepAudio[0] = voice(steps[0]) }
        setPhase(.guiding)
        speaking = true
        let first = await sayAudio?.value
        guard mine == round else { return }
        let now = Date()
        lastTimings = Timings(listen: listenTime, ask: askTime, speak: now.timeIntervalSince(speakStart), firstSound: now.timeIntervalSince(start))
        if let first { _ = await player.play(first) }
        guard mine == round else { return }
        if steps.isEmpty {
            speaking = false
        } else {
            await speakStep(0, round: mine)
        }
    }

    private func voice(_ text: String) -> Task<Data?, Never>? {
        let line = text.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !line.isEmpty else { return nil }
        let client = client
        let language = language
        return Task { try? await client.speak(text: line, language: language) }
    }

    private func audio(for step: Int) -> Task<Data?, Never>? {
        if let cached = stepAudio[step] { return cached }
        guard steps.indices.contains(step) else { return nil }
        let task = voice(steps[step])
        stepAudio[step] = task
        return task
    }

    private func speakStep(_ step: Int, round mine: Int) async {
        guard steps.indices.contains(step) else { return }
        speaking = true
        let data = await audio(for: step)?.value
        if steps.indices.contains(step + 1) { _ = audio(for: step + 1) }
        guard mine == round, index == step else { return }
        if let data { _ = await player.play(data) }
        if mine == round, index == step { speaking = false }
    }

    func perform(_ command: GuideCommand) {
        guard hasGuide else {
            if command == .stop { close() }
            return
        }
        round += 1
        let mine = round
        player.stop()
        if phase != .guiding { setPhase(.guiding) }
        switch command {
        case .stop:
            close()
            return
        case .next:
            guard index + 1 < steps.count else {
                speaking = false
                return
            }
            index += 1
        case .back:
            guard index > 0 else {
                speaking = false
                return
            }
            index -= 1
        case .again:
            break
        }
        if steps.isEmpty {
            speaking = true
            Task {
                if let data = await sayAudio?.value, mine == round { _ = await player.play(data) }
                if mine == round { speaking = false }
            }
        } else {
            let step = index
            Task { await speakStep(step, round: mine) }
        }
    }

    func reopen() {
        guard hasGuide else { return }
        setPhase(.guiding)
    }

    func close() {
        round += 1
        holding = false
        player.stop()
        recorder.stopAndDiscard()
        speaking = false
        setPhase(.hidden)
    }

    private func show(note text: String) {
        recorder.stopAndDiscard()
        note = text
        setPhase(.note)
        round += 1
        let mine = round
        Task {
            try? await Task.sleep(for: .seconds(3.5))
            guard mine == round, phase == .note else { return }
            if hasGuide { setPhase(.guiding) } else { setPhase(.hidden) }
        }
    }

    private func setPhase(_ next: Phase) {
        phase = next
        if next != .listening { stopMeterIfQuiet() }
        let shouldShow = next != .hidden
        if shouldShow != visible {
            visible = shouldShow
            onVisible(shouldShow)
        }
        onPhase(next)
        if shouldShow { startMeter() }
    }

    private func startMeter() {
        guard meter == nil else { return }
        let timer = Timer(timeInterval: 1.0 / 24.0, repeats: true) { _ in
            MainActor.assumeIsolated { [weak self] in self?.tick() }
        }
        RunLoop.main.add(timer, forMode: .common)
        meter = timer
    }

    private func tick() {
        let raw = recorder.isRecording ? recorder.level() : player.level()
        level = level * 0.55 + raw * 0.45
        if phase == .hidden { stopMeterIfQuiet() }
    }

    private func stopMeterIfQuiet() {
        guard phase == .hidden else { return }
        meter?.invalidate()
        meter = nil
        level = 0
    }
}
