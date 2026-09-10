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

/// Matches the CSS radius on .panel. The blur is a native layer behind the
/// webview, so if it does not round with the panel it shows as square corners
/// poking out from under the rounded ones.
const CORNER_RADIUS: f64 = 14.0;

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

/// Whether the panel currently holds the keyboard.
///
/// Asked of AppKit rather than of Tauri. `is_focused` reports whether the
/// *application* is active, and this one deliberately never activates — so it
/// answers false even when the panel is right there with a cursor blinking in
/// it, and the hotkey could then only ever open, never close.
#[cfg(target_os = "macos")]
fn holds_keyboard(win: &WebviewWindow) -> bool {
    use objc2_app_kit::NSWindow;

    let Ok(ptr) = win.ns_window() else { return false };
    if ptr.is_null() {
        return false;
    }
    let ns: &NSWindow = unsafe { &*(ptr as *const NSWindow) };
    ns.isKeyWindow()
}

#[cfg(not(target_os = "macos"))]
fn holds_keyboard(win: &WebviewWindow) -> bool {
    win.is_focused().unwrap_or(false)
}

pub fn toggle(app: &AppHandle) {
    let Some(win) = window(app) else { return };

    // Visible but not holding the keyboard means it was pinned and clicked
    // away from. Treat the hotkey as a request to come back rather than to
    // dismiss, or the panel that deliberately stayed up is the one the hotkey
    // cannot recover.
    if win.is_visible().unwrap_or(false) && holds_keyboard(&win) {
        hide(app);
    } else {
        show(app);
    }
}

/// An NSPanel subclass that agrees to take the keyboard.
///
/// Plain NSPanel is not enough. `canBecomeKeyWindow` is false by default for a
/// window with no title bar, and this one is deliberately borderless — so
/// swapping to NSPanel alone produced a panel that appeared over full-screen
/// apps and could not be typed into. The one override is the whole reason this
/// class exists.
///
/// Registered once. A second `ClassBuilder::new` with the same name returns
/// None, so the result is cached rather than rebuilt per call.
#[cfg(target_os = "macos")]
fn vans_panel_class() -> Option<&'static objc2::runtime::AnyClass> {
    use std::sync::OnceLock;

    use objc2::runtime::{AnyClass, AnyObject, Bool, ClassBuilder, Sel};

    static CLASS: OnceLock<Option<&'static AnyClass>> = OnceLock::new();

    *CLASS.get_or_init(|| {
        extern "C" fn can_become_key(_this: &AnyObject, _cmd: Sel) -> Bool {
            Bool::YES
        }

        let superclass = AnyClass::get(c"NSPanel")?;
        let mut builder = ClassBuilder::new(c"VansPanel", superclass)?;

        // Safety: the signature matches canBecomeKeyWindow's, which takes no
        // arguments beyond the implicit receiver and selector and returns BOOL.
        unsafe {
            builder.add_method(
                objc2::sel!(canBecomeKeyWindow),
                // Inferred rather than spelled out: naming the receiver's
                // lifetime pins it, and the trait wants a signature generic
                // over any lifetime.
                can_become_key as extern "C" fn(_, _) -> _,
            );
        }
        Some(builder.register())
    })
}

