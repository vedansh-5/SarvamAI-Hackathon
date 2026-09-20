//! The overlay: a panel summoned by a hotkey, over whatever you are doing.
//!
//! VANS is not a place you go. It is something you call while working in
//! another application, so the window has no dock icon, no taskbar entry, and
//! no chrome — and it starts hidden, because an app that seizes the screen on
//! launch is the thing this replaces.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;

use tauri::{AppHandle, Emitter, LogicalPosition, LogicalSize, Manager, WebviewWindow};
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
const CORNER_RADIUS: f64 = 18.0;

/// How close to the edge of the display the panel is allowed to sit.
const EDGE_MARGIN: f64 = 12.0;

/// Whether the panel has been given a position yet. It is centred once, on the
/// first summon; after that where it sits is wherever the user dragged it to.
static PLACED: AtomicBool = AtomicBool::new(false);

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
/// `show` is the whole of it on macOS. It orders the panel front and makes it
/// key, and a non-activating panel takes the keyboard without its application
/// becoming the active one — which is the entire point of the class swap in
/// `float_over_fullscreen`.
///
/// `set_focus` is deliberately not called. Tauri routes it through
/// `activateIgnoringOtherApps`, which makes VANS the active application, and
/// that is not cosmetic: macOS withdraws the windows of every application that
/// is not active from the accessibility API when Stage Manager is on. The
/// window the user asked about stops publishing anything at all, so pointing at
/// a control in it — and every re-read that multi-step coaching depends on —
/// fails with the window apparently closed.
pub fn show(app: &AppHandle) {
    let Some(win) = window(app) else { return };

    // Capture context before VANS seizes the active focus
    let context = crate::screen::capture_active_context(false);

    if !PLACED.swap(true, Ordering::Relaxed) {
        let _ = win.center();
    }

    // Every time, not only on resize: a panel that was left off screen must
    // come back when it is summoned, or the hotkey appears to do nothing.
    let height = win
        .outer_size()
        .map(|s| s.to_logical::<f64>(win.scale_factor().unwrap_or(1.0)).height)
        .unwrap_or(MIN_HEIGHT);
    keep_on_screen(&win, height);

    let _ = win.show();
    #[cfg(not(target_os = "macos"))]
    let _ = win.set_focus();

    let _ = win.emit("overlay://shown", &context);
}

/// Every path that puts the panel away, with why.
///
/// The panel was dismissing itself with nobody touching it, and three guesses
/// at the cause had each been half right. Naming the caller is the difference
/// between reading the answer out of a log and guessing a fourth time.
pub fn hide_because(app: &AppHandle, reason: &str) {
    eprintln!("[vans] hiding: {reason}");
    let Some(win) = window(app) else { return };
    let _ = win.hide();
}

pub fn set_pinned(pinned: bool) {
    PINNED.store(pinned, Ordering::Relaxed);
}

/// True while the user is dragging the panel by its header.
///
/// Moving a window changes which window is key, so a drag produces exactly
/// the event that dismiss-on-blur is watching for — the panel disappeared
/// from under the pointer that was moving it, which reads as "dragging does
/// nothing" rather than as "it closed".
static DRAGGING: AtomicBool = AtomicBool::new(false);

pub fn set_dragging(dragging: bool) {
    DRAGGING.store(dragging, Ordering::Relaxed);
}

/// Where the panel was when the current drag began, in logical points.
static DRAG_ORIGIN: Mutex<Option<(f64, f64)>> = Mutex::new(None);

/// Records the panel's position so a drag can be measured from it.
///
/// Read here rather than in the window, because the frontend's route to it is
/// two async calls that can fail into a console nobody is reading. The whole
/// drag having quietly done nothing was the symptom.
pub fn begin_drag(app: &AppHandle) {
    let Some(win) = window(app) else { return };
    set_dragging(true);

    let scale = win.scale_factor().unwrap_or(1.0);
    match win.outer_position() {
        Ok(p) => {
            let at = p.to_logical::<f64>(scale);
            *DRAG_ORIGIN.lock().unwrap() = Some((at.x, at.y));
        }
        Err(e) => {
            eprintln!("[vans] could not read the panel position to drag it: {e}");
            *DRAG_ORIGIN.lock().unwrap() = None;
        }
    }
}

/// Moves the panel to where the drag has taken it.
pub fn drag_by(app: &AppHandle, dx: f64, dy: f64) {
    let Some(origin) = *DRAG_ORIGIN.lock().unwrap() else {
        return;
    };
    let Some(win) = window(app) else { return };
    if !dx.is_finite() || !dy.is_finite() {
        return;
    }
    let _ = win.set_position(LogicalPosition::new(origin.0 + dx, origin.1 + dy));

    // Dragging stops at the edge rather than off it. A panel dragged past the
    // top cannot be dragged back, because a window that is fully off screen
    // belongs to no monitor and every correction that asks for one gives up.
    let height = win
        .outer_size()
        .map(|s| s.to_logical::<f64>(win.scale_factor().unwrap_or(1.0)).height)
        .unwrap_or(MIN_HEIGHT);
    keep_on_screen(&win, height);
}

