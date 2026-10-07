//! Screen and active application context.
//!
//! Captures the frontmost application, window title, display geometry,
//! and on-demand screenshot buffers while keeping token overhead minimal.

use serde::{Deserialize, Serialize};
use std::process::Command;

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct Rect {
    pub x: f64,
    pub y: f64,
    pub width: f64,
    pub height: f64,
}

impl Default for Rect {
    fn default() -> Self {
        Self {
            x: 0.0,
            y: 0.0,
            width: 1920.0,
            height: 1080.0,
        }
    }
}

#[derive(Debug, Clone, Serialize, Deserialize)]
pub struct ScreenContext {
    pub app_name: String,
    pub bundle_id: String,
    pub window_title: String,
    pub pid: i32,
    pub bounds: Rect,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub frame_path: Option<String>,
    #[serde(skip_serializing_if = "Option::is_none")]
    pub image_base64: Option<String>,
}

#[cfg(target_os = "macos")]
#[link(name = "CoreGraphics", kind = "framework")]
extern "C" {
    fn CGPreflightScreenCaptureAccess() -> bool;
    fn CGRequestScreenCaptureAccess() -> bool;
}

/// Reports whether macOS Screen Recording permission has been granted.
pub fn has_screen_recording_permission() -> bool {
    #[cfg(target_os = "macos")]
    unsafe {
        CGPreflightScreenCaptureAccess()
    }
    #[cfg(not(target_os = "macos"))]
    true
}

/// Triggers the macOS system prompt asking for Screen Recording permission.
pub fn request_screen_recording_permission() -> bool {
    #[cfg(target_os = "macos")]
    unsafe {
        CGRequestScreenCaptureAccess()
    }
    #[cfg(not(target_os = "macos"))]
    true
}

/// Inspects the frontmost application and active window on the machine.
pub fn capture_active_context(include_screenshot: bool) -> ScreenContext {
    let (app_name, bundle_id, window_title, pid) = get_frontmost_app_info();

    let mut context = ScreenContext {
        app_name,
        bundle_id,
        window_title,
        pid,
        bounds: Rect::default(),
        frame_path: None,
        image_base64: None,
    };

    if include_screenshot && has_screen_recording_permission() {
        if let Ok(path) = capture_display_to_temp() {
            context.frame_path = Some(path);
        }
    }

    context
}

/// Captures the display to a temporary PNG file.
pub fn capture_display_to_temp() -> Result<String, String> {
    let tmp_path = std::env::temp_dir().join("vans_screen_context.png");
    let tmp_str = tmp_path.to_string_lossy().to_string();

    #[cfg(target_os = "macos")]
    {
        let status = Command::new("/usr/sbin/screencapture")
            .args(["-x", &tmp_str])
            .status()
            .map_err(|e| format!("failed to run screencapture: {e}"))?;

        if !status.success() {
            return Err("screencapture exited with non-zero status".into());
        }
        return Ok(tmp_str);
    }

    #[cfg(not(target_os = "macos"))]
    Err("screen capture not supported on this OS".into())
}

/// The name of the application in front, or None when it cannot be read.
///
/// Costs no permission — AppKit answers this directly. Cheap enough to ask
/// several times a second, which is what the coach does to notice that the person
/// has gone somewhere else.
#[cfg(target_os = "macos")]
pub fn frontmost_app_name() -> Option<String> {
    use objc2_app_kit::NSWorkspace;

    let workspace = NSWorkspace::sharedWorkspace();
    workspace
        .frontmostApplication()
        .and_then(|app| app.localizedName())
        .map(|name| clean_name(&name.to_string()))
}

pub fn clean_name(name: &str) -> String {
    name.chars()
        .filter(|c| {
            !matches!(*c,
                '\u{00AD}' | '\u{200B}'..='\u{200F}' | '\u{202A}'..='\u{202E}'
                | '\u{2060}'..='\u{2064}' | '\u{FEFF}')
                && !c.is_control()
        })
        .collect::<String>()
        .trim()
        .to_string()
}

#[cfg(not(target_os = "macos"))]
pub fn frontmost_app_name() -> Option<String> {
    None
}

/// Whether the application in front has a window on the desktop being looked at.
///
/// The name of the application in front is not enough to say the person is
/// still looking at it. Swiping to a desktop with nothing on it leaves the same
/// application "in front" with every one of its windows somewhere else. The
/// on-screen window list only holds what is on the current desktop, and who
/// owns a window needs no permission to read.
///
/// None means the list could not be read, which is not the same as no.
#[cfg(target_os = "macos")]
pub fn front_app_is_on_this_desktop() -> Option<bool> {
    use objc2_app_kit::NSWorkspace;
    use objc2_core_foundation::{CFArray, CFDictionary, CFNumber, CFString, CFType};
    use objc2_core_graphics::{
        kCGWindowLayer, kCGWindowOwnerPID, CGWindowListCopyWindowInfo, CGWindowListOption,
    };

    let pid = NSWorkspace::sharedWorkspace()
        .frontmostApplication()
        .map(|app| app.processIdentifier())?;

    type Entry = CFDictionary<CFString, CFType>;

    let options =
        CGWindowListOption::OptionOnScreenOnly | CGWindowListOption::ExcludeDesktopElements;
    let windows = CGWindowListCopyWindowInfo(options, 0)?;
    let entries: &CFArray<Entry> =
        unsafe { &*((&*windows) as *const CFArray as *const CFArray<Entry>) };

    let number = |entry: &Entry, key: &CFString| -> Option<i32> {
        entry
            .get(key)
            .and_then(|v| v.downcast::<CFNumber>().ok())
            .and_then(|n| n.as_i32())
    };

    for i in 0..entries.len() {
        let Some(entry) = entries.get(i) else { continue };
        if number(&entry, unsafe { kCGWindowLayer }) != Some(0) {
            continue;
        }
        if number(&entry, unsafe { kCGWindowOwnerPID }) == Some(pid) {
            return Some(true);
        }
    }
    Some(false)
}

