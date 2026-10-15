use serde::Serialize;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::{Duration, Instant};
use tauri::{AppHandle, LogicalPosition, LogicalSize, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder};

pub const LABEL: &str = "notch";
pub const DOT_BOX: f64 = 24.0;
const BAR_HEIGHT: f64 = 24.0;

const HOVER: Duration = Duration::from_millis(300);
const LOOK: Duration = Duration::from_millis(50);
const LEFT_ALONE: Duration = Duration::from_millis(1200);

static ON_TRIAL: AtomicBool = AtomicBool::new(false);

pub fn keep() {
    ON_TRIAL.store(false, Ordering::Relaxed);
}

static FRAME: Mutex<Option<(f64, f64, f64, f64)>> = Mutex::new(None);

#[derive(Debug, Clone, Copy, Serialize)]
pub struct Place {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
    pub notch: f64,
}

#[cfg(target_os = "macos")]
pub fn place() -> Option<Place> {
    use objc2::MainThreadMarker;
    use objc2_app_kit::NSScreen;

    let mtm = MainThreadMarker::new()?;
    let screens = NSScreen::screens(mtm);
    let primary = screens.firstObject()?;
    let primary_top = primary.frame().size.height;
    let screen = screens
        .iter()
        .find(|s| s.safeAreaInsets().top > 0.0)
        .unwrap_or(primary);

    let frame = screen.frame();
    let top = frame.origin.y + frame.size.height;
    let inset = screen.safeAreaInsets().top;
    let (notch, height, left) = if inset > 0.0 {
        let left = screen.auxiliaryTopLeftArea().size.width;
        let right = screen.auxiliaryTopRightArea().size.width;
        let notch = (frame.size.width - left - right).max(0.0);
        (notch, inset, frame.origin.x + left - DOT_BOX)
    } else {
        let bar = top - (screen.visibleFrame().origin.y + screen.visibleFrame().size.height);
        let height = if bar > 0.0 { bar } else { BAR_HEIGHT };
        (0.0, height, frame.origin.x + (frame.size.width - DOT_BOX) / 2.0)
    };
    *FRAME.lock().unwrap() = Some((left, top - height, DOT_BOX, height));
    Some(Place {
        x: left,
        y: primary_top - top,
        width: DOT_BOX,
        height,
        notch,
    })
}

#[cfg(not(target_os = "macos"))]
pub fn place() -> Option<Place> {
    None
}

fn window(app: &AppHandle) -> Option<WebviewWindow> {
    app.get_webview_window(LABEL)
}

pub fn put(app: &AppHandle) {
    let Some(win) = window(app) else { return };
    let Some(at) = place() else { return };
    let _ = win.set_size(LogicalSize::new(at.width, at.height));
    let _ = win.set_position(LogicalPosition::new(at.x, at.y));
}

#[cfg(target_os = "macos")]
fn sit_in_the_menu_bar(win: &WebviewWindow) {
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
    ns.orderFrontRegardless();
}

#[cfg(not(target_os = "macos"))]
fn sit_in_the_menu_bar(_win: &WebviewWindow) {}

#[cfg(target_os = "macos")]
fn follow_the_screens(app: &AppHandle) {
    use block2::RcBlock;
    use objc2_app_kit::NSApplicationDidChangeScreenParametersNotification;
    use objc2_foundation::{NSNotification, NSNotificationCenter};

    let handle = app.clone();
    let block = RcBlock::new(move |_note: core::ptr::NonNull<NSNotification>| {
        put(&handle);
    });
    let observer = unsafe {
        NSNotificationCenter::defaultCenter().addObserverForName_object_queue_usingBlock(
            Some(NSApplicationDidChangeScreenParametersNotification),
            None,
            None,
            &block,
        )
    };
    std::mem::forget(observer);
}

#[cfg(not(target_os = "macos"))]
fn follow_the_screens(_app: &AppHandle) {}

#[cfg(target_os = "macos")]
fn pointer_on_pill() -> bool {
    use objc2_app_kit::NSEvent;
    let Some((x, y, w, h)) = *FRAME.lock().unwrap() else { return false };
    let at = NSEvent::mouseLocation();
    at.x >= x && at.x <= x + w && at.y >= y - 1.0 && at.y <= y + h + 1.0
}

#[cfg(not(target_os = "macos"))]
fn pointer_on_pill() -> bool {
    false
}

#[cfg(target_os = "macos")]
fn dot_in_view(app: &AppHandle) -> bool {
    use objc2_app_kit::NSWindow;

    let Some(win) = window(app) else { return false };
    let Ok(ptr) = win.ns_window() else { return false };
    if ptr.is_null() {
        return false;
    }
    let ns: &NSWindow = unsafe { &*(ptr as *const NSWindow) };
    ns.isVisible() && ns.isOnActiveSpace()
}

#[cfg(not(target_os = "macos"))]
fn dot_in_view(_app: &AppHandle) -> bool {
    false
}

fn pointer_on_panel(app: &AppHandle, win: &WebviewWindow) -> bool {
    let (Ok(at), Ok(pos), Ok(size)) = (app.cursor_position(), win.outer_position(), win.outer_size()) else {
        return true;
    };
    at.x >= pos.x as f64
        && at.x <= pos.x as f64 + size.width as f64
        && at.y >= pos.y as f64
        && at.y <= pos.y as f64 + size.height as f64
}

fn open_on_hover(app: &AppHandle) {
    let handle = app.clone();
    std::thread::spawn(move || {
        let mut since: Option<Instant> = None;
        let mut away: Option<Instant> = None;
        let mut opened = false;
        loop {
            std::thread::sleep(LOOK);
            let Some(panel) = crate::overlay::window(&handle) else { continue };
            let showing = panel.is_visible().unwrap_or(false);
            let on_pill = pointer_on_pill();

            if !showing {
                ON_TRIAL.store(false, Ordering::Relaxed);
            }
            if ON_TRIAL.load(Ordering::Relaxed) {
                if on_pill || pointer_on_panel(&handle, &panel) {
                    away = None;
                } else if away.get_or_insert_with(Instant::now).elapsed() >= LEFT_ALONE {
                    ON_TRIAL.store(false, Ordering::Relaxed);
                    away = None;
                    let main = handle.clone();
                    let _ = handle.run_on_main_thread(move || {
                        crate::overlay::hide_because(&main, "opened by hovering and then left alone")
                    });
                }
            }

            if !on_pill {
                since = None;
                opened = false;
                continue;
            }
            let entered = *since.get_or_insert_with(Instant::now);
            if opened || entered.elapsed() < HOVER {
                continue;
            }
            opened = true;
            if !showing {
                away = None;
                let main = handle.clone();
                let _ = handle.run_on_main_thread(move || {
                    if dot_in_view(&main) {
                        ON_TRIAL.store(true, Ordering::Relaxed);
                        crate::overlay::show(&main);
                    }
                });
            }
        }
    });
}

pub fn install(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    let Some(at) = place() else { return Ok(()) };
    let win = WebviewWindowBuilder::new(app, LABEL, WebviewUrl::App("notch.html".into()))
        .title("VANS")
        .decorations(false)
        .transparent(true)
        .always_on_top(true)
        .skip_taskbar(true)
        .focused(false)
        .focusable(false)
        .shadow(false)
        .resizable(false)
        .accept_first_mouse(true)
        .inner_size(at.width, at.height)
        .position(at.x, at.y)
        .visible(false)
        .build()?;
    let _ = win.show();
    sit_in_the_menu_bar(&win);
    follow_the_screens(app);
    open_on_hover(app);
    Ok(())
}
