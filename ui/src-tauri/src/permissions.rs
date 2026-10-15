use serde::Serialize;
use std::collections::HashMap;
use std::process::Command;
use std::thread::sleep;
use std::time::{Duration, Instant};

#[derive(Debug, Clone, Serialize)]
pub struct Permission {
    pub name: String,
    pub label: String,
    pub why: String,
    pub state: &'static str,
    pub settings: &'static str,
}

const GRANTED: &str = "granted";
const NOT_ASKED: &str = "not_asked";
const NEEDED: &str = "needed";
const UNKNOWN: &str = "unknown";

const ACCESSIBILITY: &str = "accessibility";
const SCREEN: &str = "screen";

struct Target {
    bundle: &'static str,
    label: &'static str,
    why: &'static str,
}

const TARGETS: [Target; 5] = [
    Target {
        bundle: "com.apple.systemevents",
        label: "System Events",
        why: "To find windows and change the Mac settings you ask about.",
    },
    Target {
        bundle: "com.apple.finder",
        label: "Finder",
        why: "To see which files you have selected.",
    },
    Target {
        bundle: "com.apple.Safari",
        label: "Safari",
        why: "To guide you on websites in Safari.",
    },
    Target {
        bundle: "com.google.Chrome",
        label: "Google Chrome",
        why: "To guide you on websites in Chrome.",
    },
    Target {
        bundle: "com.apple.Notes",
        label: "Notes",
        why: "To save answers to Notes when you ask.",
    },
];

fn pane(name: &str) -> &'static str {
    match name {
        ACCESSIBILITY => "Privacy_Accessibility",
        SCREEN => "Privacy_ScreenCapture",
        _ => "Privacy_Automation",
    }
}

pub fn open_settings(name: &str) {
    let url = format!(
        "x-apple.systempreferences:com.apple.preference.security?{}",
        pane(name)
    );
    let _ = Command::new("open").arg(url).status();
}

#[cfg(target_os = "macos")]
mod mac {
    use std::ffi::c_void;

    #[repr(C)]
    pub struct AEDesc {
        descriptor_type: u32,
        data_handle: *mut c_void,
    }

    const TYPE_APPLICATION_BUNDLE_ID: u32 = u32::from_be_bytes(*b"bund");
    const TYPE_WILD_CARD: u32 = u32::from_be_bytes(*b"****");

    pub const NO_ERR: i32 = 0;
    pub const NEEDS_CONSENT: i32 = -1744;
    pub const NOT_PERMITTED: i32 = -1743;

    #[link(name = "CoreServices", kind = "framework")]
    extern "C" {
        fn AECreateDesc(type_code: u32, data: *const c_void, size: isize, out: *mut AEDesc) -> i16;
        fn AEDisposeDesc(desc: *mut AEDesc) -> i16;
        fn AEDeterminePermissionToAutomateTarget(
            target: *const AEDesc,
            event_class: u32,
            event_id: u32,
            ask: u8,
        ) -> i32;
    }

    #[link(name = "ApplicationServices", kind = "framework")]
    extern "C" {
        fn AXIsProcessTrusted() -> u8;
        fn AXIsProcessTrustedWithOptions(options: *const c_void) -> u8;
        static kAXTrustedCheckOptionPrompt: *const c_void;
    }

    #[link(name = "CoreFoundation", kind = "framework")]
    extern "C" {
        static kCFBooleanTrue: *const c_void;
        static kCFTypeDictionaryKeyCallBacks: u8;
        static kCFTypeDictionaryValueCallBacks: u8;
        fn CFDictionaryCreate(
            allocator: *const c_void,
            keys: *const *const c_void,
            values: *const *const c_void,
            count: isize,
            key_callbacks: *const c_void,
            value_callbacks: *const c_void,
        ) -> *const c_void;
        fn CFRelease(cf: *const c_void);
    }

    #[link(name = "CoreGraphics", kind = "framework")]
    extern "C" {
        fn CGPreflightScreenCaptureAccess() -> bool;
        fn CGRequestScreenCaptureAccess() -> bool;
    }

    pub fn trusted() -> bool {
        unsafe { AXIsProcessTrusted() != 0 }
    }

    pub fn ask_trust() {
        unsafe {
            let keys = [kAXTrustedCheckOptionPrompt];
            let values = [kCFBooleanTrue];
            let options = CFDictionaryCreate(
                std::ptr::null(),
                keys.as_ptr(),
                values.as_ptr(),
                1,
                &kCFTypeDictionaryKeyCallBacks as *const u8 as *const c_void,
                &kCFTypeDictionaryValueCallBacks as *const u8 as *const c_void,
            );
            AXIsProcessTrustedWithOptions(options);
            if !options.is_null() {
                CFRelease(options);
            }
        }
    }

    pub fn screen_granted() -> bool {
        unsafe { CGPreflightScreenCaptureAccess() }
    }

    pub fn ask_screen() {
        unsafe {
            CGRequestScreenCaptureAccess();
        }
    }

