import AppKit

let arguments = Array(CommandLine.arguments.dropFirst())

func finish(_ code: Int32) -> Never {
    fflush(stdout)
    exit(code)
}

if arguments.first == "--selftest" {
    Task { finish(await SelfTest.run(files: Array(arguments.dropFirst()))) }
    dispatchMain()
} else if arguments.first == "--match" {
    Task { finish(await SelfTest.match(Array(arguments.dropFirst()))) }
    dispatchMain()
} else if arguments.first == "--snapshot", arguments.count > 1 {
    finish(Snapshot.run(into: arguments[1]))
} else if arguments.first == "--hotkey-check" {
    finish(SelfTest.hotkeyCheck())
} else {
    let app = NSApplication.shared
    let controller = AppController()
    app.delegate = controller
    app.setActivationPolicy(.accessory)
    app.run()
}
