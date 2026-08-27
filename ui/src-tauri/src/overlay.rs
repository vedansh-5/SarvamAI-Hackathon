//! The overlay: a panel summoned by a hotkey, over whatever you are doing.
//!
//! VANS is not a place you go. It is something you call while working in
//! another application, so the window has no dock icon, no taskbar entry, and
//! no chrome — and it starts hidden, because an app that seizes the screen on
//! launch is the thing this replaces.

use tauri::{AppHandle, Emitter, Manager, WebviewWindow};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};

/// The summoning shortcut.
///
/// Option-Space rather than Command-Space, which is Spotlight's and cannot be
/// taken without breaking something the user relies on.
pub fn hotkey() -> Shortcut {
    Shortcut::new(Some(Modifiers::ALT), Code::Space)
}

pub fn window(app: &AppHandle) -> Option<WebviewWindow> {
    app.get_webview_window("main")
}

/// Brings the panel up and gives it the keyboard.
///
/// The activation policy is Accessory, so the process is not a foreground app
/// and focus does not automatically follow the window. Asking for it
/// explicitly is what lets the user start typing immediately instead of
/// clicking the panel first.
pub fn show(app: &AppHandle) {
    let Some(win) = window(app) else { return };

    // Capture context before VANS seizes the active focus
    let context = crate::screen::capture_active_context(false);

    let _ = win.center();
    let _ = win.show();
    let _ = win.set_focus();

    #[cfg(target_os = "macos")]
    let _ = app.set_activation_policy(tauri::ActivationPolicy::Regular);

    let _ = win.emit("overlay://shown", &context);
}

/// Dismisses the panel and hands focus back.
///
/// Returning to Accessory is what returns the user to the application they
/// were in. Without it the empty VANS process stays frontmost and they have to
/// click their own window again.
pub fn hide(app: &AppHandle) {
    let Some(win) = window(app) else { return };
    let _ = win.hide();

    #[cfg(target_os = "macos")]
    let _ = app.set_activation_policy(tauri::ActivationPolicy::Accessory);
}

pub fn toggle(app: &AppHandle) {
    let Some(win) = window(app) else { return };
    match win.is_visible() {
        Ok(true) => hide(app),
        _ => show(app),
    }
}

/// Registers the hotkey and the dismiss-on-blur behaviour.
pub fn install(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    // No dock icon. The panel is reached by hotkey, not by an icon to click.
    #[cfg(target_os = "macos")]
    let _ = app.set_activation_policy(tauri::ActivationPolicy::Accessory);

    let handle = app.clone();
    app.global_shortcut().on_shortcut(hotkey(), move |_, _, event| {
        // Fire on press. Acting on release too would toggle twice per tap.
        if event.state() == ShortcutState::Pressed {
            eprintln!("[vans] hotkey fired");
            toggle(&handle);
        }
    })?;

    // Registration can fail silently from the user's point of view — another
    // app already owns the combination, and the panel simply never appears.
    // Say so on stderr rather than leaving them pressing a dead key.
    eprintln!(
        "[vans] hotkey Alt+Space registered = {}",
        app.global_shortcut().is_registered(hotkey())
    );

    if let Some(win) = window(app) {
        let handle = app.clone();
        win.on_window_event(move |event| {
            // Clicking away dismisses, the way a summoned panel should. In-flight
            // work is unaffected: the webview is hidden, not destroyed, so a plan
            // waiting for confirmation is still there when it comes back.
            if let tauri::WindowEvent::Focused(false) = event {
                hide(&handle);
            }
        });
    }

    Ok(())
}
