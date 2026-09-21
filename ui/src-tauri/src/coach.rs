//! The on-screen coach: a window that points at a control and says what to do.
//!
//! It is deliberately not the panel. The panel takes the keyboard and sits
//! where the user put it; the coach covers the screen, never takes a click, and
//! exists only while it is pointing at something.

use std::sync::Mutex;

use serde::Serialize;
use tauri::{
    AppHandle, Emitter, LogicalPosition, LogicalSize, Manager, WebviewUrl, WebviewWindow,
    WebviewWindowBuilder,
};

pub const LABEL: &str = "coach";

/// What is being pointed at, in screen points, while the coach is up.
///
/// Held so a click can be compared against it. Polling the window for changes
/// answers "something happened" a moment later and only sometimes; a mouse-down
/// inside the ring is the person doing the step, now and without ambiguity.
static POINTED_AT: Mutex<Option<(f64, f64, f64, f64)>> = Mutex::new(None);

/// Emitted to the panel when the person clicks what the coach pointed at.
pub const ACTED: &str = "coach://acted";

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

/// cover sizes the coach to the display the target is on.
fn cover(win: &WebviewWindow, at: (f64, f64)) {
    let monitors = win.available_monitors().unwrap_or_default();
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

    let Some(m) = chosen else { return };

    let scale = m.scale_factor();
    let p = m.position();
    let s = m.size();
    let _ = win.set_position(LogicalPosition::new(p.x as f64 / scale, p.y as f64 / scale));
    let _ = win.set_size(LogicalSize::new(
        s.width as f64 / scale,
        s.height as f64 / scale,
    ));
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
    let Some(win) = build(app) else { return };

    *POINTED_AT.lock().unwrap() = Some((p.x, p.y, p.w, p.h));

    cover(&win, (p.x, p.y));

    // The page draws in window coordinates, so the rectangle is moved from
    // screen space into the window that is about to cover that screen.
    if let Ok(origin) = win.outer_position() {
        let scale = win.scale_factor().unwrap_or(1.0);
        p.x -= origin.x as f64 / scale;
        p.y -= origin.y as f64 / scale;
    }

    show_without_taking_the_keyboard(&win);
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

/// clear takes the coach off the screen.
pub fn clear(app: &AppHandle) {
    *POINTED_AT.lock().unwrap() = None;

    let handle = app.clone();
    let _ = app.run_on_main_thread(move || {
        if let Some(win) = handle.get_webview_window(LABEL) {
            let _ = win.emit_to(LABEL, "coach:clear", ());
            let _ = win.hide();
        }
    });
}
