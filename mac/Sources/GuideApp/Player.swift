import AVFoundation
import Foundation

@MainActor
final class Player: NSObject, AVAudioPlayerDelegate {
    private var player: AVAudioPlayer?
    private var waiting: CheckedContinuation<Bool, Never>?

    var isPlaying: Bool { player?.isPlaying == true }

    func play(_ data: Data) async -> Bool {
        stop()
        guard let next = try? AVAudioPlayer(data: data) else { return false }
        next.delegate = self
        next.isMeteringEnabled = true
        player = next
        guard next.play() else {
            player = nil
            return false
        }
        return await withCheckedContinuation { waiting = $0 }
    }

    func level() -> Double {
        guard let player, player.isPlaying else { return 0 }
        player.updateMeters()
        return max(0, min(1, (Double(player.averagePower(forChannel: 0)) + 40) / 40))
    }

    func stop() {
        player?.stop()
        player = nil
        finish(false)
    }

    private func finish(_ completed: Bool) {
        let cont = waiting
        waiting = nil
        cont?.resume(returning: completed)
    }

    nonisolated func audioPlayerDidFinishPlaying(_ player: AVAudioPlayer, successfully flag: Bool) {
        let id = ObjectIdentifier(player)
        Task { @MainActor in
            guard let current = self.player, ObjectIdentifier(current) == id else { return }
            self.player = nil
            self.finish(flag)
        }
    }

    nonisolated func audioPlayerDecodeErrorDidOccur(_ player: AVAudioPlayer, error: Error?) {
        let id = ObjectIdentifier(player)
        Task { @MainActor in
            guard let current = self.player, ObjectIdentifier(current) == id else { return }
            self.player = nil
            self.finish(false)
        }
    }
}
