//! The on-screen coach: a window that points at a control and says what to do.
//!
//! It is deliberately not the panel. The panel takes the keyboard and sits
//! where the user put it; the coach covers the screen, never takes a click, and
//! exists only while it is pointing at something.

use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::Duration;

use serde::Serialize;
use tauri::{
    AppHandle, Emitter, Listener, LogicalPosition, LogicalSize, Manager, WebviewUrl, WebviewWindow,
    WebviewWindowBuilder,
};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Shortcut, ShortcutState};

pub const LABEL: &str = "coach";

/// What is being pointed at, in screen points, while the coach is up.
///
/// Held so a click can be compared against it. Polling the window for changes
/// answers "something happened" a moment later and only sometimes; a mouse-down
/// inside the ring is the person doing the step, now and without ambiguity.
static POINTED_AT: Mutex<Option<(f64, f64, f64, f64)>> = Mutex::new(None);

/// Emitted to the panel when the person clicks what the coach pointed at.
pub const ACTED: &str = "coach://acted";

/// Emitted to the panel when the person presses Escape during a coached run.
pub const STOP: &str = "coach://stop";

/// Whether Escape is currently held for the coach.
static HOLDING_ESCAPE: AtomicBool = AtomicBool::new(false);

fn escape() -> Shortcut {
    Shortcut::new(None, Code::Escape)
}

/// Makes Escape stop the coach, for as long as the coach is on screen.
///
/// The panel already stops a run on Escape — but only while the panel has the
/// keyboard, and during a coached run the panel has put itself away so the person
/// can use their own application. So Escape went to that application and VANS
/// never heard it, which is the one key everyone reaches for to make something go
/// away.
///
/// Registered the same way as ⌥Space, which needs no permission, and only while
/// coaching. Holding Escape any longer would take it from every other
/// application on the machine.
fn hold_escape(app: &AppHandle) {
    if HOLDING_ESCAPE.swap(true, Ordering::SeqCst) {
        return;
    }
    let handle = app.clone();
    let registered = app
        .global_shortcut()
        .on_shortcut(escape(), move |_, _, event| {
            if event.state() == ShortcutState::Pressed {
                eprintln!("[vans] escape: stopping the coached run");
                let _ = handle.emit(STOP, ());
            }
        });
    if let Err(e) = registered {
        HOLDING_ESCAPE.store(false, Ordering::SeqCst);
        eprintln!("[vans] could not take Escape for the coach: {e}");
    }
}

fn release_escape(app: &AppHandle) {
    if HOLDING_ESCAPE.swap(false, Ordering::SeqCst) {
        let _ = app.global_shortcut().unregister(escape());
    }
}

/// Sent by the coach page once its listeners are attached.
const READY: &str = "coach://ready";

/// The last thing the coach was asked to point at, in screen points.
///
/// The window is built the first time it is needed and its page takes a moment
/// to load. An event emitted into that gap reaches nobody — which is exactly the
/// first step of every run, so the first thing anyone was ever shown was
/// nothing. The page says when it is listening, and the pointer is drawn again.
static LAST: Mutex<Option<Pointer>> = Mutex::new(None);

/// The application the current run is being coached in.
///
/// The coach draws over somebody else's window, so it only belongs on screen
/// while that window is the one in front. Swiping to another desktop used to
/// take the ring along — the window joins all Spaces so that it can be summoned
/// anywhere, and the same flag makes it follow you when you leave.
static COACHING: Mutex<Option<String>> = Mutex::new(None);

/// Whether the ring is on screen right now, as against merely remembered.
///
/// Read by the hotkey: while the coach is showing, the hotkey means "next step",
/// and while it is not, it means "open the panel".
static SHOWING: AtomicBool = AtomicBool::new(false);

/// One watcher for the life of the process, started the first time anything is
/// pointed at.
static WATCHING: AtomicBool = AtomicBool::new(false);

/// How often to check whether the person is still looking at the application
/// being coached. Fast enough that leaving feels like it dismissed itself,
/// slow enough to be free.
const LOOK_EVERY: Duration = Duration::from_millis(250);

