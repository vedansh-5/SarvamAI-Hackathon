mod engine;
mod overlay;

use std::path::PathBuf;

use engine::{Engine, RpcError};
use serde_json::{json, Value};
use tauri::{Manager, State};

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
            dismiss
        ])
        .run(tauri::generate_context!())
        .expect("error while running VANS");
}
