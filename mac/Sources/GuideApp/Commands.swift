import Foundation

enum GuideCommand: String, CaseIterable, Sendable {
    case next, again, back, stop
}

struct CommandWords: Sendable {
    var words: [GuideCommand: [String]]

    init(_ raw: [String: [String]] = [:]) {
        var out: [GuideCommand: [String]] = [:]
        for command in GuideCommand.allCases {
            let list = (raw[command.rawValue] ?? []).map(CommandWords.clean).filter { !$0.isEmpty }
            if !list.isEmpty { out[command] = list }
        }
        words = out
    }

    var isEmpty: Bool { words.isEmpty }

    func first(_ command: GuideCommand) -> String? { words[command]?.first }

    func match(_ text: String) -> GuideCommand? {
        let said = CommandWords.clean(text)
        guard !said.isEmpty else { return nil }
        let order: [GuideCommand] = [.stop, .back, .again, .next]
        for command in order where words[command]?.contains(said) == true {
            return command
        }
        guard said.split(separator: " ").count <= 3 else { return nil }
        let padded = " \(said) "
        var best: (GuideCommand, Int)?
        for command in order {
            for word in words[command] ?? [] where padded.contains(" \(word) ") {
                if best == nil || word.count > best!.1 { best = (command, word.count) }
            }
        }
        return best?.0
    }

    static func clean(_ text: String) -> String {
        let kept = text.lowercased().unicodeScalars.map { scalar -> Character in
            if CharacterSet.punctuationCharacters.contains(scalar) || CharacterSet.symbols.contains(scalar) || scalar == "\u{0964}" || scalar == "\u{0965}" {
                return " "
            }
            return Character(scalar)
        }
        return String(kept).split(whereSeparator: { $0.isWhitespace }).joined(separator: " ")
    }
}