/// is_showing reports that the coach is on screen and the hotkey should advance
/// the step rather than open the panel.
pub fn is_showing() -> bool {
    SHOWING.load(Ordering::Relaxed)
}

/// Watches which application is in front, and takes the coach off the screen
/// while it is not the one being coached.
///
/// The step is kept, not cancelled. The engine goes on waiting through the whole
/// of this — a window that publishes nothing is somebody having stepped away, and
/// coaching is meant to survive that — so coming back redraws the step that was
/// already there rather than starting the run again.
fn watch_who_is_in_front(app: &AppHandle) {
    if WATCHING.swap(true, Ordering::SeqCst) {
        return;
    }

    let handle = app.clone();
    std::thread::spawn(move || loop {
        std::thread::sleep(LOOK_EVERY);

        let Some(coached) = COACHING.lock().unwrap().clone() else {
            continue;
        };

        // VANS in front means the person is using VANS, which is not the same as
        // having left. Hiding here took the ring away the instant the panel was
        // touched, so it was drawn and gone before anyone saw it.
        if crate::screen::vans_is_in_front() {
            continue;
        }

        let front = crate::screen::frontmost_app_name();

        // Unreadable is not "somewhere else". Leaving the ring up on a maybe is
        // better than flickering it off every time the question cannot be
        // answered.
        let Some(front) = front else { continue };

        let here = front.eq_ignore_ascii_case(&coached);

        if here == SHOWING.load(Ordering::Relaxed) {
            continue;
        }

        if here {
            let last = LAST.lock().unwrap().clone();
            if let Some(p) = last {
                let inner = handle.clone();
                let _ = handle.run_on_main_thread(move || draw(&inner, p));
            }
        } else {
            let inner = handle.clone();
            let _ = handle.run_on_main_thread(move || {
                if let Some(win) = inner.get_webview_window(LABEL) {
                    let _ = win.hide();
                }
            });
            SHOWING.store(false, Ordering::Relaxed);
        }
    });
}

fn redraw_when_the_page_is_listening(app: &AppHandle) {
    let handle = app.clone();
    app.listen_any(READY, move |_| {
        let last = LAST.lock().unwrap().clone();
        let Some(p) = last else { return };
        let inner = handle.clone();
        let _ = handle.run_on_main_thread(move || draw(&inner, p));
    });
}

fn watch_for_the_click(app: &AppHandle) {
    #[cfg(target_os = "macos")]
    {
        use block2::RcBlock;
        use objc2_app_kit::{NSEvent, NSEventMask, NSScreen};

        let handle = app.clone();
        let block = RcBlock::new(move |_event: core::ptr::NonNull<NSEvent>| {
            let Some((x, y, w, h)) = *POINTED_AT.lock().unwrap() else {
                return;
            };

            let at = NSEvent::mouseLocation();

            // AppKit measures from the bottom-left of the primary display and the
            // accessibility API from the top-left, so one of them has to be
            // flipped before they can be compared.
            let flipped = unsafe {
                NSScreen::screens(objc2_foundation::MainThreadMarker::new_unchecked())
                    .firstObject()
                    .map(|s| s.frame().size.height)
                    .unwrap_or(0.0)
            } - at.y;

            if at.x < x || at.x > x + w || flipped < y || flipped > y + h {
                return;
            }
            if let Err(e) = handle.emit(ACTED, ()) {
                eprintln!("[vans] could not report the click on the tip: {e}");
            }
        });

        let _ = NSEvent::addGlobalMonitorForEventsMatchingMask_handler(
            NSEventMask::LeftMouseDown,
            &block,
        );
        std::mem::forget(block);
    }

    #[cfg(not(target_os = "macos"))]
    let _ = app;
}

/// Pointer is a rectangle in screen coordinates and what to say about it.
#[derive(Clone, Serialize)]
pub struct Pointer {
    pub x: f64,
    pub y: f64,
    pub w: f64,
    pub h: f64,
    pub label: String,
    pub step: String,

    /// click, type or press — what the overlay draws on the control.
    pub verb: String,
    /// The text to type or the keys to hold, when the verb needs one.
    pub detail: String,

    /// The application being coached, so the coach can tell when the person has
    /// gone somewhere else and take itself off the screen.
    pub app: String,

