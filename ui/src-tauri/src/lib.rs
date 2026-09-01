mod engine;
mod overlay;
pub mod screen;

use std::path::PathBuf;

use engine::{Engine, RpcError};
use screen::ScreenContext;
use serde_json::{json, Value};
use tauri::{Manager, State};

/// Returns the current active application and screen context.
#[tauri::command]
fn get_screen_context(include_screenshot: Option<bool>) -> ScreenContext {
    screen::capture_active_context(include_screenshot.unwrap_or(false))
}

/// Checks whether Screen Recording permission is granted.
#[tauri::command]
fn check_screen_recording() -> bool {
    screen::has_screen_recording_permission()
}

/// Prompts the user for Screen Recording permission.
#[tauri::command]
fn request_screen_recording() -> bool {
    screen::request_screen_recording_permission()
}

/// Commands are thin on purpose. Every decision the runtime makes — what to
/// extract, whether it verified, whether a plan may be committed — stays in
/// the runtime, so the window cannot talk it into anything by calling in a
/// different order.
#[tauri::command]
async fn preflight(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("preflight", None).await
}

/// Sends an Apple event to Notes, which launches it and may raise the
/// automation prompt. Only ever called from a button that says so (D-037).
#[tauri::command]
async fn check_notes(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("check_notes", None).await
}

/// Extracts and verifies. Touches nothing outside the browser.
#[tauri::command]
async fn prepare(
    engine: State<'_, Engine>,
    url: String,
    intent: String,
    title: String,
) -> Result<Value, RpcError> {
    engine
        .call("prepare", Some(json!({ "url": url, "intent": intent, "title": title })))
        .await
}

/// Confirms a plan. The only command with a side effect, and it takes an id
/// the window cannot invent — the runtime issued it, and issues it once.
#[tauri::command]
async fn commit(engine: State<'_, Engine>, plan_id: String) -> Result<Value, RpcError> {
    engine
        .call("commit", Some(json!({ "plan_id": plan_id })))
        .await
}

/// Dismisses the panel. Escape and Cancel both route here rather than closing
/// the window, which would destroy state the user may come back to.
#[tauri::command]
fn dismiss(app: tauri::AppHandle) {
    overlay::hide(&app);
}

/// Asks the panel to stay up when it loses focus.
///
/// The window knows which screen it is showing; the shell knows what a panel
/// is. This is the one bit the window has to tell the shell, and it is a
/// presentation request only — it cannot reach the runtime or a plan.
#[tauri::command]
fn set_pinned(pinned: bool) {
    overlay::set_pinned(pinned);
}

/// Reports how tall the content is so the window can match it.
#[tauri::command]
fn resize_panel(app: tauri::AppHandle, height: f64) {
    overlay::resize(&app, height);
}

/// Applies the chosen theme to the window and to the blur behind it.
///
/// The frontend owns the choice — it is the side that persists it and draws
/// it — but the blur is an AppKit layer the frontend cannot reach, so the
/// choice has to be told to the shell as well as to the stylesheet.
#[tauri::command]
fn set_theme(app: tauri::AppHandle, dark: bool) {
    overlay::set_theme(&app, dark);
}

/// Finds the runtime binary.
///
/// The bundled copy is preferred so a shipped app never depends on a checkout.
/// The environment variable is for development, and the sibling path is the
/// convenience case of running from the repo.
fn engine_path(app: &tauri::AppHandle) -> PathBuf {
    if let Ok(explicit) = std::env::var("VANS_ENGINE_BIN") {
        let p = PathBuf::from(explicit);
        if p.exists() {
            return p;
        }
    }
    if let Ok(dir) = app.path().resource_dir() {
        let bundled = dir.join("vans-engine");
        if bundled.exists() {
            return bundled;
        }
    }
    PathBuf::from("../vans-engine")
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .setup(|app| {
            let handle = app.handle().clone();
            let handle_for_overlay = app.handle().clone();
            let path = engine_path(&handle);

            let engine = Engine::spawn(handle, &path).map_err(|e| {
                format!(
                    "Could not start the VANS runtime at {}: {e}\n\n\
                     Build it with:  go build -o ui/vans-engine ./cmd/vans-engine",
                    path.display()
                )
            })?;
            app.manage(engine);

            overlay::install(&handle_for_overlay)?;
            Ok(())
        })
        .plugin(tauri_plugin_global_shortcut::Builder::new().build())
        .invoke_handler(tauri::generate_handler![
            preflight,
            check_notes,
            prepare,
            commit,
            dismiss,
            set_pinned,
            resize_panel,
            set_theme,
            get_screen_context,
            check_screen_recording,
            request_screen_recording
        ])
        .run(tauri::generate_context!())
        .expect("error while running VANS");
}
