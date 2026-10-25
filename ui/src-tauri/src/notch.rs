use serde::Serialize;
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::{Duration, Instant};
use tauri::{AppHandle, Emitter, LogicalPosition, LogicalSize, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder};

pub const LABEL: &str = "notch";
pub const DOT_BOX: f64 = 24.0;
const BAR_HEIGHT: f64 = 24.0;

const HOVER: Duration = Duration::from_millis(300);
const LOOK: Duration = Duration::from_millis(50);
const LEFT_ALONE: Duration = Duration::from_millis(1200);
const STAYS_BRIGHT: Duration = Duration::from_millis(1500);
const GOES_OUT: Duration = Duration::from_secs(5);

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Calm {
    Bright,
    Faded,
    Out,
}

impl Calm {
    fn word(self) -> &'static str {
        match self {
            Calm::Bright => "bright",
            Calm::Faded => "faded",
            Calm::Out => "out",
        }
    }
}

fn calm_for(since_on_pill: Option<Duration>, cursor_still_for: Duration) -> Calm {
    if since_on_pill.is_some_and(|d| d < STAYS_BRIGHT) {
        Calm::Bright
    } else if cursor_still_for >= GOES_OUT {
        Calm::Out
    } else {
        Calm::Faded
    }
}

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
fn bar_screen() -> Option<(objc2::rc::Retained<objc2_app_kit::NSScreen>, f64)> {
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
    Some((screen, primary_top))
}

#[cfg(target_os = "macos")]
pub fn under_the_bar(width: f64, gap: f64) -> Option<(f64, f64)> {
    let (screen, primary_top) = bar_screen()?;
    let frame = screen.frame();
    let top = frame.origin.y + frame.size.height;
    let inset = screen.safeAreaInsets().top;
    let visible = screen.visibleFrame();
    let bar = if inset > 0.0 { inset } else { top - (visible.origin.y + visible.size.height) };
    let bar = if bar > 0.0 { bar } else { BAR_HEIGHT };
    Some((frame.origin.x + (frame.size.width - width) / 2.0, primary_top - top + bar + gap))
}

#[cfg(not(target_os = "macos"))]
pub fn under_the_bar(_width: f64, _gap: f64) -> Option<(f64, f64)> {
    None
}

#[derive(Debug, Clone, Copy, Serialize)]
pub struct Island {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
    pub notch: f64,
    pub bar: f64,
}

pub fn island_for(screen_x: f64, screen_width: f64, left: f64, notch: f64, bar: f64, width: f64, body: f64) -> (f64, f64, f64) {
    let width = width.max(notch + 2.0 * 96.0).min(screen_width);
    let centre = if notch > 0.0 { screen_x + left + notch / 2.0 } else { screen_x + screen_width / 2.0 };
    let x = (centre - width / 2.0).clamp(screen_x, screen_x + screen_width - width);
    (x, width, bar + body)
}

#[cfg(target_os = "macos")]
pub fn island(width: f64, body: f64, gap: f64) -> Option<Island> {
    let (screen, primary_top) = bar_screen()?;
    let frame = screen.frame();
    let top = frame.origin.y + frame.size.height;
    let inset = screen.safeAreaInsets().top;
    if inset > 0.0 {
        let left = screen.auxiliaryTopLeftArea().size.width;
        let right = screen.auxiliaryTopRightArea().size.width;
        let notch = (frame.size.width - left - right).max(0.0);
        let (x, width, height) = island_for(frame.origin.x, frame.size.width, left, notch, inset, width, body);
        return Some(Island { x, y: primary_top - top, width, height, notch, bar: inset });
    }
    let (_, y) = under_the_bar(width, gap)?;
    let (x, width, height) = island_for(frame.origin.x, frame.size.width, 0.0, 0.0, 0.0, width, body);
    Some(Island { x, y, width, height, notch: 0.0, bar: 0.0 })
}

