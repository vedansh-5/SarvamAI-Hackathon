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

#[cfg(target_os = "macos")]
fn get_frontmost_app_info() -> (String, String, String, i32) {
    // Queries System Events for the active frontmost process and window title.
    let script = r#"
        tell application "System Events"
            set frontProc to first application process whose frontmost is true
            set procName to name of frontProc
            set bId to bundle identifier of frontProc
            set pId to unix id of frontProc
            set winTitle to ""
            try
                set winTitle to name of front window of frontProc
            end try
            return procName & "|||" & bId & "|||" & (pId as string) & "|||" & winTitle
        end tell
    "#;

    let output = Command::new("osascript")
        .args(["-e", script])
        .output();

    if let Ok(out) = output {
        if out.status.success() {
            let text = String::from_utf8_lossy(&out.stdout).trim().to_string();
            let parts: Vec<&str> = text.split("|||").collect();
            if parts.len() >= 4 {
                let app_name = parts[0].trim().to_string();
                let bundle_id = parts[1].trim().to_string();
                let pid: i32 = parts[2].trim().parse().unwrap_or(0);
                let window_title = parts[3].trim().to_string();
                return (app_name, bundle_id, window_title, pid);
            }
        }
    }

    ("Unknown".into(), "".into(), "".into(), 0)
}

#[cfg(not(target_os = "macos"))]
fn get_frontmost_app_info() -> (String, String, String, i32) {
    ("Unknown".into(), "".into(), "".into(), 0)
}
