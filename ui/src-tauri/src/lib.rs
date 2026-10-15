mod coach;
mod engine;
mod login;
mod notch;
mod overlay;
mod permissions;
mod watching;
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

#[tauri::command]
async fn permissions() -> Result<Vec<permissions::Permission>, String> {
    tauri::async_runtime::spawn_blocking(permissions::check)
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn ask_permissions() -> Result<Vec<permissions::Permission>, String> {
    tauri::async_runtime::spawn_blocking(permissions::ask_all)
        .await
        .map_err(|e| e.to_string())
}

#[tauri::command]
async fn ask_permissions_again(app: tauri::AppHandle) -> Result<Vec<permissions::Permission>, String> {
    let identifier = app.config().identifier.clone();
    tauri::async_runtime::spawn_blocking(move || permissions::ask_again(&identifier))
        .await
        .map_err(|e| e.to_string())?
}

#[tauri::command]
fn open_permission_settings(name: String) {
    permissions::open_settings(&name);
}

#[tauri::command]
fn reopen(app: tauri::AppHandle) {
    app.restart();
}

#[tauri::command]
fn keep_panel() {
    notch::keep();
}

#[tauri::command]
fn take_keyboard(app: tauri::AppHandle) {
    overlay::take_keyboard(&app);
}

#[tauri::command]
fn login_item() -> login::LoginItem {
    login::status()
}

#[tauri::command]
fn set_login_item(on: bool) -> Result<login::LoginItem, String> {
    login::set(on)
}

/// Commands are thin on purpose. Every decision the runtime makes — what to
/// extract, whether it verified, whether a plan may be committed — stays in
/// the runtime, so the window cannot talk it into anything by calling in a
/// different order.
#[tauri::command]
async fn preflight(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("preflight", None).await
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
    mode: Option<String>,
) -> Result<Value, RpcError> {
    let mut params = json!({ "instruction": instruction, "destination": destination });
    if let Some(ctx) = context {
        params["context"] = ctx;
    }
    if let Some(mode) = mode {
        params["mode"] = json!(mode);
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
async fn watching(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("watching", None).await
}

#[tauri::command]
async fn set_watching(app: tauri::AppHandle, engine: State<'_, Engine>, on: bool) -> Result<Value, RpcError> {
    let now = engine.call("set_watching", Some(json!({ "on": on }))).await?;
    let _ = tauri::Emitter::emit(&app, "pill://watching", &now);
    Ok(now)
}

#[tauri::command]
async fn noted_clicks(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("noted_clicks", None).await
}

#[tauri::command]
async fn clear_clicks(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("clear_clicks", None).await
}

#[tauri::command]
async fn advance_guide(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("advance_guide", None).await
}

#[allow(clippy::too_many_arguments)]
#[tauri::command]
fn point_at(
    app: tauri::AppHandle,
    x: f64,
    y: f64,
    w: f64,
    h: f64,
    label: String,
    step: String,
    verb: Option<String>,
    detail: Option<String>,
    app_name: Option<String>,
    mode: Option<String>,
    offer: Option<bool>,
    offer_all: Option<bool>,
    avoid: Option<Vec<f64>>,
) {
    coach::point_at(
        &app,
        coach::Pointer {
            x,
            y,
            w,
            h,
            label,
            step,
            app: app_name.unwrap_or_default(),
            verb: verb.unwrap_or_default(),
            detail: detail.unwrap_or_default(),
            mode: mode.unwrap_or_default(),
            ox: 0.0,
            oy: 0.0,
            offer: offer.unwrap_or(false),
            offer_all: offer_all.unwrap_or(false),
            avoid: avoid.filter(|a| a.len() == 4).unwrap_or_default(),
        },
    );
}

#[tauri::command]
async fn answer_guide(
    engine: State<'_, Engine>,
    yes: bool,
    all: Option<bool>,
) -> Result<Value, RpcError> {
    engine
        .call(
            "answer_guide",
            Some(json!({ "yes": yes, "all": all.unwrap_or(false) })),
        )
        .await
}

#[tauri::command]
async fn answer_question(engine: State<'_, Engine>, answer: String) -> Result<Value, RpcError> {
    engine
        .call("answer_question", Some(json!({ "answer": answer })))
        .await
}

#[tauri::command]
async fn can_send_feedback(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("can_send_feedback", None).await
}

#[tauri::command]
async fn send_feedback(
    engine: State<'_, Engine>,
    kind: String,
    text: String,
) -> Result<Value, RpcError> {
    engine
        .call("send_feedback", Some(json!({ "kind": kind, "text": text })))
        .await
}

#[tauri::command]
async fn rules(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("rules", None).await
}

#[tauri::command]
async fn set_rules(
    engine: State<'_, Engine>,
    never: Vec<String>,
    spend_limit: f64,
    ask_first: Vec<String>,
) -> Result<Value, RpcError> {
    engine
        .call(
            "set_rules",
            Some(json!({ "never": never, "spend_limit": spend_limit, "ask_first": ask_first })),
        )
        .await
}

#[tauri::command]
async fn model_settings(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("model_settings", None).await
}

#[tauri::command]
#[allow(clippy::too_many_arguments)]
async fn set_model_settings(
    engine: State<'_, Engine>,
    api_key: String,
    base_url: String,
    model: String,
    fast_model: String,
    fallback_model: String,
    price_in: f64,
    price_out: f64,
) -> Result<Value, RpcError> {
    engine
        .call(
            "set_model_settings",
            Some(json!({
                "api_key": api_key,
                "base_url": base_url,
                "model": model,
                "fast_model": fast_model,
                "fallback_model": fallback_model,
                "price_in": price_in,
                "price_out": price_out,
            })),
        )
        .await
}

#[tauri::command]
async fn forget_model_key(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("forget_model_key", None).await
}

#[tauri::command]
async fn forget_instruction(engine: State<'_, Engine>, text: String) -> Result<Value, RpcError> {
    engine
        .call("forget_instruction", Some(json!({ "text": text })))
        .await
}

#[tauri::command]
async fn resume_drive(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("resume_drive", None).await
}

#[tauri::command]
fn pause_pointer(app: tauri::AppHandle, ask: Option<String>, label: Option<String>) {
    coach::paused(&app, ask.unwrap_or_default(), label.unwrap_or_default());
}

#[tauri::command]
fn clear_pointer(app: tauri::AppHandle) {
    coach::clear(&app);
}

/// Marks the last control done, then takes the overlay away on its own.
#[tauri::command]
fn finish_pointer(app: tauri::AppHandle) {
    coach::finish(&app);
}

#[tauri::command]
async fn clear_style(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("clear_style", None).await
}

/// What VANS has changed on this machine. Read-only, and asks for nothing.
#[tauri::command]
async fn history(engine: State<'_, Engine>) -> Result<Value, RpcError> {
    engine.call("history", None).await
}

/// Puts one recorded change back. No gate: undoing is not a thing to be
/// protected from, and asking twice teaches people to dismiss the asking.
#[tauri::command]
async fn undo(engine: State<'_, Engine>, id: i64) -> Result<Value, RpcError> {
    engine.call("undo", Some(json!({ "id": id }))).await
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
/// Lets a window report what it is doing into the same log as the shell.
///
/// A webview's console is somewhere nobody is looking, and the two failures that
/// cost the most here — a page that never loaded and an event that reached no
/// listener — both look identical from outside: nothing happens, quietly.
#[tauri::command]
fn note(from: String, what: String) {
    eprintln!("[vans] {from}: {what}");
}

/// Brings the panel back, for when the runtime has something to tell the person.
///
/// The coach puts it away while it is pointing — the steps belong on the
/// application, not in a window over it — so this is how the end of a run, or a
/// failure, gets said.
#[tauri::command]
fn summon(app: tauri::AppHandle) {
    overlay::show(&app);
}

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
            notch::install(&handle_for_overlay)?;
            watching::install(&handle_for_overlay);
            Ok(())
        })
        .plugin(tauri_plugin_global_shortcut::Builder::new().build())
        .invoke_handler(tauri::generate_handler![
            preflight,
            prepare,
            destinations,
            suggestions,
            commit,
            clear_style,
            guide,
            stop_guide,
            advance_guide,
            watching,
            set_watching,
            noted_clicks,
            clear_clicks,
            resume_drive,
            answer_guide,
            answer_question,
            rules,
            set_rules,
            can_send_feedback,
            send_feedback,
            model_settings,
            set_model_settings,
            forget_model_key,
            forget_instruction,
            pause_pointer,
            history,
            undo,
            point_at,
            clear_pointer,
            finish_pointer,
            dismiss,
            summon,
            note,
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
            permissions,
            ask_permissions,
            ask_permissions_again,
            open_permission_settings,
            reopen,
            take_keyboard,
            keep_panel,
            login_item,
            set_login_item
        ])
        .run(tauri::generate_context!())
        .expect("error while running VANS");
}
