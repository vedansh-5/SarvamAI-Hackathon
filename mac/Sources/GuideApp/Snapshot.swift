import AppKit
import SwiftUI

@MainActor
enum Snapshot {
    static func run(into folder: String) -> Int32 {
        let model = GuideModel()
        model.commands = CommandWords(["next": ["next"], "again": ["again"], "back": ["back"], "stop": ["stop"]])
        let shots: [(String, (GuideModel) -> Void)] = [
            ("listening", { $0.phase = .listening; $0.level = 0.6 }),
            ("thinking", { $0.phase = .thinking; $0.heard = "How do I take a screenshot?" }),
            ("guiding", {
                $0.phase = .guiding
                $0.say = "Here is how to take a screenshot."
                $0.steps = ["Press Command, Shift and 3 together", "Find the picture on your desktop", "Double-click it to open"]
                $0.index = 1
                $0.speaking = true
                $0.level = 0.5
            }),
            ("note", { $0.phase = .note; $0.note = "I didn't catch that. Hold the talk key and try again." }),
        ]
        var failed = false
        for (name, setup) in shots {
            setup(model)
            let renderer = ImageRenderer(content: GuideView(model: model).background(Color(white: 0.85)))
            renderer.scale = 2
            guard let image = renderer.cgImage,
                  let png = NSBitmapImageRep(cgImage: image).representation(using: .png, properties: [:]) else {
                failed = true
                continue
            }
            let url = URL(fileURLWithPath: folder).appendingPathComponent("panel-\(name).png")
            do { try png.write(to: url) } catch { failed = true; continue }
            print("wrote \(url.path)")
        }
        return failed ? 1 : 0
    }
}