#[cfg(not(target_os = "macos"))]
pub fn island(_width: f64, _body: f64, _gap: f64) -> Option<Island> {
    None
}

#[cfg(target_os = "macos")]
pub fn place() -> Option<Place> {
    let (screen, primary_top) = bar_screen()?;

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
pub fn sit_in_the_menu_bar(win: &WebviewWindow) {
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
pub fn sit_in_the_menu_bar(_win: &WebviewWindow) {}

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
fn pointer() -> Option<(f64, f64)> {
    let at = objc2_app_kit::NSEvent::mouseLocation();
    Some((at.x, at.y))
}

#[cfg(not(target_os = "macos"))]
fn pointer() -> Option<(f64, f64)> {
    None
}

fn pointer_on_pill() -> bool {
    let Some((x, y, w, h)) = *FRAME.lock().unwrap() else { return false };
    let Some(at) = pointer() else { return false };
    at.0 >= x && at.0 <= x + w && at.1 >= y - 1.0 && at.1 <= y + h + 1.0
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
        let mut last_at = pointer();
        let mut last_moved = Instant::now();
        let mut last_on_pill: Option<Instant> = None;
        let mut calm: Option<Calm> = None;
        loop {
            std::thread::sleep(LOOK);
            let Some(panel) = crate::overlay::window(&handle) else { continue };
            let showing = panel.is_visible().unwrap_or(false);
            let on_pill = pointer_on_pill();

            let at = pointer();
            if at != last_at {
                last_at = at;
                last_moved = Instant::now();
            }
            if on_pill {
                last_on_pill = Some(Instant::now());
            }
            let now = calm_for(last_on_pill.map(|t| t.elapsed()), last_moved.elapsed());
            if calm != Some(now) {
                calm = Some(now);
                let _ = handle.emit_to(LABEL, "pill://calm", now.word());
            }

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

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_dot_is_bright_while_the_pointer_is_on_it_and_for_a_moment_after() {
        assert_eq!(calm_for(Some(Duration::ZERO), Duration::from_secs(60)), Calm::Bright);
        assert_eq!(calm_for(Some(Duration::from_millis(1400)), Duration::ZERO), Calm::Bright);
    }

    #[test]
    fn the_dot_fades_after_the_pointer_leaves_and_comes_back_faded_when_it_moves() {
        assert_eq!(calm_for(Some(Duration::from_millis(1600)), Duration::ZERO), Calm::Faded);
        assert_eq!(calm_for(None, Duration::from_secs(1)), Calm::Faded);
        assert_eq!(calm_for(None, Duration::ZERO), Calm::Faded);
    }

    #[test]
    fn the_island_is_centred_on_the_notch_and_stays_on_the_screen() {
        let (x, width, height) = island_for(0.0, 1512.0, 650.0, 212.0, 32.0, 460.0, 70.0);
        assert_eq!(width, 460.0);
        assert_eq!(x + width / 2.0, 650.0 + 106.0);
        assert_eq!(height, 102.0);
        let (_, wide, _) = island_for(0.0, 1512.0, 600.0, 400.0, 32.0, 460.0, 70.0);
        assert_eq!(wide, 400.0 + 192.0);
        let (x, width, _) = island_for(1512.0, 1920.0, 0.0, 0.0, 0.0, 460.0, 70.0);
        assert_eq!(x, 1512.0 + (1920.0 - 460.0) / 2.0);
        assert_eq!(width, 460.0);
        let (x, _, _) = island_for(0.0, 300.0, 0.0, 0.0, 0.0, 460.0, 70.0);
        assert_eq!(x, 0.0);
    }

    #[test]
    fn the_dot_goes_out_when_the_pointer_has_been_still_for_a_while() {
        assert_eq!(calm_for(None, Duration::from_secs(5)), Calm::Out);
        assert_eq!(calm_for(Some(Duration::from_secs(30)), Duration::from_secs(30)), Calm::Out);
    }
}
