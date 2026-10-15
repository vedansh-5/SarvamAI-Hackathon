use tauri::{AppHandle, Manager};

use crate::engine::Engine;

fn tell(app: &AppHandle, method: &'static str, params: serde_json::Value) {
    let h = app.clone();
    tauri::async_runtime::spawn(async move {
        let engine = h.state::<Engine>();
        let _ = engine.call(method, Some(params)).await;
    });
}

#[cfg(target_os = "macos")]
fn moments(app: &AppHandle) {
    use block2::RcBlock;
    use objc2_app_kit::{
        NSWorkspace, NSWorkspaceDidActivateApplicationNotification, NSWorkspaceDidWakeNotification,
        NSWorkspaceWillSleepNotification,
    };
    use objc2_foundation::{NSDistributedNotificationCenter, NSNotification, NSString};

    let workspace = NSWorkspace::sharedWorkspace();
    let center = workspace.notificationCenter();

    let handle = app.clone();
    let front = RcBlock::new(move |_note: core::ptr::NonNull<NSNotification>| {
        let name = NSWorkspace::sharedWorkspace()
            .frontmostApplication()
            .and_then(|a| a.localizedName())
            .map(|n| n.to_string())
            .unwrap_or_default();
        if !name.is_empty() {
            tell(&handle, "moment", serde_json::json!({ "kind": "front", "app": name }));
        }
    });
    let observer = unsafe {
        center.addObserverForName_object_queue_usingBlock(
            Some(NSWorkspaceDidActivateApplicationNotification),
            None,
            None,
            &front,
        )
    };
    std::mem::forget(observer);

    let mut on: Vec<(&NSString, &'static str)> = Vec::new();
    unsafe {
        on.push((NSWorkspaceDidWakeNotification, "wake"));
        on.push((NSWorkspaceWillSleepNotification, "sleep"));
    }
    for (name, kind) in on {
        let handle = app.clone();
        let block = RcBlock::new(move |_note: core::ptr::NonNull<NSNotification>| {
            if kind == "wake" {
                crate::nudge::woke();
            }
            tell(&handle, "moment", serde_json::json!({ "kind": kind }));
        });
        let observer = unsafe { center.addObserverForName_object_queue_usingBlock(Some(name), None, None, &block) };
        std::mem::forget(observer);
    }

    let distributed = NSDistributedNotificationCenter::defaultCenter();
    for (name, kind) in [("com.apple.screenIsLocked", "lock"), ("com.apple.screenIsUnlocked", "unlock")] {
        let handle = app.clone();
        let block = RcBlock::new(move |_note: core::ptr::NonNull<NSNotification>| {
            if kind == "unlock" {
                crate::nudge::woke();
            }
            tell(&handle, "moment", serde_json::json!({ "kind": kind }));
        });
        let name = NSString::from_str(name);
        let observer =
            unsafe { distributed.addObserverForName_object_queue_usingBlock(Some(&name), None, None, &block) };
        std::mem::forget(observer);
    }
}

#[cfg(not(target_os = "macos"))]
fn moments(_app: &AppHandle) {}

pub fn install(app: &AppHandle) {
    moments(app);
    #[cfg(target_os = "macos")]
    {
        use block2::RcBlock;
        use objc2_app_kit::{NSEvent, NSEventMask, NSScreen};

        let handle = app.clone();
        let block = RcBlock::new(move |_event: core::ptr::NonNull<NSEvent>| {
            let at = NSEvent::mouseLocation();
            let top = unsafe {
                NSScreen::screens(objc2_foundation::MainThreadMarker::new_unchecked())
                    .firstObject()
                    .map(|s| s.frame().size.height)
                    .unwrap_or(0.0)
            };
            let (x, y) = (at.x.round() as i64, (top - at.y).round() as i64);
            tell(&handle, "noticed", serde_json::json!({ "x": x, "y": y }));
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
