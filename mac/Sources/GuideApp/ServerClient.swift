import Foundation

struct Brand: Decodable, Sendable {
    var name: String
    var tagline: String?
    var emoji: String?
}

struct Heard: Decodable, Sendable {
    var text: String
    var language: String
}

struct Step: Codable, Sendable, Equatable {
    var text: String
    var target: String?
    var app: String?

    init(text: String, target: String? = nil, app: String? = nil) {
        self.text = text
        self.target = Step.tidy(target)
        self.app = Step.tidy(app)
    }

    private enum Keys: String, CodingKey { case text, target, app }

    init(from decoder: Decoder) throws {
        if let single = try? decoder.singleValueContainer(), let line = try? single.decode(String.self) {
            self.init(text: line)
            return
        }
        let box = try decoder.container(keyedBy: Keys.self)
        let text = (try? box.decodeIfPresent(String.self, forKey: .text)) ?? nil
        let target = (try? box.decodeIfPresent(String.self, forKey: .target)) ?? nil
        let app = (try? box.decodeIfPresent(String.self, forKey: .app)) ?? nil
        self.init(text: text ?? "", target: target, app: app)
    }

    func encode(to encoder: Encoder) throws {
        var box = encoder.container(keyedBy: Keys.self)
        try box.encode(text, forKey: .text)
        try box.encode(target, forKey: .target)
        try box.encode(app, forKey: .app)
    }

    private static func tidy(_ value: String?) -> String? {
        let line = value?.trimmingCharacters(in: .whitespacesAndNewlines) ?? ""
        return line.isEmpty || line.lowercased() == "null" ? nil : line
    }
}

struct Answer: Decodable, Sendable {
    var id: String?
    var question: String?
    var language: String?
    var say: String
    var steps: [Step]
    var by: String?

    private enum Keys: String, CodingKey {
        case id, question, language, say, steps, by
        case stepsText = "steps_text"
    }

    init(say: String, steps: [Step], language: String? = nil, by: String? = nil) {
        self.say = say
        self.steps = steps
        self.language = language
        self.by = by
    }

    init(from decoder: Decoder) throws {
        let box = try decoder.container(keyedBy: Keys.self)
        id = try? box.decodeIfPresent(String.self, forKey: .id)
        question = try? box.decodeIfPresent(String.self, forKey: .question)
        language = try? box.decodeIfPresent(String.self, forKey: .language)
        say = ((try? box.decodeIfPresent(String.self, forKey: .say)) ?? nil) ?? ""
        by = try? box.decodeIfPresent(String.self, forKey: .by)
        let full = ((try? box.decodeIfPresent([Step].self, forKey: .steps)) ?? nil) ?? []
        let plain = ((try? box.decodeIfPresent([String].self, forKey: .stepsText)) ?? nil) ?? []
        let chosen = full.isEmpty ? plain.map { Step(text: $0) } : full
        steps = chosen.filter { !$0.text.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty }
    }
}

struct Turn: Codable, Sendable {
    var question: String
    var say: String
    var steps: [Step]
}

struct Screen: Encodable, Sendable {
    var app: String?
    var menus: [String]
    var image: String?

    private enum Keys: String, CodingKey { case app, menus, image }

    func encode(to encoder: Encoder) throws {
        var box = encoder.container(keyedBy: Keys.self)
        try box.encode(app, forKey: .app)
        try box.encode(menus, forKey: .menus)
        try box.encode(image, forKey: .image)
    }
}

struct ServerError: Error, LocalizedError, Sendable {
    var message: String
    var errorDescription: String? { message }
}

private struct ServerFailure: Decodable {
    var error: String?
}

struct ServerClient: Sendable {
    let base: URL
    let session: URLSession

    init(base: URL? = nil) {
        let env = ProcessInfo.processInfo.environment["APP_SERVER"] ?? ""
        let fallback = URL(string: "http://127.0.0.1:5050")!
        self.base = base ?? URL(string: env.trimmingCharacters(in: .whitespaces)).flatMap { $0.scheme == nil ? nil : $0 } ?? fallback
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 40
        config.waitsForConnectivity = false
        self.session = URLSession(configuration: config)
    }

    func brand() async throws -> Brand {
        try await get("api/brand", timeout: 4)
    }

    func commands() async throws -> [String: [String]] {
        try await get("api/commands", timeout: 4)
    }

    func listen(wav: Data, filename: String = "speech.wav") async throws -> Heard {
        let boundary = "b-\(UUID().uuidString)"
        var body = Data()
        body.append(Data("--\(boundary)\r\n".utf8))
        body.append(Data("Content-Disposition: form-data; name=\"audio\"; filename=\"\(filename)\"\r\n".utf8))
        body.append(Data("Content-Type: audio/wav\r\n\r\n".utf8))
        body.append(wav)
        body.append(Data("\r\n--\(boundary)--\r\n".utf8))
        var request = URLRequest(url: base.appendingPathComponent("api/listen"))
        request.httpMethod = "POST"
        request.setValue("multipart/form-data; boundary=\(boundary)", forHTTPHeaderField: "Content-Type")
        request.httpBody = body
        return try await send(request)
    }

    func ask(text: String, language: String, turns: [Turn] = [], screen: Screen? = nil, done: [Step] = []) async throws -> Answer {
        struct Body: Encodable {
            var text: String
            var language: String
            var turns: [Turn]
            var screen: Screen?
            var done: [Step]?
        }
        let body = Body(text: text, language: language, turns: turns, screen: screen, done: done.isEmpty ? nil : done)
        return try await post("api/ask", body, timeout: screen?.image == nil ? 40 : 60)
    }

    func speak(text: String, language: String) async throws -> Data {
        struct Body: Encodable { var text: String; var language: String }
        struct Reply: Decodable { var audio: String }
        let reply: Reply = try await post("api/speak", Body(text: text, language: language))
        guard let data = Data(base64Encoded: reply.audio), !data.isEmpty else {
            throw ServerError(message: "The voice came back empty.")
        }
        return data
    }

    private func get<T: Decodable>(_ path: String, timeout: TimeInterval) async throws -> T {
        var request = URLRequest(url: base.appendingPathComponent(path))
        request.timeoutInterval = timeout
        return try await send(request)
    }

    private func post<T: Decodable, B: Encodable>(_ path: String, _ body: B, timeout: TimeInterval = 40) async throws -> T {
        var request = URLRequest(url: base.appendingPathComponent(path))
        request.timeoutInterval = timeout
        request.httpMethod = "POST"
        request.setValue("application/json", forHTTPHeaderField: "Content-Type")
        request.httpBody = try JSONEncoder().encode(body)
        return try await send(request)
    }

    private func send<T: Decodable>(_ request: URLRequest) async throws -> T {
        let data: Data
        let response: URLResponse
        do {
            (data, response) = try await session.data(for: request)
        } catch {
            throw ServerError(message: "Can't reach the server at \(base.absoluteString).")
        }
        let status = (response as? HTTPURLResponse)?.statusCode ?? 0
        guard (200..<300).contains(status) else {
            let text = (try? JSONDecoder().decode(ServerFailure.self, from: data))?.error
            throw ServerError(message: text ?? "The server said \(status).")
        }
        do {
            return try JSONDecoder().decode(T.self, from: data)
        } catch {
            throw ServerError(message: "The server sent something unexpected.")
        }
    }
}