/// Lets the panel appear over a full-screen application.
///
/// Joining all Spaces is only half of it. A full-screen app owns its own
/// Space, and an ordinary window — even one that can join every Space — is
/// still hidden behind it. `FullScreenAuxiliary` is the behaviour that says
/// "I am a panel, show me over the full-screen window rather than switching
/// away from it", and there is no Tauri call for it.
///
/// The level goes up to match. Always-on-top puts the window above normal
/// windows, and a full-screen app is not a normal window: below the status
/// level it stays behind. Stationary keeps it from sliding under a Spaces
/// swipe, which for a panel that is meant to be above everything reads as it
/// falling off the screen.
#[cfg(target_os = "macos")]
fn float_over_fullscreen(win: &WebviewWindow) {
    use objc2::runtime::AnyObject;
    use objc2_app_kit::{NSWindow, NSWindowCollectionBehavior, NSWindowLevel, NSWindowStyleMask};

    let Ok(ptr) = win.ns_window() else {
        eprintln!("[vans] no native window; the panel will not cover full-screen apps");
        return;
    };
    if ptr.is_null() {
        return;
    }

    // Safety: Tauri hands back the NSWindow backing this webview, and this
    // runs on the main thread during setup, which is where AppKit requires it.
    let ns: &NSWindow = unsafe { &*(ptr as *const NSWindow) };

    // Stationary is deliberately not in this set. It belongs to the same
    // mutually-exclusive group as Managed and Transient, and AppKit ignores a
    // behaviour set that contradicts itself rather than reporting it.
    ns.setCollectionBehavior(
        NSWindowCollectionBehavior::CanJoinAllSpaces
            | NSWindowCollectionBehavior::FullScreenAuxiliary,
    );

    // NSStatusWindowLevel. Written as a number because objc2 exposes the level
    // as a bare integer rather than the old named constants.
    const STATUS_LEVEL: NSWindowLevel = 25;
    ns.setLevel(STATUS_LEVEL);

    // The collection behaviour above is necessary and not sufficient, which is
    // the part that cost an afternoon: with both flags set and the level
    // raised, the panel still would not appear over a full-screen app.
    //
    // The reason is focus. An ordinary NSWindow becoming key activates its
    // application, and activating an application pulls macOS out of the
    // full-screen Space to get to it — so the panel is drawn correctly, in a
    // Space you are no longer looking at.
    //
    // NSWindowStyleMaskNonactivatingPanel is the flag that says "take the
    // keyboard without activating me", and AppKit only honours it on NSPanel.
    // Tauri builds an NSWindow, so the class is swapped underneath it. That is
    // legitimate — NSPanel adds no instance variables, so the object layout is
    // unchanged and the existing delegate and content view keep working.
    let Some(panel_class) = vans_panel_class() else {
        eprintln!("[vans] could not build the panel class; full-screen overlay unavailable");
        return;
    };
    unsafe {
        objc2::ffi::object_setClass(ptr as *mut AnyObject, panel_class);
    }
    ns.setStyleMask(ns.styleMask() | NSWindowStyleMask::NonactivatingPanel);

    eprintln!(
        "[vans] panel: behaviour = {:?}, level = {}, nonactivating = {}, can be key = {}",
        ns.collectionBehavior(),
        ns.level(),
        ns.styleMask().contains(NSWindowStyleMask::NonactivatingPanel),
        ns.canBecomeKeyWindow()
    );
}