    pub mode: String,
    pub ox: f64,
    pub oy: f64,
}

fn build(app: &AppHandle) -> Option<WebviewWindow> {
    if let Some(win) = app.get_webview_window(LABEL) {
        return Some(win);
    }

    let win = WebviewWindowBuilder::new(app, LABEL, WebviewUrl::App("coach.html".into()))
        .title("VANS")
        .decorations(false)
        .transparent(true)
        .always_on_top(true)
        .skip_taskbar(true)
        .focused(false)
        .shadow(false)
        .resizable(false)
        .visible(false)
        .build()
        .map_err(|e| eprintln!("[vans] could not build the coach window: {e}"))
        .ok()?;

    // Never takes a click. The point is to show someone where to press, which
    // is useless if the thing showing them is in the way.
    let _ = win.set_ignore_cursor_events(true);
    float_above(&win);
    watch_for_the_click(app);
    redraw_when_the_page_is_listening(app);
    Some(win)
}

/// The coach sits above the panel and joins every Space, for the reasons in
/// overlay.rs — with one difference: it is never key, so it needs no panel
/// class, only a level and a collection behaviour.
#[cfg(target_os = "macos")]
fn float_above(win: &WebviewWindow) {
    use objc2_app_kit::{NSWindow, NSWindowCollectionBehavior, NSWindowLevel};

    let Ok(ptr) = win.ns_window() else { return };
    if ptr.is_null() {
        return;
    }
    let ns: &NSWindow = unsafe { &*(ptr as *const NSWindow) };

    ns.setCollectionBehavior(
        NSWindowCollectionBehavior::CanJoinAllSpaces
            | NSWindowCollectionBehavior::FullScreenAuxiliary
            | NSWindowCollectionBehavior::IgnoresCycle,
    );

    // One above the panel's status level, so a tip is never covered by the
    // window that produced it.
    const ABOVE_STATUS: NSWindowLevel = 26;
    ns.setLevel(ABOVE_STATUS);
    ns.setIgnoresMouseEvents(true);
}

#[cfg(not(target_os = "macos"))]
fn float_above(_win: &WebviewWindow) {}

/// cover sizes the coach to the display the target is on, and says where that
/// display starts.
///
/// The origin is returned rather than read back off the window afterwards.
/// `outer_position` right after `set_position` still reports where the window
/// used to be, and the first step of a session was drawn at (-196, -133) —
/// off the screen — because it was translated by a stale origin.
fn cover(win: &WebviewWindow, at: (f64, f64)) -> Option<(f64, f64)> {
    let monitors = win.available_monitors().unwrap_or_default();
    if monitors.is_empty() {
        eprintln!("[vans] coach: no monitors reported; the overlay has no size to take");
        return None;
    }
    let chosen = monitors
        .iter()
        .find(|m| {
            let p = m.position();
            let s = m.size();
            let scale = m.scale_factor();
            let (x0, y0) = (p.x as f64 / scale, p.y as f64 / scale);
            let (w, h) = (s.width as f64 / scale, s.height as f64 / scale);
            at.0 >= x0 && at.0 < x0 + w && at.1 >= y0 && at.1 < y0 + h
        })
        .or_else(|| monitors.first());

    let m = chosen?;

    let scale = m.scale_factor();
    let p = m.position();
    let s = m.size();
    let origin = (p.x as f64 / scale, p.y as f64 / scale);
    let _ = win.set_position(LogicalPosition::new(origin.0, origin.1));
    let _ = win.set_size(LogicalSize::new(
        s.width as f64 / scale,
        s.height as f64 / scale,
    ));
    eprintln!(
        "[vans] coach: covering display at ({}, {}) {}x{} scale {}",
        origin.0,
        origin.1,
        s.width as f64 / scale,
        s.height as f64 / scale,
        scale
    );
    Some(origin)
}

/// point_at shows the coach over rect, with label beside it.
///
/// The work is marshalled to the main thread rather than assumed to be on it.
/// Building a window and setting a level are AppKit calls, and AppKit takes the
/// process down when they arrive from anywhere else — which is not a failure
/// the caller of a drawing function should have to know about.
pub fn point_at(app: &AppHandle, p: Pointer) {
    let handle = app.clone();
    if let Err(e) = app.run_on_main_thread(move || draw(&handle, p)) {
        eprintln!("[vans] could not reach the main thread to draw a tip: {e}");
    }
}

