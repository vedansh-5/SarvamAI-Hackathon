mod coach;
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

/// Works out what an instruction means and prepares it. Touches nothing
/// outside the browser — the side effect, if the destination has one, waits
/// for commit.
#[tauri::command]
async fn prepare(
    engine: State<'_, Engine>,
    instruction: String,
    destination: String,
    context: Option<Value>,
) -> Result<Value, RpcError> {
    let mut params = json!({ "instruction": instruction, "destination": destination });
    if let Some(ctx) = context {
        params["context"] = ctx;
    }
    engine.call("prepare", Some(params)).await
}

#[tauri::command]
async fn guide(engine: State<'_, Engine>, plan_id: String) -> Result<Value, RpcError> {
    engine
        .call("guide", Some(serde_json::json!({ "plan_id": plan_id })))
        .await
}

#[tauri::command]
async fn stop_guide(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("stop_guide", None).await
}

#[tauri::command]
async fn advance_guide(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("advance_guide", None).await
}

#[tauri::command]
fn point_at(app: tauri::AppHandle, x: f64, y: f64, w: f64, h: f64, label: String, step: String) {

    coach::point_at(&app, coach::Pointer { x, y, w, h, label, step });
}

#[tauri::command]
fn clear_pointer(app: tauri::AppHandle) {
    coach::clear(&app);
}

#[tauri::command]
async fn clear_style(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("clear_style", None).await
}

#[tauri::command]
async fn destinations(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("destinations", None).await
}

/// The things worth asking for, filtered by what this build can carry out and
/// led by whatever the user is looking at.
#[tauri::command]
async fn suggestions(
    engine: State<'_, Engine>,
    app_name: Option<String>,
) -> Result<Value, RpcError> {
    engine
        .call(
            "suggestions",
            Some(json!({ "app_name": app_name.unwrap_or_default() })),
        )
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
    overlay::hide_because(&app, "the window asked to be dismissed");
}

/// Asks the panel to stay up when it loses focus.
///
/// The window knows which screen it is showing; the shell knows what a panel
/// is. This is the one bit the window has to tell the shell, and it is a
/// presentation request only — it cannot reach the runtime or a plan.
#[tauri::command]
fn begin_drag(app: tauri::AppHandle) {
    overlay::begin_drag(&app);
}

#[tauri::command]
fn drag_by(app: tauri::AppHandle, dx: f64, dy: f64) {
    overlay::drag_by(&app, dx, dy);
}

#[tauri::command]
fn end_drag() {
    overlay::end_drag();
}

#[tauri::command]
fn set_pinned(pinned: bool) {
    overlay::set_pinned(pinned);
}

/// Reports how tall the content is so the window can match it.
#[tauri::command]
fn resize_panel(app: tauri::AppHandle, height: f64) {
    overlay::resize(&app, height);
}

#[tauri::command]
fn compact_panel(app: tauri::AppHandle, compact: bool) {
    overlay::set_compact(&app, compact);
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

/// Turns the frosting behind the panel on or off.
///
/// Only the blur lives out here. How opaque the panel's own surface is stays
/// in the stylesheet, where it is one custom property — pushing it through the
/// shell as well would mean two places deciding the same appearance.
#[tauri::command]
fn set_blur(app: tauri::AppHandle, blurred: bool, dark: bool) {
    overlay::set_blur(&app, blurred, dark);
}

/// Opens a link in the user's browser.
///
/// The webview will not do it: a target="_blank" inside Tauri has nowhere to
/// go, so every link in a result was inert.
///
/// Only http and https. These URLs were scraped off a page, which makes them
/// attacker-controlled in the ordinary case, and `open` on macOS will happily
/// launch a file:// path, a custom scheme registered by any installed app, or
/// anything else it is handed. The same reasoning already governs the note
/// renderer (D-033); it applies at least as strongly to something that hands
/// the URL to the operating system.
#[tauri::command]
fn open_url(url: String) -> Result<(), String> {
    let lowered = url.trim().to_ascii_lowercase();
    if !(lowered.starts_with("http://") || lowered.starts_with("https://")) {
        return Err(format!("refusing to open a non-web link: {url}"));
    }

    // Passed as an argument rather than through a shell, so a URL containing
    // shell metacharacters is a URL and not a command.
    std::process::Command::new("open")
        .arg("--")
        .arg(url.trim())
        .spawn()
        .map_err(|e| e.to_string())?;
    Ok(())
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
            destinations,
            suggestions,
            commit,
            clear_style,
            guide,
            stop_guide,
            advance_guide,
            point_at,
            clear_pointer,
            dismiss,
            set_pinned,
            begin_drag,
            drag_by,
            end_drag,
            resize_panel,
            compact_panel,
            set_theme,
            set_blur,
            open_url,
            get_screen_context,
            check_screen_recording,
            request_screen_recording
        ])
        .run(tauri::generate_context!())
        .expect("error while running VANS");
}
