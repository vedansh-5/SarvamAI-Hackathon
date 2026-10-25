import AVFoundation
import Carbon.HIToolbox
import Foundation

@MainActor
enum SelfTest {
    static func run(files: [String]) async -> Int32 {
        let client = ServerClient()
        print("server   \(client.base.absoluteString)")
        let brand = try? await client.brand()
        print("brand    \(brand.map { "\($0.name)" } ?? "not available, using \(GuideModel.fallbackName)")")
        let words = (try? await client.commands()).map(CommandWords.init) ?? CommandWords()
        print("commands \(words.isEmpty ? "off (no word list)" : GuideCommand.allCases.map { "\($0.rawValue)=\(words.words[$0]?.count ?? 0)" }.joined(separator: " "))")
        var failed = false
        for path in files {
            print("")
            if await !one(path, client: client, words: words) { failed = true }
        }
        return failed ? 1 : 0
    }

    private static func one(_ path: String, client: ServerClient, words: CommandWords) async -> Bool {
        let url = URL(fileURLWithPath: path)
        guard let wav = try? Data(contentsOf: url) else {
            print("file     \(path): cannot read")
            return false
        }
        let seconds = (try? AVAudioFile(forReading: url)).map { Double($0.length) / $0.fileFormat.sampleRate } ?? 0
        print(String(format: "file     %@ (%d KB, %.1f s of speech)", url.lastPathComponent, wav.count / 1024, seconds))
        let start = Date()
        do {
            let heard = try await client.listen(wav: wav)
            let listenTime = Date().timeIntervalSince(start)
            print(String(format: "listen   %.2f s  \"%@\"  language %@", listenTime, heard.text, heard.language))
            if let command = words.match(heard.text) {
                print("command  \(command.rawValue) (during a guide this phrase moves the guide, no ask)")
                return true
            }
            print("command  none, so it is a question")
            let askStart = Date()
            let answer = try await client.ask(text: heard.text, language: heard.language)
            let askTime = Date().timeIntervalSince(askStart)
            let language = answer.language ?? heard.language
            print(String(format: "ask      %.2f s  language %@", askTime, language))
            print("say      \(answer.say)")
            for (i, step) in answer.steps.enumerated() { print("step \(i + 1)   \(step)") }
            let speakStart = Date()
            async let sayAudio = client.speak(text: answer.say, language: language)
            async let stepAudio = optionalSpeak(client, answer.steps.first, language)
            let first = try await sayAudio
            let firstTime = Date().timeIntervalSince(speakStart)
            let total = Date().timeIntervalSince(start)
            let second = try await stepAudio
            let secondTime = Date().timeIntervalSince(speakStart)
            let firstLength = (try? AVAudioPlayer(data: first)).map { $0.duration } ?? 0
            print(String(format: "speak    %.2f s  say audio %d KB (%.1f s long)", firstTime, first.count / 1024, firstLength))
            if let second {
                print(String(format: "speak    %.2f s  step 1 audio %d KB, fetched alongside", secondTime, second.count / 1024))
            }
            print(String(format: "total    %.2f s  from end of recording to first sound", total))
            return true
        } catch {
            print("failed   \(error.localizedDescription)")
            return false
        }
    }

    nonisolated private static func optionalSpeak(_ client: ServerClient, _ text: String?, _ language: String) async throws -> Data? {
        guard let text else { return nil }
        return try await client.speak(text: text, language: language)
    }

    static func match(_ phrases: [String]) async -> Int32 {
        guard let raw = try? await ServerClient().commands() else {
            print("commands off: /api/commands not reachable")
            return 1
        }
        let words = CommandWords(raw)
        for phrase in phrases {
            print("\"\(phrase)\" -> \(words.match(phrase)?.rawValue ?? "none")")
        }
        return 0
    }

    static func hotkeyCheck() -> Int32 {
        let status = Hotkeys.shared.register(id: 9, keyCode: TalkKey.code, modifiers: optionKey, onPress: {})
        Hotkeys.shared.unregister(id: 9)
        print(status == noErr ? "\(TalkKey.label) hotkey: registers fine" : "\(TalkKey.label) hotkey: failed with \(status)")
        return status == noErr ? 0 : 1
    }
}
