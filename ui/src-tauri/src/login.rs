use serde::Serialize;

#[derive(Debug, Clone, Serialize)]
pub struct LoginItem {
    pub on: bool,
    pub works: bool,
    pub approve: bool,
}

fn installed() -> bool {
    std::env::current_exe()
        .map(|p| p.to_string_lossy().contains(".app/Contents/MacOS/"))
        .unwrap_or(false)
}

#[cfg(target_os = "macos")]
pub fn status() -> LoginItem {
    use objc2_service_management::{SMAppService, SMAppServiceStatus};
    let state = unsafe { SMAppService::mainAppService().status() };
    LoginItem {
        on: state == SMAppServiceStatus::Enabled,
        works: installed(),
        approve: state == SMAppServiceStatus::RequiresApproval,
    }
}

#[cfg(target_os = "macos")]
pub fn set(on: bool) -> Result<LoginItem, String> {
    use objc2_service_management::SMAppService;
    if !installed() {
        return Err("Starting at login works in the installed app only.".into());
    }
    let service = unsafe { SMAppService::mainAppService() };
    let done = unsafe {
        if on {
            service.registerAndReturnError()
        } else {
            service.unregisterAndReturnError()
        }
    };
    done.map_err(|e| e.localizedDescription().to_string())?;
    Ok(status())
}

#[cfg(not(target_os = "macos"))]
pub fn status() -> LoginItem {
    LoginItem {
        on: false,
        works: false,
        approve: false,
    }
}

#[cfg(not(target_os = "macos"))]
pub fn set(_on: bool) -> Result<LoginItem, String> {
    Err("Starting at login works on macOS only.".into())
}
