use std::sync::Mutex;
use std::time::{Duration, Instant};
use tauri::{AppHandle, LogicalPosition, LogicalSize, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder};

pub const LABEL: &str = "nudge";
const WIDTH: f64 = 440.0;
const GAP: f64 = 6.0;
const STILL_FOR: f64 = 2.5;
const AFTER_WAKE: Duration = Duration::from_secs(5);
const HOLD_AT_MOST: Duration = Duration::from_secs(120);
const LOOK: Duration = Duration::from_millis(400);

static WOKE: Mutex<Option<Instant>> = Mutex::new(None);

pub fn woke() {
    *WOKE.lock().unwrap() = Some(Instant::now());
}

fn settled() -> bool {
    WOKE.lock().unwrap().is_none_or(|at| at.elapsed() >= AFTER_WAKE)
}

#[cfg(target_os = "macos")]
fn idle_seconds() -> f64 {
    #[link(name = "CoreGraphics", kind = "framework")]
    extern "C" {
        fn CGEventSourceSecondsSinceLastEventType(state: i32, event_type: u32) -> f64;
    }
    const COMBINED_SESSION: i32 = 0;
    const ANY_INPUT: u32 = u32::MAX;
    unsafe { CGEventSourceSecondsSinceLastEventType(COMBINED_SESSION, ANY_INPUT) }
}

#[cfg(not(target_os = "macos"))]
fn idle_seconds() -> f64 {
    STILL_FOR
}

fn window(app: &AppHandle) -> Option<WebviewWindow> {
    app.get_webview_window(LABEL)
}

fn put(app: &AppHandle, height: f64) {
    let Some(win) = window(app) else { return };
    let Some(at) = crate::notch::place() else { return };
    let x = (at.x + crate::notch::DOT_BOX / 2.0 - WIDTH / 2.0).max(8.0);
    let y = at.y + at.height + GAP;
    let _ = win.set_size(LogicalSize::new(WIDTH, height));
    let _ = win.set_position(LogicalPosition::new(x, y));
}

#[cfg(target_os = "macos")]
fn float(win: &WebviewWindow) {
    use objc2_app_kit::{NSWindow, NSWindowCollectionBehavior, NSWindowLevel};

    let Ok(ptr) = win.ns_window() else { return };
    if ptr.is_null() {
        return;
    }
    let ns: &NSWindow = unsafe { &*(ptr as *const NSWindow) };
    ns.setCollectionBehavior(
        NSWindowCollectionBehavior::CanJoinAllSpaces
            | NSWindowCollectionBehavior::Stationary
            | NSWindowCollectionBehavior::IgnoresCycle,
    );
    const OVER_THE_MENU_BAR: NSWindowLevel = 25;
    ns.setLevel(OVER_THE_MENU_BAR);
    ns.setHasShadow(false);
}

#[cfg(not(target_os = "macos"))]
fn float(_win: &WebviewWindow) {}

pub async fn show_when_still(app: AppHandle, height: f64) {
    let started = Instant::now();
    while started.elapsed() < HOLD_AT_MOST && !(settled() && idle_seconds() >= STILL_FOR) {
        tokio::time::sleep(LOOK).await;
    }
    let handle = app.clone();
    let _ = app.run_on_main_thread(move || {
        put(&handle, height);
        if let Some(win) = window(&handle) {
            float(&win);
            let _ = win.show();
        }
    });
}

pub fn hide(app: &AppHandle) {
    if let Some(win) = window(app) {
        let _ = win.hide();
    }
}

pub fn open_app(name: &str) -> Result<(), String> {
    let name = name.trim();
    if name.is_empty() {
        return Err("No application named.".into());
    }
    let done = std::process::Command::new("open")
        .args(["-a", name])
        .status()
        .map_err(|e| e.to_string())?;
    if !done.success() {
        return Err(format!("macOS could not open {name}."));
    }
    Ok(())
}

pub fn install(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    let win = WebviewWindowBuilder::new(app, LABEL, WebviewUrl::App("nudge.html".into()))
        .title("Vspk")
        .decorations(false)
        .transparent(true)
        .always_on_top(true)
        .skip_taskbar(true)
        .focused(false)
        .focusable(false)
        .shadow(false)
        .resizable(false)
        .accept_first_mouse(true)
        .inner_size(WIDTH, 46.0)
        .visible(false)
        .build()?;
    float(&win);
    Ok(())
}