/// Frosts the window using AppKit rather than CSS.
///
/// `backdrop-filter` did work, but it arrived seconds after the panel did: a
/// transparent WKWebView has nothing behind it inside the page to sample, so
/// the blur only resolves once the compositor has caught up with the window.
/// A summoned panel that spends its first seconds as a solid box has failed at
/// the moment it is actually being looked at.
///
/// NSVisualEffectView sits behind the webview and is composited by the window
/// server, so it is correct in the first frame. It also samples the desktop
/// properly, which CSS cannot do from inside a transparent window.
#[cfg(target_os = "macos")]
fn frost(win: &WebviewWindow, theme: tauri::Theme) {
    use window_vibrancy::{
        apply_liquid_glass, apply_vibrancy, LiquidGlassOptions, NSGlassEffectViewStyle,
        NSVisualEffectMaterial, NSVisualEffectState,
    };

    // Liquid Glass where the OS has it. NSVisualEffectView is the older
    // frosted-pane look; on macOS 26 the system's own panels are lit glass
    // with a specular edge, and a panel that floats over that using the
    // previous decade's material reads as a foreign object rather than a
    // native one.
    //
    // Tried first and fallen back on rather than gated on a version check:
    // the call fails on an OS without it, which is the same answer a version
    // check would have given, without a second thing to keep in step.
    let glass = apply_liquid_glass(
        win,
        LiquidGlassOptions::new(NSGlassEffectViewStyle::Regular).radius(CORNER_RADIUS),
    );
    if glass.is_ok() {
        return;
    }

    // The material carries its own light/dark rendering, so the theme has to
    // reach AppKit as well as CSS — a light panel over a dark blur reads as a
    // dark panel with a light patch in it.
    let material = match theme {
        tauri::Theme::Dark => NSVisualEffectMaterial::HudWindow,
        _ => NSVisualEffectMaterial::Popover,
    };

    // Active regardless of focus. The panel pins itself on some screens, and
    // FollowsWindowActiveState would drain the blur out of a pinned panel the
    // moment the user clicked away to do what it asked.
    // A failure here is invisible from the outside — the panel simply renders
    // as a flat tint over a transparent window, which looks like a styling
    // choice rather than a broken one. Same reasoning as the hotkey below.
    if let Err(e) = apply_vibrancy(
        win,
        material,
        Some(NSVisualEffectState::Active),
        Some(CORNER_RADIUS),
    ) {
        eprintln!("[vans] could not frost the panel: {e}");
    }
}

/// Applies a theme to the window, and to the blur behind it.
pub fn set_theme(app: &AppHandle, dark: bool) {
    let Some(win) = window(app) else { return };
    let theme = if dark {
        tauri::Theme::Dark
    } else {
        tauri::Theme::Light
    };
    let _ = win.set_theme(Some(theme));
    #[cfg(target_os = "macos")]
    if BLURRED.load(Ordering::Relaxed) {
        frost(&win, theme);
    }
}

/// Whether the frosting layer is present.
///
/// Separate from how opaque the panel's own tint is, because they are
/// genuinely different questions. The blur decides whether you can make out
/// what is behind the panel; the tint decides how strongly the panel's own
/// surface sits over it. Frosted-and-faint and clear-and-solid are both
/// reasonable places to be, and one slider could not express either.
static BLURRED: AtomicBool = AtomicBool::new(true);

/// Turns the frosting layer on or off.
pub fn set_blur(app: &AppHandle, blurred: bool, dark: bool) {
    let Some(win) = window(app) else { return };
    BLURRED.store(blurred, Ordering::Relaxed);

    #[cfg(target_os = "macos")]
    {
        if blurred {
            let theme = if dark {
                tauri::Theme::Dark
            } else {
                tauri::Theme::Light
            };
            frost(&win, theme);
        } else {
            // Both, because which one was applied depends on the OS and this
            // is the only place that knows it should now be neither.
            let _ = window_vibrancy::clear_liquid_glass(&win);
            if let Err(e) = window_vibrancy::clear_vibrancy(&win) {
                eprintln!("[vans] could not clear the frosting: {e}");
            }
        }
    }

    #[cfg(not(target_os = "macos"))]
    let _ = (win, dark);
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
        #[cfg(target_os = "macos")]
        float_over_fullscreen(&win);

        // Follow the user between desktops instead of dragging them to VANS.
        //
        // A window belongs to the Space it was created on. Summoning it from
        // another one made macOS switch Spaces to reach it — the whole desktop
        // slid sideways on every press of the hotkey. Joining all Spaces is
        // what Spotlight does and what a summoned panel has to do: it appears
        // where the user already is, over a full-screen app included.
        let _ = win.set_visible_on_all_workspaces(true);

        // Light until the window says otherwise. Doing it here rather than
        // waiting for the frontend means the first frame is already frosted;
        // the frontend re-applies only if the stored choice differs.
        let _ = win.set_theme(Some(tauri::Theme::Light));
        #[cfg(target_os = "macos")]
        frost(&win, tauri::Theme::Light);

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
