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

struct Answer: Decodable, Sendable {
    var id: String?
    var question: String?
    var language: String?
    var say: String
    var steps: [String]
}

struct Turn: Codable, Sendable {
    var question: String
    var say: String
    var steps: [String]
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

    func ask(text: String, language: String, turns: [Turn] = []) async throws -> Answer {
        struct Body: Encodable { var text: String; var language: String; var turns: [Turn] }
        return try await post("api/ask", Body(text: text, language: language, turns: turns))
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

    private func post<T: Decodable, B: Encodable>(_ path: String, _ body: B) async throws -> T {
        var request = URLRequest(url: base.appendingPathComponent(path))
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