#[cfg(not(target_os = "macos"))]
pub fn front_app_is_on_this_desktop() -> Option<bool> {
    None
}

/// Whether the application in front is VANS itself.
///
/// The panel is meant to be non-activating, but typing into it can still make this
/// process the active one — seen live, with the panel's own header reading
/// "Active: vans". Anything deciding what the person is looking at has to treat
/// that as "still here", not "gone somewhere else".
#[cfg(target_os = "macos")]
pub fn vans_is_in_front() -> bool {
    use objc2_app_kit::NSWorkspace;

    NSWorkspace::sharedWorkspace()
        .frontmostApplication()
        .map(|app| app.processIdentifier() as u32 == std::process::id())
        .unwrap_or(false)
}

#[cfg(not(target_os = "macos"))]
pub fn vans_is_in_front() -> bool {
    false
}

#[cfg(target_os = "macos")]
fn get_frontmost_app_info() -> (String, String, String, i32) {
    use objc2_app_kit::NSWorkspace;

    // Application identity comes from AppKit directly and needs no permission
    // at all. The previous implementation asked System Events over osascript,
    // which needs an Accessibility grant the user has not given and costs
    // 50-100ms on a path that runs every single time the panel is summoned.
    let (app_name, bundle_id, pid) = {
        let workspace = NSWorkspace::sharedWorkspace();
        match workspace.frontmostApplication() {
            Some(app) => {
                let name = app
                    .localizedName()
                    .map(|s| s.to_string())
                    .unwrap_or_else(|| "Unknown".to_string());
                let bundle = app
                    .bundleIdentifier()
                    .map(|s| s.to_string())
                    .unwrap_or_default();
                (name, bundle, app.processIdentifier())
            }
            None => ("Unknown".to_string(), String::new(), 0),
        }
    };

    (app_name, bundle_id, frontmost_window_title(pid), pid)
}

/// The title of the frontmost window belonging to `pid`.
///
/// Unlike the application's identity, a window title is private: macOS gates
/// it behind Screen Recording (via CGWindowList) or Accessibility (via the AX
/// API). Neither is required to run VANS, so a missing title degrades to an
/// empty string rather than failing — the caller still knows which application
/// is in front, which is the part that matters most.
#[cfg(target_os = "macos")]
fn frontmost_window_title(pid: i32) -> String {
    use objc2_core_foundation::{CFArray, CFDictionary, CFNumber, CFString, CFType};
    use objc2_core_graphics::{
        kCGWindowLayer, kCGWindowName, kCGWindowOwnerPID, CGWindowListCopyWindowInfo,
        CGWindowListOption,
    };

    if pid == 0 || !has_screen_recording_permission() {
        return String::new();
    }

    type Entry = CFDictionary<CFString, CFType>;

    let options =
        CGWindowListOption::OptionOnScreenOnly | CGWindowListOption::ExcludeDesktopElements;
    let Some(windows) = CGWindowListCopyWindowInfo(options, 0) else {
        return String::new();
    };

    // CGWindowListCopyWindowInfo is declared as an untyped CFArray. Its
    // contents are documented to be window-info dictionaries keyed by CFString.
    let entries: &CFArray<Entry> =
        unsafe { &*((&*windows) as *const CFArray as *const CFArray<Entry>) };

    let number = |entry: &Entry, key: &CFString| -> Option<i32> {
        entry
            .get(key)
            .and_then(|v| v.downcast::<CFNumber>().ok())
            .and_then(|n| n.as_i32())
    };

    for i in 0..entries.len() {
        let Some(entry) = entries.get(i) else { continue };

        // Layer 0 is an ordinary window. Anything above it is a panel, menu or
        // overlay — including our own.
        if number(&entry, unsafe { kCGWindowLayer }) != Some(0) {
            continue;
        }
        if number(&entry, unsafe { kCGWindowOwnerPID }) != Some(pid) {
            continue;
        }

        if let Some(name) = entry
            .get(unsafe { kCGWindowName })
            .and_then(|v| v.downcast::<CFString>().ok())
        {
            return name.to_string();
        }
    }

    String::new()
}

#[cfg(not(target_os = "macos"))]
fn get_frontmost_app_info() -> (String, String, String, i32) {
    ("Unknown".into(), "".into(), "".into(), 0)
}

#[cfg(test)]
mod tests {
    use super::*;

    /// Exercises the real AppKit lookup. Application identity must come back
    /// without any permission being granted — that is the whole point of not
    /// going through System Events.
    #[test]
    fn frontmost_app_is_identified_without_permission() {
        let ctx = capture_active_context(false);
        eprintln!(
            "app={:?} bundle={:?} pid={} title={:?} screen_recording={}",
            ctx.app_name,
            ctx.bundle_id,
            ctx.pid,
            ctx.window_title,
            has_screen_recording_permission()
        );
        assert!(ctx.pid > 0, "no frontmost pid; AppKit lookup returned nothing");
        assert_ne!(ctx.app_name, "Unknown", "frontmost application not identified");
    }
}
