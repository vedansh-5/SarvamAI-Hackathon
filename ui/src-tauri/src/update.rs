use std::time::Duration;

use tauri::{AppHandle, Emitter, Manager, Url};
use tauri_plugin_updater::UpdaterExt;

use crate::engine::Engine;

const FIRST_LOOK: Duration = Duration::from_secs(90);
const LOOK_EVERY: Duration = Duration::from_secs(6 * 60 * 60);
const PUBKEY: Option<&str> = option_env!("VANS_UPDATER_PUBKEY");

pub fn install(app: &AppHandle) {
    let Some(pubkey) = PUBKEY.filter(|k| !k.trim().is_empty()) else {
        return;
    };
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(FIRST_LOOK).await;
        loop {
            if let Err(e) = look(&app, pubkey).await {
                eprintln!("[update] {e}");
            }
            tokio::time::sleep(LOOK_EVERY).await;
        }
    });
}

async fn look(app: &AppHandle, pubkey: &str) -> Result<(), String> {
    let engine = app.try_state::<Engine>().ok_or("no engine")?;
    let source = engine.call("update_source", None).await.map_err(|e| e.message)?;
    let server = source.get("server").and_then(|v| v.as_str()).unwrap_or_default().trim_end_matches('/');
    let code = source.get("code").and_then(|v| v.as_str()).unwrap_or_default();
    if server.is_empty() || code.is_empty() {
        return Ok(());
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
    let _ = app.emit("vans://updated", update.version.clone());
    Ok(())
}
