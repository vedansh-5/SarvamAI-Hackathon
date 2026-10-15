use tauri::{AppHandle, Manager};

use crate::engine::Engine;

pub fn install(app: &AppHandle) {
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
            let h = handle.clone();
            tauri::async_runtime::spawn(async move {
                let engine = h.state::<Engine>();
                let _ = engine
                    .call("noticed", Some(serde_json::json!({ "x": x, "y": y })))
                    .await;
            });
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
