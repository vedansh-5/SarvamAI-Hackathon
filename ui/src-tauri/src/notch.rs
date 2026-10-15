use serde::Serialize;
use tauri::{AppHandle, Emitter, LogicalPosition, LogicalSize, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder};

pub const LABEL: &str = "notch";
const WING: f64 = 56.0;
const BARE_WIDTH: f64 = 120.0;
const BAR_HEIGHT: f64 = 24.0;

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
    let (notch, height) = if inset > 0.0 {
        let left = screen.auxiliaryTopLeftArea().size.width;
        let right = screen.auxiliaryTopRightArea().size.width;
        ((frame.size.width - left - right).max(0.0), inset)
    } else {
        let bar = top - (screen.visibleFrame().origin.y + screen.visibleFrame().size.height);
        (0.0, if bar > 0.0 { bar } else { BAR_HEIGHT })
    };
    let width = if notch > 0.0 { notch + 2.0 * WING } else { BARE_WIDTH };
    Some(Place {
        x: frame.origin.x + (frame.size.width - width) / 2.0,
        y: primary_top - top,
        width,
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
    let _ = win.emit("notch://place", at);
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
    Ok(())
}
