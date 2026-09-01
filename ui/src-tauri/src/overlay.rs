//! The overlay: a panel summoned by a hotkey, over whatever you are doing.
//!
//! VANS is not a place you go. It is something you call while working in
//! another application, so the window has no dock icon, no taskbar entry, and
//! no chrome — and it starts hidden, because an app that seizes the screen on
//! launch is the thing this replaces.

use std::sync::atomic::{AtomicBool, Ordering};

use tauri::{AppHandle, Emitter, LogicalSize, Manager, WebviewWindow};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};

/// Whether clicking away leaves the panel up.
///
/// Dismiss-on-blur is right for the task form and wrong for preflight, which
/// tells you to go start Chrome in a terminal and then disappears the moment
/// you click the terminal. A screen whose whole purpose is to send you
/// somewhere else has to survive you going there.
static PINNED: AtomicBool = AtomicBool::new(false);

/// Below this the panel reads as a glitch rather than a window.
const MIN_HEIGHT: f64 = 180.0;

/// The panel is an overlay, not a full-screen app: past this fraction of the
/// display it stops looking like something summoned over your work.
const MAX_SCREEN_FRACTION: f64 = 0.85;

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

    // Only recentre when summoning it fresh. A pinned panel that the user
    // clicked away from is still on screen where they left it, and moving it
    // out from under them on the way back is disorienting.
    if !win.is_visible().unwrap_or(false) {
        let _ = win.center();
    }
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

pub fn set_pinned(pinned: bool) {
    PINNED.store(pinned, Ordering::Relaxed);
}

/// Sizes the window to the height its content asked for.
///
/// A summoned panel is not a document viewer. At a fixed height the preflight
/// screen — the one screen whose entire job is to be read — came up scrolled
/// into its own middle with the heading off the top, which is how the fixed
/// size was found. The window follows the content instead, within bounds.
pub fn resize(app: &AppHandle, content_height: f64) {
    let Some(win) = window(app) else { return };
    if !content_height.is_finite() {
        return;
    }

    let ceiling = win
        .current_monitor()
        .ok()
        .flatten()
        .map(|m| m.size().to_logical::<f64>(m.scale_factor()).height * MAX_SCREEN_FRACTION)
        .unwrap_or(f64::INFINITY);

    // clamp panics if the bounds cross, which they do on a display shorter
    // than MIN_HEIGHT/MAX_SCREEN_FRACTION.
    let height = content_height.clamp(MIN_HEIGHT, ceiling.max(MIN_HEIGHT));

    let width = win
        .inner_size()
        .map(|s| s.to_logical::<f64>(win.scale_factor().unwrap_or(1.0)).width)
        .unwrap_or(720.0);

    let _ = win.set_size(LogicalSize::new(width, height));

    // Recentre rather than anchoring an edge. Growth is driven by moving
    // between screens, which is infrequent and deliberate, and centring is the
    // only rule that cannot walk the panel off the bottom of the display.
    if win.is_visible().unwrap_or(false) {
        let _ = win.center();
    }
}

pub fn toggle(app: &AppHandle) {
    let Some(win) = window(app) else { return };
    // Visible but not focused means pinned and clicked away from. Treat the
    // hotkey as a request to come back, not to dismiss — otherwise the panel
    // that deliberately stayed up is the one the hotkey cannot recover.
    let up = win.is_visible().unwrap_or(false) && win.is_focused().unwrap_or(false);
    if up {
        hide(app);
    } else {
        show(app);
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
            // Clicking away dismisses, the way a summoned panel should, unless
            // the window has pinned itself. In-flight work is unaffected either
            // way: the webview is hidden, not destroyed, so a plan waiting for
            // confirmation is still there when it comes back.
            if let tauri::WindowEvent::Focused(false) = event {
                if !PINNED.load(Ordering::Relaxed) {
                    hide(&handle);
                }
            }
        });
    }

    Ok(())
}