    pub fn automation(bundle: &str, ask: bool) -> i32 {
        unsafe {
            let mut desc = AEDesc {
                descriptor_type: 0,
                data_handle: std::ptr::null_mut(),
            };
            let made = AECreateDesc(
                TYPE_APPLICATION_BUNDLE_ID,
                bundle.as_ptr() as *const c_void,
                bundle.len() as isize,
                &mut desc,
            );
            if made != 0 {
                return made as i32;
            }
            let status =
                AEDeterminePermissionToAutomateTarget(&desc, TYPE_WILD_CARD, TYPE_WILD_CARD, ask as u8);
            AEDisposeDesc(&mut desc);
            status
        }
    }

    pub fn installed(bundle: &str) -> bool {
        use objc2_app_kit::NSWorkspace;
        use objc2_foundation::NSString;
        NSWorkspace::sharedWorkspace()
            .URLForApplicationWithBundleIdentifier(&NSString::from_str(bundle))
            .is_some()
    }

    pub fn running(bundle: &str) -> bool {
        use objc2_app_kit::NSRunningApplication;
        use objc2_foundation::NSString;
        NSRunningApplication::runningApplicationsWithBundleIdentifier(&NSString::from_str(bundle))
            .count()
            > 0
    }

    pub fn quit(bundle: &str) {
        use objc2_app_kit::NSRunningApplication;
        use objc2_foundation::NSString;
        for app in NSRunningApplication::runningApplicationsWithBundleIdentifier(&NSString::from_str(bundle)) {
            app.terminate();
        }
    }
}

#[cfg(target_os = "macos")]
fn state_of(status: i32) -> &'static str {
    match status {
        mac::NO_ERR => GRANTED,
        mac::NEEDS_CONSENT => NOT_ASKED,
        mac::NOT_PERMITTED => NEEDED,
        _ => UNKNOWN,
    }
}

#[cfg(target_os = "macos")]
fn automation_state(target: &Target, seen: &HashMap<&str, &'static str>) -> &'static str {
    if mac::running(target.bundle) {
        return state_of(mac::automation(target.bundle, false));
    }
    seen.get(target.bundle).copied().unwrap_or(UNKNOWN)
}

#[cfg(target_os = "macos")]
pub fn check() -> Vec<Permission> {
    check_with(&HashMap::new())
}

#[cfg(target_os = "macos")]
fn check_with(seen: &HashMap<&str, &'static str>) -> Vec<Permission> {
    let mut out = vec![
        Permission {
            name: ACCESSIBILITY.into(),
            label: "Accessibility".into(),
            why: "To see the buttons in other apps and point at the right one.".into(),
            state: if mac::trusted() { GRANTED } else { NEEDED },
            settings: pane(ACCESSIBILITY),
        },
        Permission {
            name: SCREEN.into(),
            label: "Screen Recording".into(),
            why: "To read window titles, and to look at apps that do not describe their buttons.".into(),
            state: if mac::screen_granted() { GRANTED } else { NEEDED },
            settings: pane(SCREEN),
        },
    ];
    for target in TARGETS.iter().filter(|t| mac::installed(t.bundle)) {
        out.push(Permission {
            name: target.bundle.into(),
            label: format!("Control {}", target.label),
            why: target.why.into(),
            state: automation_state(target, seen),
            settings: pane(target.bundle),
        });
    }
    out
}

#[cfg(target_os = "macos")]
fn wait_until_running(bundle: &str) -> bool {
    let started = Instant::now();
    while started.elapsed() < Duration::from_secs(8) {
        if mac::running(bundle) {
            sleep(Duration::from_millis(400));
            return true;
        }
        sleep(Duration::from_millis(150));
    }
    false
}

#[cfg(target_os = "macos")]
pub fn ask_all() -> Vec<Permission> {
    let mut seen = HashMap::new();
    for target in TARGETS.iter().filter(|t| mac::installed(t.bundle)) {
        let was_running = mac::running(target.bundle);
        if was_running && mac::automation(target.bundle, false) != mac::NEEDS_CONSENT {
            continue;
        }
        if !was_running {
            let _ = Command::new("open").args(["-g", "-j", "-b", target.bundle]).status();
            if !wait_until_running(target.bundle) {
                continue;
            }
        }
        seen.insert(target.bundle, state_of(mac::automation(target.bundle, true)));
        if !was_running {
            mac::quit(target.bundle);
        }
    }
    if !mac::trusted() {
        mac::ask_trust();
    }
    if !mac::screen_granted() {
        mac::ask_screen();
    }
    check_with(&seen)
}

#[cfg(not(target_os = "macos"))]
pub fn check() -> Vec<Permission> {
    Vec::new()
}

#[cfg(not(target_os = "macos"))]
pub fn ask_all() -> Vec<Permission> {
    Vec::new()
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn checking_lists_every_permission_without_asking() {
        let found = check();
        for p in &found {
            println!("{} {} {}", p.name, p.state, p.settings);
        }
        if cfg!(target_os = "macos") {
            assert!(found.iter().any(|p| p.name == ACCESSIBILITY));
            assert!(found.iter().any(|p| p.name == SCREEN));
            assert!(found.iter().any(|p| p.name == "com.apple.finder"));
        }
        for p in &found {
            assert!([GRANTED, NOT_ASKED, NEEDED, UNKNOWN].contains(&p.state));
        }
    }
}
