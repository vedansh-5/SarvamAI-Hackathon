use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use serde::Serialize;
use tauri::{AppHandle, Emitter, Manager, Url};
use tauri_plugin_updater::UpdaterExt;

use crate::engine::Engine;

const FIRST_LOOK: Duration = Duration::from_secs(90);
const LOOK_EVERY: Duration = Duration::from_secs(60 * 60);
const STALE: Duration = Duration::from_secs(30 * 60);
const AWAY_FOR: f64 = 10.0 * 60.0;
const WATCH_EVERY: Duration = Duration::from_secs(60);
const PUBKEY: Option<&str> = option_env!("VANS_UPDATER_PUBKEY");

static READY: Mutex<Option<String>> = Mutex::new(None);
static LAST_LOOK: Mutex<Option<Instant>> = Mutex::new(None);
static LOOKING: AtomicBool = AtomicBool::new(false);

#[derive(Debug, Clone, Serialize)]
pub struct UpdateState {
    pub current: String,
    pub updates: bool,
    pub ready: Option<String>,
    pub looked: bool,
    pub error: Option<String>,
}

fn pubkey() -> Option<&'static str> {
    PUBKEY.filter(|k| !k.trim().is_empty())
}

pub fn state(app: &AppHandle) -> UpdateState {
    UpdateState {
        current: app.package_info().version.to_string(),
        updates: pubkey().is_some(),
        ready: READY.lock().ok().and_then(|r| r.clone()),
        looked: false,
        error: None,
    }
}

pub fn install(app: &AppHandle) {
    if pubkey().is_none() {
        return;
    }
    let looking = app.clone();
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(FIRST_LOOK).await;
        loop {
            if let Err(e) = look(&looking).await {
                eprintln!("[update] {e}");
            }
            tokio::time::sleep(LOOK_EVERY).await;
        }
    });
    let watching = app.clone();
    tauri::async_runtime::spawn(async move {
        loop {
            tokio::time::sleep(WATCH_EVERY).await;
            if READY.lock().map(|r| r.is_some()).unwrap_or(false) && away(&watching) {
                eprintln!("[update] restarting into the new version while the Mac is not in use");
                watching.restart();
            }
        }
    });
}

pub fn look_if_stale(app: &AppHandle) {
    if pubkey().is_none() || READY.lock().map(|r| r.is_some()).unwrap_or(true) {
        return;
    }
    let stale = LAST_LOOK.lock().map(|l| l.is_none_or(|at| at.elapsed() >= STALE)).unwrap_or(false);
    if !stale {
        return;
    }
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        if let Err(e) = look(&app).await {
            eprintln!("[update] {e}");
        }
    });
}

pub async fn look_now(app: &AppHandle) -> UpdateState {
    let mut out = state(app);
    if !out.updates || out.ready.is_some() {
        return out;
    }
    match look(app).await {
        Ok(()) => {
            out.looked = true;
            out.ready = READY.lock().ok().and_then(|r| r.clone());
        }
        Err(e) => out.error = Some(e),
    }
    out
}

fn away(app: &AppHandle) -> bool {
    if crate::coach::is_showing() {
        return false;
    }
    let panel_up = app
        .get_webview_window("main")
        .and_then(|w| w.is_visible().ok())
        .unwrap_or(false);
    !panel_up && idle_seconds() >= AWAY_FOR
}

#[cfg(target_os = "macos")]
fn idle_seconds() -> f64 {
    #[link(name = "CoreGraphics", kind = "framework")]
    extern "C" {
        fn CGEventSourceSecondsSinceLastEventType(state: i32, event_type: u32) -> f64;
    }
    unsafe { CGEventSourceSecondsSinceLastEventType(0, u32::MAX) }
}

#[cfg(not(target_os = "macos"))]
fn idle_seconds() -> f64 {
    0.0
}

async fn look(app: &AppHandle) -> Result<(), String> {
    let Some(pubkey) = pubkey() else {
        return Ok(());
    };
    if LOOKING.swap(true, Ordering::SeqCst) {
        return Ok(());
    }
    let result = fetch(app, pubkey).await;
    LOOKING.store(false, Ordering::SeqCst);
    if let Ok(mut at) = LAST_LOOK.lock() {
        *at = Some(Instant::now());
    }
    result
}

async fn fetch(app: &AppHandle, pubkey: &str) -> Result<(), String> {
    let engine = app.try_state::<Engine>().ok_or("no engine")?;
    let source = engine.call("update_source", None).await.map_err(|e| e.message)?;
    let server = source.get("server").and_then(|v| v.as_str()).unwrap_or_default().trim_end_matches('/');
    let code = source.get("code").and_then(|v| v.as_str()).unwrap_or_default();
    if server.is_empty() || code.is_empty() {
        return Err("VANS needs your invite code before it can look for updates".into());
    }
    let endpoint = Url::parse(&format!("{server}/v1/update/{{{{target}}}}/{{{{arch}}}}/{{{{current_version}}}}"))
        .map_err(|e| e.to_string())?;
    let updater = app
        .updater_builder()
        .pubkey(pubkey)
        .endpoints(vec![endpoint])
        .and_then(|b| b.header("Authorization", format!("Bearer {code}")))
        .and_then(|b| b.build())
        .map_err(|e| e.to_string())?;
    let Some(update) = updater.check().await.map_err(|e| e.to_string())? else {
        return Ok(());
    };
    eprintln!("[update] {} found; installing", update.version);
    update.download_and_install(|_, _| {}, || {}).await.map_err(|e| e.to_string())?;
    if let Ok(mut ready) = READY.lock() {
        *ready = Some(update.version.clone());
    }
    let _ = app.emit("vans://updated", update.version.clone());
    Ok(())
}