fn draw(app: &AppHandle, mut p: Pointer) {
    *LAST.lock().unwrap() = Some(p.clone());

    // Only a step with a control to point at belongs to one application's window.
    // A step with nothing to point at is an instruction — usually "open System
    // Settings" — and the application it names is by definition not in front yet.
    // Watching for it hid the one step telling the person to go and open it, the
    // moment it appeared (seen live 2026-09-29).
    let points_at_something = p.w > 0.0 && p.h > 0.0;
    *COACHING.lock().unwrap() = if points_at_something && !p.app.is_empty() {
        Some(p.app.clone())
    } else {
        None
    };
    watch_who_is_in_front(app);

    let Some(win) = build(app) else { return };

    *POINTED_AT.lock().unwrap() = Some((p.x, p.y, p.w, p.h));

    // The page draws in window coordinates, so the rectangle is moved from screen
    // space into the window that is about to cover that screen.
    if let Some(origin) = cover(&win, (p.x, p.y)) {
        p.x -= origin.0;
        p.y -= origin.1;
        p.ox = origin.0;
        p.oy = origin.1;
    }

    show_without_taking_the_keyboard(&win);
    SHOWING.store(true, Ordering::Relaxed);
    hold_escape(app);
    eprintln!(
        "[vans] coach: drawing {}x{} at ({}, {}) in window coordinates, visible = {:?}",
        p.w,
        p.h,
        p.x,
        p.y,
        win.is_visible()
    );
    if let Err(e) = win.emit_to(LABEL, "coach:point", p) {
        eprintln!("[vans] could not draw the tip: {e}");
    }
}

/// Puts the coach on screen over whatever application is active.
///
/// Both calls, and in this order. `show()` is what tells Tauri the window is up,
/// and without it the webview has no reason to paint. `orderFrontRegardless` is
/// then what guarantees it is ordered above the active application's windows
/// without VANS having to become active to get there — which it must not do,
/// because an application that is not active stops publishing its window to the
/// accessibility API (D-075).
#[cfg(target_os = "macos")]
fn show_without_taking_the_keyboard(win: &WebviewWindow) {
    use objc2_app_kit::NSWindow;

    let _ = win.show();

    let Ok(ptr) = win.ns_window() else { return };
    if ptr.is_null() {
        return;
    }
    let ns: &NSWindow = unsafe { &*(ptr as *const NSWindow) };
    ns.orderFrontRegardless();
}

#[cfg(not(target_os = "macos"))]
fn show_without_taking_the_keyboard(win: &WebviewWindow) {
    let _ = win.show();
}

/// How long the tick stays up before the overlay goes away.
const LINGER: std::time::Duration = std::time::Duration::from_millis(1100);

/// finish marks the last control done and then takes the coach off the screen.
///
/// A run that finished and a run that was stopped used to look identical: the
/// overlay simply vanished. The wait is here rather than in the window so the
/// caller has one call and no timing to get right.
pub fn finish(app: &AppHandle) {
    let handle = app.clone();
    let _ = app.run_on_main_thread(move || {
        if let Some(win) = handle.get_webview_window(LABEL) {
            let _ = win.emit_to(LABEL, "coach:done", ());
        }
        let later = handle.clone();
        std::thread::spawn(move || {
            std::thread::sleep(LINGER);
            clear(&later);
        });
    });
}

/// clear takes the coach off the screen.
pub fn clear(app: &AppHandle) {
    *POINTED_AT.lock().unwrap() = None;
    *LAST.lock().unwrap() = None;
    *COACHING.lock().unwrap() = None;
    SHOWING.store(false, Ordering::Relaxed);
    release_escape(app);

    let handle = app.clone();
    let _ = app.run_on_main_thread(move || {
        if let Some(win) = handle.get_webview_window(LABEL) {
            let _ = win.emit_to(LABEL, "coach:clear", ());
            let _ = win.hide();
        }
    });
}
