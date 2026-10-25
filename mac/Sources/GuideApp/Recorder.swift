import AVFoundation
import Foundation

@MainActor
final class Recorder {
    private var recorder: AVAudioRecorder?
    private var startedAt: Date?
    private let file = FileManager.default.temporaryDirectory.appendingPathComponent("guide-speech.wav")

    var isRecording: Bool { recorder?.isRecording == true }

    static func askPermission() async -> Bool {
        switch AVCaptureDevice.authorizationStatus(for: .audio) {
        case .authorized: return true
        case .notDetermined: return await AVCaptureDevice.requestAccess(for: .audio)
        default: return false
        }
    }

    func start() throws {
        stopAndDiscard()
        let settings: [String: Any] = [
            AVFormatIDKey: Int(kAudioFormatLinearPCM),
            AVSampleRateKey: 16_000.0,
            AVNumberOfChannelsKey: 1,
            AVLinearPCMBitDepthKey: 16,
            AVLinearPCMIsFloatKey: false,
            AVLinearPCMIsBigEndianKey: false,
        ]
        let next = try AVAudioRecorder(url: file, settings: settings)
        next.isMeteringEnabled = true
        guard next.record() else { throw ServerError(message: "The microphone did not start.") }
        recorder = next
        startedAt = Date()
    }

    func level() -> Double {
        guard let recorder, recorder.isRecording else { return 0 }
        recorder.updateMeters()
        let power = Double(recorder.averagePower(forChannel: 0))
        return max(0, min(1, (power + 50) / 50))
    }

    func finish() -> (data: Data, seconds: Double)? {
        guard let recorder else { return nil }
        let seconds = Date().timeIntervalSince(startedAt ?? Date())
        recorder.stop()
        self.recorder = nil
        guard let data = try? Data(contentsOf: file) else { return nil }
        try? FileManager.default.removeItem(at: file)
        return (data, seconds)
    }

    func stopAndDiscard() {
        recorder?.stop()
        recorder = nil
        try? FileManager.default.removeItem(at: file)
    }
}