/// Dismisses the panel when a click lands outside it.
///
/// A click, specifically. Blur was tried and removed: a panel that floats over
/// full-screen apps and never activates gives up key status for the menu bar
/// auto-revealing, for a full-screen window's controls appearing, for being
/// moved — none of which is the user going somewhere else. Watching for the
/// mouse going down in another application is the thing that actually
/// happened, and hovering and scrolling do not produce it.
///
/// The monitor is global, so it never sees clicks inside VANS itself; the
/// frame check is there for the case where it does.
#[cfg(target_os = "macos")]
fn dismiss_on_click_outside(app: &AppHandle) {
    use block2::RcBlock;
    use objc2_app_kit::{NSEvent, NSEventMask, NSWindow};

    let handle = app.clone();
    let block = RcBlock::new(move |event: core::ptr::NonNull<NSEvent>| {
        if PINNED.load(Ordering::Relaxed) || DRAGGING.load(Ordering::Relaxed) {
            return;
        }
        let Some(win) = window(&handle) else { return };
        if !win.is_visible().unwrap_or(false) {
            return;
        }

        let at = NSEvent::mouseLocation();

        let Ok(ptr) = win.ns_window() else { return };
        if ptr.is_null() {
            return;
        }
        let ns: &NSWindow = unsafe { &*(ptr as *const NSWindow) };
        let frame = ns.frame();

        // Screen coordinates, origin bottom-left, same for both.
        let inside = at.x >= frame.origin.x
            && at.x <= frame.origin.x + frame.size.width
            && at.y >= frame.origin.y
            && at.y <= frame.origin.y + frame.size.height;
        if inside {
            return;
        }

        let inner = handle.clone();
        let _ = handle.run_on_main_thread(move || hide_because(&inner, "clicked outside"));
        let _ = event;
    });

    let mask = NSEventMask::LeftMouseDown
        | NSEventMask::RightMouseDown
        | NSEventMask::OtherMouseDown;
    // The returned token is deliberately dropped: the monitor lives as long as
    // the process, which is as long as the panel can be shown.
    let _ = NSEvent::addGlobalMonitorForEventsMatchingMask_handler(mask, &block);
    std::mem::forget(block);
}

pub fn end_drag() {
    set_dragging(false);
    *DRAG_ORIGIN.lock().unwrap() = None;
}

static COMPACT: AtomicBool = AtomicBool::new(false);

pub fn set_compact(app: &AppHandle, compact: bool) {
    COMPACT.store(compact, Ordering::Relaxed);
    let Some(win) = window(app) else { return };
    let width = if compact { 380.0 } else { 720.0 };
    let height = win
        .inner_size()
        .map(|s| s.to_logical::<f64>(win.scale_factor().unwrap_or(1.0)).height)
        .unwrap_or(MIN_HEIGHT);
    let _ = win.set_size(LogicalSize::new(width, height));
    keep_on_screen(&win, height);
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

    let width = if COMPACT.load(Ordering::Relaxed) {
        380.0
    } else {
        720.0
    };

    let _ = win.set_size(LogicalSize::new(width, height));
    keep_on_screen(&win, height);
}

/// Where the panel sits is the user's, not the runtime's.
///
/// It is centred once, the first time it is summoned, and never moved again —
/// growing content pushes the bottom edge down rather than recentring, because
/// recentring on every resize walked the panel out from under a drag and made
/// it look as though it could not be moved at all. The only exception is
/// falling off the display, which is corrected by the smallest shift that
/// puts it back.
// A window that has drifted entirely off screen reports no current monitor, so
// asking only for that one is how a panel gets stuck where it cannot be seen.
fn monitor_for(win: &WebviewWindow) -> Option<tauri::Monitor> {
    if let Ok(Some(m)) = win.current_monitor() {
        return Some(m);
    }
    win.primary_monitor().ok().flatten()
}

fn keep_on_screen(win: &WebviewWindow, height: f64) {
    let Some(monitor) = monitor_for(win) else {
        return;
    };
    let scale = monitor.scale_factor();
    let area = monitor.size().to_logical::<f64>(scale);
    let origin = monitor.position().to_logical::<f64>(scale);

    let Ok(position) = win.outer_position() else {
        return;
    };
    let position = position.to_logical::<f64>(win.scale_factor().unwrap_or(scale));

    let width = if COMPACT.load(Ordering::Relaxed) {
        380.0
    } else {
        720.0
    };

    let bottom = origin.y + area.height;
    let lowest = bottom - height - EDGE_MARGIN;
    let y = position.y.min(lowest).max(origin.y + EDGE_MARGIN);

    let rightmost = origin.x + area.width - width - EDGE_MARGIN;
    let x = position.x.min(rightmost).max(origin.x + EDGE_MARGIN);

    if (y - position.y).abs() > 1.0 || (x - position.x).abs() > 1.0 {
        let _ = win.set_position(LogicalPosition::new(x, y));
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
        hide_because(app, "hotkey pressed while it held the keyboard");
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
    // Applying is not idempotent: it inserts a tagged glass view and reparents
    // the webview into it. Called twice without clearing, the panel ends up
    // with two of them and a content view nested inside the first, and the
    // clear that follows only finds the outer one — which is what made the
    // toggle stop responding after the opacity slider had been dragged.
    let _ = window_vibrancy::clear_liquid_glass(win);
    let _ = window_vibrancy::clear_vibrancy(win);

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

        #[cfg(target_os = "macos")]
        dismiss_on_click_outside(app);

        // Deliberately no dismiss-on-blur.
        //
        // A panel that floats over full-screen apps and never activates gives
        // up key status constantly, and for reasons that have nothing to do
        // with the user leaving: the menu bar auto-revealing over a
        // full-screen window does it, so does that window's own controls
        // appearing, so does being moved or resized. The log showed three
        // Focused(false) events with nobody touching anything, each one
        // closing the panel.
        //
        // Blur was never a reliable signal here, and every attempt to make it
        // one added a condition that was wrong somewhere else. Dismissal is
        // now only ever asked for: Escape, the hotkey, or the window saying
        // so. Nobody has ever complained that it stayed open.
    }

    Ok(())
}
