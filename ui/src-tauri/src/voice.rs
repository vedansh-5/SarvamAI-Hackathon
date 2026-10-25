use serde::{Deserialize, Serialize};
use serde_json::json;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use tauri::{AppHandle, Emitter, Listener, LogicalPosition, LogicalSize, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};

use crate::screen::ScreenContext;

pub const LABEL: &str = "voice";
pub const HEARD: &str = "voice://heard";
const SAYS: &str = "voice://says";
const WORDS: &str = "voice://words";
const LEVEL: &str = "voice://level";
const SHAPE: &str = "voice://shape";
const CLOSE: &str = "voice://close";
const QUIET: &str = "voice://quiet";
const LISTENING: &str = "pill://listening";
const SETTINGS_FILE: &str = "voice.json";

const WIDTH: f64 = 460.0;
const BODY: f64 = 70.0;
const GAP: f64 = 6.0;
const CLOSING: Duration = Duration::from_millis(320);

const HOLD: Duration = Duration::from_millis(280);
const QUIET_ENDS: Duration = Duration::from_millis(1400);
const NOTHING_SAID: Duration = Duration::from_secs(6);
const LONGEST: Duration = Duration::from_secs(30);
const LAST_WORDS: Duration = Duration::from_millis(1800);
const LEVEL_EVERY: Duration = Duration::from_millis(60);
const WATCH: Duration = Duration::from_millis(100);
const AFTER_SPEAKING: Duration = Duration::from_secs(1);
const SPEAKING_DB: f32 = -42.0;
const WARM_UP: Duration = Duration::from_millis(300);
const SENT_AFTER: Duration = Duration::from_millis(700);
const HINT_CHARS: usize = 60;

const NO_EARS: &str = "Vspk can't hear you. Allow Microphone and Speech Recognition for Vspk in System Settings › Privacy & Security.";
const ON_THIS_MAC: &str = "Vspk listens on this Mac only. Turn on Dictation in System Settings › Keyboard so your language is on this Mac, then try again.";

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "kebab-case")]
pub enum Key {
    OptionSpace,
    ControlOption,
}

#[derive(Debug, Clone, Serialize, Deserialize)]
#[serde(default)]
pub struct Settings {
    pub on: bool,
    pub key: Key,
    pub aloud: bool,
    pub sounds: bool,
}

impl Default for Settings {
    fn default() -> Self {
        Settings { on: true, key: Key::OptionSpace, aloud: true, sounds: true }
    }
}

static SETTINGS: Mutex<Option<Settings>> = Mutex::new(None);

pub fn settings() -> Settings {
    SETTINGS.lock().unwrap().clone().unwrap_or_default()
}

fn settings_path(app: &AppHandle) -> Option<std::path::PathBuf> {
    app.path().app_config_dir().ok().map(|d| d.join(SETTINGS_FILE))
}

fn load_settings(app: &AppHandle) {
    let read = settings_path(app)
        .and_then(|p| std::fs::read_to_string(p).ok())
        .and_then(|raw| serde_json::from_str::<Settings>(&raw).ok())
        .unwrap_or_default();
    *SETTINGS.lock().unwrap() = Some(read);
}

pub fn save_settings(app: &AppHandle, next: Settings) -> Result<Settings, String> {
    let path = settings_path(app).ok_or("Vspk has no folder to keep its settings in.")?;
    if let Some(dir) = path.parent() {
        std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
    }
    let raw = serde_json::to_string_pretty(&next).map_err(|e| e.to_string())?;
    std::fs::write(&path, raw).map_err(|e| e.to_string())?;
    *SETTINGS.lock().unwrap() = Some(next.clone());
    if !next.on {
        let main = app.clone();
        let _ = app.run_on_main_thread(move || cancel(&main));
    }
    if !next.aloud {
        crate::speak::hush(app);
    }
    Ok(next)
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Mode {
    Held,
    HandsFree,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum End {
    Done,
    NothingSaid,
    TooLong,
}

fn end_for(mode: Mode, listening: Duration, quiet: Option<Duration>) -> Option<End> {
    if listening >= LONGEST {
        return Some(End::TooLong);
    }
    if mode == Mode::Held {
        return None;
    }
    match quiet {
        Some(q) if q >= QUIET_ENDS => Some(End::Done),
        None if listening >= NOTHING_SAID => Some(End::NothingSaid),
        _ => None,
    }
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
enum Command {
    Next,
    Stop,
    Again,
}

fn command(said: &str) -> Option<Command> {
    let lowered = said.to_lowercase();
    let words: Vec<&str> = lowered
        .split(|c: char| !c.is_alphanumeric() && c != '\'')
        .filter(|w| !w.is_empty())
        .collect();
    if words.is_empty() || words.len() > 4 {
        return None;
    }
    if words.iter().any(|w| matches!(*w, "stop" | "cancel" | "quit" | "exit" | "enough")) {
        return Some(Command::Stop);
    }
    if words.iter().any(|w| matches!(*w, "again" | "repeat" | "pardon")) || lowered.contains("say that") {
        return Some(Command::Again);
    }
    if words.iter().any(|w| matches!(*w, "next" | "continue" | "done" | "onward")) {
        return Some(Command::Next);
    }
    None
}

pub fn step_words(verb: &str, label: &str, detail: &str, points: bool) -> String {
    let label = label.trim();
    let detail = detail.trim();
    if !points {
        return label.to_string();
    }
    let thing = if label.is_empty() { "here" } else { label };
    match verb.trim().to_ascii_lowercase().as_str() {
        "type" if detail.is_empty() => format!("Type in {thing}"),
        "type" => format!("Type {detail}"),
        "press" if detail.is_empty() => "Press the keys".into(),
        "press" => format!("Press {detail}"),
        "scroll" => format!("Scroll {detail}").trim_end().to_string(),
        "doubleclick" => format!("Double-click {thing}"),
        "rightclick" => format!("Right-click {thing}"),
        "hover" => format!("Point at {thing}, without clicking"),
        "drag" if detail.is_empty() => format!("Drag {thing}"),
        "drag" => format!("Drag {thing} to {detail}"),
        "set" if detail.is_empty() => format!("Set {thing}"),
        "set" => format!("Set {thing} to {detail}"),
        "wait" => format!("Wait for {thing}"),
        _ => format!("Click {thing}"),
    }
}

fn hints(app_name: &str, title: &str) -> Vec<String> {
    let mut out = vec!["Vspk".to_string()];
    for hint in [app_name.trim(), title.trim()] {
        if hint.is_empty() || out.iter().any(|h| h.eq_ignore_ascii_case(hint)) {
            continue;
        }
        out.push(hint.chars().take(HINT_CHARS).collect());
    }
    out
}

fn quiet_for(mode: Mode, quiet: Option<Duration>) -> f32 {
    match (mode, quiet) {
        (Mode::HandsFree, Some(q)) => (q.as_secs_f32() / QUIET_ENDS.as_secs_f32()).clamp(0.0, 1.0),
        _ => 0.0,
    }
}

fn level_of(rms: f32) -> f32 {
    let db = 20.0 * rms.max(1e-6).log10();
    ((db + 55.0) / 45.0).clamp(0.0, 1.0)
}

fn speaking_rms(rms: f32) -> bool {
    20.0 * rms.max(1e-6).log10() > SPEAKING_DB
}

pub struct Heard {
    words: String,
    started: Instant,
    spoke: Option<Instant>,
    sent: Option<Instant>,
    ending: bool,
}

impl Heard {
    fn new() -> Self {
        Heard { words: String::new(), started: Instant::now(), spoke: None, sent: None, ending: false }
    }

    fn sound(&mut self, rms: f32) -> Option<f32> {
        if speaking_rms(rms) && self.started.elapsed() >= WARM_UP {
            self.spoke = Some(Instant::now());
        }
        if self.sent.is_some_and(|t| t.elapsed() < LEVEL_EVERY) {
            return None;
        }
        self.sent = Some(Instant::now());
        Some(level_of(rms))
    }
}

struct Session {
    id: u64,
    mode: Mode,
    guide: bool,
    context: ScreenContext,
    heard: Arc<Mutex<Heard>>,
}

static SESSION: Mutex<Option<Session>> = Mutex::new(None);
static NEXT_ID: AtomicU64 = AtomicU64::new(1);
static KEY_DOWN: AtomicBool = AtomicBool::new(false);
static PRESS: AtomicU64 = AtomicU64::new(0);
static HELD: AtomicBool = AtomicBool::new(false);
static CHORD: AtomicBool = AtomicBool::new(false);
static SPOILED: AtomicBool = AtomicBool::new(false);
static SAID: AtomicU64 = AtomicU64::new(0);
static HOLDING_ESC: AtomicBool = AtomicBool::new(false);
static ALOUD: AtomicBool = AtomicBool::new(false);
static SPEAKING: AtomicBool = AtomicBool::new(false);
static LAST_STEP: Mutex<String> = Mutex::new(String::new());
static LAST_SAID: Mutex<String> = Mutex::new(String::new());

fn hands_free_key() -> Shortcut {
    Shortcut::new(Some(Modifiers::ALT | Modifiers::SHIFT), Code::Space)
}

fn esc() -> Shortcut {
    Shortcut::new(None, Code::Escape)
}

fn window(app: &AppHandle) -> Option<WebviewWindow> {
    app.get_webview_window(LABEL)
}

fn show_island(app: &AppHandle) {
    let Some(win) = window(app) else { return };
    if let Some(at) = crate::notch::island(WIDTH, BODY, GAP) {
        let _ = win.set_size(LogicalSize::new(at.width, at.height));
        let _ = win.set_position(LogicalPosition::new(at.x, at.y));
        let _ = app.emit_to(LABEL, SHAPE, json!({ "notch": at.notch, "bar": at.bar, "width": at.width }));
    }
    let _ = win.show();
    crate::notch::sit_in_the_menu_bar(&win);
    let _ = win.set_ignore_cursor_events(true);
}

fn close_island(app: &AppHandle, how: &str) {
    let n = SAID.load(Ordering::SeqCst);
    let _ = app.emit_to(LABEL, CLOSE, json!({ "how": how }));
    let handle = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(CLOSING);
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || {
            if SAID.load(Ordering::SeqCst) == n {
                if let Some(win) = window(&main) {
                    let _ = win.hide();
                }
            }
        });
    });
}

fn tell(app: &AppHandle, state: &str, words: &str) -> u64 {
    let n = SAID.fetch_add(1, Ordering::SeqCst) + 1;
    show_island(app);
    let _ = app.emit_to(LABEL, SAYS, json!({ "state": state, "words": words }));
    n
}

fn tell_then_go(app: &AppHandle, state: &str, words: &str, after: Duration) {
    let n = tell(app, state, words);
    go_after(app, n, after);
}

fn go_after(app: &AppHandle, n: u64, after: Duration) {
    let handle = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(after);
        if SAID.load(Ordering::SeqCst) != n || SESSION.lock().unwrap().is_some() || SPEAKING.load(Ordering::SeqCst) {
            return;
        }
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || {
            if SAID.load(Ordering::SeqCst) == n {
                close_island(&main, "done");
            }
        });
    });
}

fn chime(name: &'static str) {
    if settings().sounds {
        mac::chime(name);
    }
}

fn arm(app: &AppHandle) {
    if SESSION.lock().unwrap().is_some() {
        return;
    }
    tell(app, "arming", "");
}

fn disarm(app: &AppHandle) {
    if SESSION.lock().unwrap().is_none() && !SPEAKING.load(Ordering::SeqCst) {
        SAID.fetch_add(1, Ordering::SeqCst);
        close_island(app, "tap");
    }
}

fn listening_light(app: &AppHandle, on: bool) {
    let _ = app.emit_to(crate::notch::LABEL, LISTENING, on);
}

fn hold_esc(app: &AppHandle) {
    if HOLDING_ESC.load(Ordering::SeqCst) || app.global_shortcut().is_registered(esc()) {
        return;
    }
    HOLDING_ESC.store(true, Ordering::SeqCst);
    let registered = app.global_shortcut().on_shortcut(esc(), |app, _, event| {
        if event.state() == ShortcutState::Pressed {
            let main = app.clone();
            let _ = app.run_on_main_thread(move || {
                cancel(&main);
                crate::speak::hush(&main);
                quiet(&main);
            });
        }
    });
    if let Err(e) = registered {
        HOLDING_ESC.store(false, Ordering::SeqCst);
        eprintln!("[vans] could not take Escape while listening: {e}");
    }
}

fn release_esc(app: &AppHandle) {
    if SESSION.lock().unwrap().is_some() || SPEAKING.load(Ordering::SeqCst) {
        return;
    }
    if HOLDING_ESC.swap(false, Ordering::SeqCst) {
        let _ = app.global_shortcut().unregister(esc());
    }
}

fn holds_to_talk(key: Key) -> bool {
    let s = settings();
    s.on && s.key == key
}

fn press(app: &AppHandle) {
    crate::speak::hush(app);
    let main = app.clone();
    let _ = app.run_on_main_thread(move || arm(&main));
    let press = PRESS.fetch_add(1, Ordering::SeqCst) + 1;
    let handle = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(HOLD);
        if !KEY_DOWN.load(Ordering::SeqCst) || PRESS.load(Ordering::SeqCst) != press || SPOILED.load(Ordering::SeqCst) {
            return;
        }
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || {
            if KEY_DOWN.load(Ordering::SeqCst) && PRESS.load(Ordering::SeqCst) == press && !SPOILED.load(Ordering::SeqCst) {
                HELD.store(true, Ordering::SeqCst);
                start(&main, Mode::Held);
            }
        });
    });
}

fn hands_free_running() -> bool {
    SESSION.lock().unwrap().as_ref().is_some_and(|s| s.mode == Mode::HandsFree)
}

pub fn key_down(app: &AppHandle) {
    if KEY_DOWN.swap(true, Ordering::SeqCst) {
        return;
    }
    if !holds_to_talk(Key::OptionSpace) {
        crate::speak::hush(app);
        return;
    }
    SPOILED.store(false, Ordering::SeqCst);
    press(app);
}

pub fn key_up(app: &AppHandle) -> bool {
    KEY_DOWN.store(false, Ordering::SeqCst);
    if HELD.swap(false, Ordering::SeqCst) || hands_free_running() {
        let main = app.clone();
        let _ = app.run_on_main_thread(move || finish_current(&main));
        return false;
    }
    if holds_to_talk(Key::OptionSpace) {
        let main = app.clone();
        let _ = app.run_on_main_thread(move || disarm(&main));
    }
    true
}

fn chord(app: &AppHandle, held: bool) {
    let was = CHORD.swap(held, Ordering::SeqCst);
    if held == was {
        return;
    }
    if held {
        if !holds_to_talk(Key::ControlOption) {
            CHORD.store(false, Ordering::SeqCst);
            return;
        }
        SPOILED.store(false, Ordering::SeqCst);
        KEY_DOWN.store(true, Ordering::SeqCst);
        mac::watch_other_keys(app);
        press(app);
        return;
    }
    mac::unwatch_other_keys();
    KEY_DOWN.store(false, Ordering::SeqCst);
    if HELD.swap(false, Ordering::SeqCst) {
        finish_current(app);
    } else {
        disarm(app);
    }
}

fn chord_spoiled(app: &AppHandle) {
    if !CHORD.load(Ordering::SeqCst) || SPOILED.swap(true, Ordering::SeqCst) {
        return;
    }
    if HELD.swap(false, Ordering::SeqCst) {
        cancel(app);
    } else {
        disarm(app);
    }
}

pub fn hands_free(app: &AppHandle) {
    let main = app.clone();
    let _ = app.run_on_main_thread(move || {
        if SESSION.lock().unwrap().is_some() {
            finish_current(&main);
        } else if settings().on {
            crate::speak::hush(&main);
            start(&main, Mode::HandsFree);
        }
    });
}

fn start(app: &AppHandle, mode: Mode) {
    if SESSION.lock().unwrap().is_some() {
        return;
    }
    match mac::ears() {
        Ears::Ask => {
            tell(app, "note", "Allow Vspk to hear you, then try again.");
            mac::ask_for_ears(app);
            return;
        }
        Ears::Refused => {
            tell_then_go(app, "note", NO_EARS, Duration::from_secs(6));
            return;
        }
        Ears::Ready => {}
    }

    chime("Tink");
    let guide = crate::coach::is_showing();
    let context = crate::screen::capture_active_context(false);
    let id = NEXT_ID.fetch_add(1, Ordering::SeqCst);
    let heard = Arc::new(Mutex::new(Heard::new()));
    if let Err(why) = mac::listen(app, id, &heard, &hints(&context.app_name, &context.window_title)) {
        tell_then_go(app, "note", &why, Duration::from_secs(6));
        return;
    }
    *SESSION.lock().unwrap() = Some(Session { id, mode, guide, context, heard: heard.clone() });
    eprintln!("[vans] listening ({mode:?}{})", if guide { ", during a guide" } else { "" });
    hold_esc(app);
    listening_light(app, true);
    tell(app, "listening", if guide { "Say next, again or stop" } else { "" });
    watch(app, id, mode, heard);
}

fn watch(app: &AppHandle, id: u64, mode: Mode, heard: Arc<Mutex<Heard>>) {
    let handle = app.clone();
    std::thread::spawn(move || loop {
        std::thread::sleep(WATCH);
        if SESSION.lock().unwrap().as_ref().map(|s| s.id) != Some(id) {
            return;
        }
        let (end, quiet) = {
            let h = heard.lock().unwrap();
            if h.ending {
                return;
            }
            let since = h.spoke.map(|t| t.elapsed());
            (end_for(mode, h.started.elapsed(), since), quiet_for(mode, since))
        };
        if mode == Mode::HandsFree {
            let _ = handle.emit_to(LABEL, QUIET, quiet);
        }
        let Some(end) = end else { continue };
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || match end {
            End::NothingSaid => {
                cancel(&main);
                tell_then_go(&main, "note", "Didn't hear anything.", Duration::from_millis(1600));
            }
            End::Done | End::TooLong => finish(&main, id),
        });
        return;
    });
}

fn finish_current(app: &AppHandle) {
    let id = SESSION.lock().unwrap().as_ref().map(|s| s.id);
    if let Some(id) = id {
        finish(app, id);
    }
}

fn finish(app: &AppHandle, id: u64) {
    let words = {
        let session = SESSION.lock().unwrap();
        let Some(s) = session.as_ref().filter(|s| s.id == id) else { return };
        let mut h = s.heard.lock().unwrap();
        if h.ending {
            return;
        }
        h.ending = true;
        h.words.clone()
    };
    mac::stop_hearing();
    listening_light(app, false);
    tell(app, "thinking", &words);
    let handle = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(LAST_WORDS);
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || deliver(&main, id));
    });
}

fn deliver(app: &AppHandle, id: u64) {
    let session = {
        let mut slot = SESSION.lock().unwrap();
        if slot.as_ref().map(|s| s.id) != Some(id) {
            return;
        }
        slot.take()
    };
    let Some(session) = session else { return };
    mac::drop_live();
    release_esc(app);
    listening_light(app, false);

    if crate::sarvam::on() {
        let live = session.heard.lock().unwrap().words.trim().to_string();
        tell(app, "thinking", if live.is_empty() { "…" } else { &live });
        let handle = app.clone();
        std::thread::spawn(move || {
            let heard = crate::sarvam::hear();
            let main = handle.clone();
            let _ = handle.run_on_main_thread(move || match heard {
                Ok(h) => {
                    eprintln!("[vans] sarvam heard ({}): {:?}", h.language, h.text);
                    act_on(&main, session, h.text);
                }
                Err(e) => {
                    eprintln!("[vans] sarvam: {e}");
                    act_on(&main, session, live);
                }
            });
        });
        return;
    }
    let words = session.heard.lock().unwrap().words.trim().to_string();
    act_on(app, session, words);
}

fn act_on(app: &AppHandle, session: Session, words: String) {
    eprintln!("[vans] heard: {words:?}");
    if words.is_empty() {
        tell_then_go(app, "note", "Didn't catch that.", Duration::from_millis(1600));
        return;
    }
    if session.guide {
        match command(&words) {
            Some(Command::Next) => {
                let _ = app.emit(crate::coach::NEXT, ());
                tell_then_go(app, "heard", &words, Duration::from_millis(900));
            }
            Some(Command::Stop) => {
                let _ = app.emit(crate::coach::STOP, ());
                tell_then_go(app, "heard", &words, Duration::from_millis(900));
            }
            Some(Command::Again) => {
                let again = LAST_SAID.lock().unwrap().clone();
                if again.is_empty() {
                    tell_then_go(app, "heard", &words, Duration::from_millis(900));
                } else {
                    crate::speak::say(app, &again);
                }
            }
            None => tell_then_go(app, "note", "During a guide, say \"next\", \"again\" or \"stop\".", Duration::from_millis(2500)),
        }
        return;
    }
    ALOUD.store(true, Ordering::SeqCst);
    LAST_STEP.lock().unwrap().clear();
    chime("Pop");
    let _ = app.emit_to("main", HEARD, json!({ "text": words, "context": session.context }));
    let n = tell(app, "heard", &words);
    let handle = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(SENT_AFTER);
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || {
            if SAID.load(Ordering::SeqCst) == n && SESSION.lock().unwrap().is_none() && !SPEAKING.load(Ordering::SeqCst) {
                close_island(&main, "sent");
            }
        });
    });
}

fn cancel(app: &AppHandle) {
    if SESSION.lock().unwrap().take().is_none() {
        return;
    }
    HELD.store(false, Ordering::SeqCst);
    mac::stop_hearing();
    mac::drop_live();
    release_esc(app);
    listening_light(app, false);
    SAID.fetch_add(1, Ordering::SeqCst);
    close_island(app, "cancel");
}

fn words_now(app: &AppHandle, id: u64, words: String) {
    let session = SESSION.lock().unwrap();
    let Some(s) = session.as_ref().filter(|s| s.id == id) else { return };
    s.heard.lock().unwrap().words = words.clone();
    let _ = app.emit_to(LABEL, WORDS, &words);
}

fn level_now(app: &AppHandle, heard: &Mutex<Heard>, rms: f32) {
    let level = heard.lock().unwrap().sound(rms);
    if let Some(level) = level {
        let _ = app.emit_to(LABEL, LEVEL, level);
    }
}

pub fn speaking(app: &AppHandle, words: &str) {
    SPEAKING.store(true, Ordering::SeqCst);
    *LAST_SAID.lock().unwrap() = words.to_string();
    hold_esc(app);
    tell(app, "speaking", words);
}

pub fn shown_not_said(app: &AppHandle, words: &str) {
    *LAST_SAID.lock().unwrap() = words.to_string();
    tell_then_go(app, "heard", words, Duration::from_secs(4));
}

pub fn quiet(app: &AppHandle) {
    if !SPEAKING.swap(false, Ordering::SeqCst) {
        return;
    }
    release_esc(app);
    if SESSION.lock().unwrap().is_some() {
        return;
    }
    let n = SAID.load(Ordering::SeqCst);
    go_after(app, n, AFTER_SPEAKING);
}

pub fn set_aloud(on: bool) {
    ALOUD.store(on, Ordering::SeqCst);
    LAST_STEP.lock().unwrap().clear();
}

fn aloud_now() -> bool {
    ALOUD.load(Ordering::SeqCst) && settings().on && settings().aloud
}

pub fn say_if_asked_aloud(app: &AppHandle, text: &str) -> bool {
    if !aloud_now() {
        return false;
    }
    crate::speak::say(app, text);
    true
}

pub fn step_aloud(app: &AppHandle, p: &crate::coach::Pointer) {
    if !aloud_now() || p.mode == "drive" {
        return;
    }
    let key = format!("{}|{}|{}|{}", p.step, p.label, p.verb, p.detail);
    {
        let mut last = LAST_STEP.lock().unwrap();
        if *last == key {
            return;
        }
        *last = key;
    }
    crate::speak::say(app, &step_words(&p.verb, &p.label, &p.detail, p.w > 0.0 && p.h > 0.0));
}

#[derive(Clone, Serialize)]
pub struct EarsState {
    pub state: &'static str,
}

pub fn ears_state() -> EarsState {
    EarsState {
        state: match mac::ears() {
            Ears::Ready => "granted",
            Ears::Ask => "not_asked",
            Ears::Refused => "needed",
        },
    }
}

pub fn ask_ears(app: &AppHandle) {
    let main = app.clone();
    let _ = app.run_on_main_thread(move || mac::ask_for_ears(&main));
}

pub enum Ears {
    Ready,
    Ask,
    Refused,
}

fn after_asking(app: &AppHandle) {
    let _ = app.emit("voice://ears", ears_state());
    match mac::ears() {
        Ears::Ready => tell_then_go(app, "note", "Vspk can hear you now. Hold the key and talk.", Duration::from_secs(3)),
        _ => tell_then_go(app, "note", NO_EARS, Duration::from_secs(6)),
    }
}

#[cfg(target_os = "macos")]
mod mac {
    use super::{Ears, Heard, ON_THIS_MAC};
    use block2::RcBlock;
    use objc2::rc::Retained;
    use objc2::runtime::{AnyObject, Bool};
    use objc2::AnyThread;
    use objc2_app_kit::{NSEvent, NSEventMask, NSEventModifierFlags};
    use objc2_av_foundation::{AVAuthorizationStatus, AVCaptureDevice, AVMediaTypeAudio};
    use objc2_avf_audio::{AVAudioEngine, AVAudioPCMBuffer, AVAudioTime};
    use objc2_foundation::{NSArray, NSError, NSString};
    use objc2_speech::{
        SFSpeechAudioBufferRecognitionRequest, SFSpeechRecognitionResult, SFSpeechRecognitionTask,
        SFSpeechRecognizer, SFSpeechRecognizerAuthorizationStatus,
    };
    use std::cell::RefCell;
    use std::ptr::NonNull;
    use std::sync::{Arc, Mutex};
    use tauri::AppHandle;

    type Tap = RcBlock<dyn Fn(NonNull<AVAudioPCMBuffer>, NonNull<AVAudioTime>)>;

    struct Live {
        engine: Retained<AVAudioEngine>,
        request: Retained<SFSpeechAudioBufferRecognitionRequest>,
        task: Retained<SFSpeechRecognitionTask>,
        _recognizer: Retained<SFSpeechRecognizer>,
        _tap: Tap,
        stopped: bool,
    }

    thread_local! {
        static LIVE: RefCell<Option<Live>> = const { RefCell::new(None) };
        static OTHER_KEYS: RefCell<Option<Retained<AnyObject>>> = const { RefCell::new(None) };
    }

    pub fn ears() -> Ears {
        let speech = unsafe { SFSpeechRecognizer::authorizationStatus() };
        let mic = unsafe { AVMediaTypeAudio.map(|t| AVCaptureDevice::authorizationStatusForMediaType(t)) };
        let Some(mic) = mic else { return Ears::Refused };
        if speech == SFSpeechRecognizerAuthorizationStatus::Authorized && mic == AVAuthorizationStatus::Authorized {
            Ears::Ready
        } else if speech == SFSpeechRecognizerAuthorizationStatus::NotDetermined
            || mic == AVAuthorizationStatus::NotDetermined
        {
            Ears::Ask
        } else {
            Ears::Refused
        }
    }

    pub fn ask_for_ears(app: &AppHandle) {
        let handle = app.clone();
        let after_speech = RcBlock::new(move |_status: SFSpeechRecognizerAuthorizationStatus| {
            let after = handle.clone();
            let after_mic = RcBlock::new(move |_granted: Bool| {
                let main = after.clone();
                let _ = after.run_on_main_thread(move || super::after_asking(&main));
            });
            unsafe {
                if let Some(audio) = AVMediaTypeAudio {
                    AVCaptureDevice::requestAccessForMediaType_completionHandler(audio, &after_mic);
                }
            }
        });
        unsafe { SFSpeechRecognizer::requestAuthorization(&after_speech) };
    }

    fn samples(buffer: &AVAudioPCMBuffer) -> &[f32] {
        unsafe {
            let frames = buffer.frameLength() as usize;
            let channels = buffer.floatChannelData();
            if channels.is_null() || frames == 0 {
                return &[];
            }
            std::slice::from_raw_parts((*channels).as_ptr(), frames)
        }
    }

    fn rms(samples: &[f32]) -> f32 {
        if samples.is_empty() {
            return 0.0;
        }
        (samples.iter().map(|s| s * s).sum::<f32>() / samples.len() as f32).sqrt()
    }

    pub fn listen(app: &AppHandle, id: u64, heard: &Arc<Mutex<Heard>>, hints: &[String]) -> Result<(), String> {
        unsafe {
            let recognizer = SFSpeechRecognizer::init(SFSpeechRecognizer::alloc()).ok_or(ON_THIS_MAC)?;
            if !recognizer.supportsOnDeviceRecognition() {
                return Err(ON_THIS_MAC.into());
            }
            if !recognizer.isAvailable() {
                return Err("Speech recognition isn't ready yet. Try again in a moment.".into());
            }

            let request = SFSpeechAudioBufferRecognitionRequest::new();
            request.setShouldReportPartialResults(true);
            request.setRequiresOnDeviceRecognition(true);
            request.setAddsPunctuation(true);
            let hints: Vec<Retained<NSString>> = hints.iter().map(|h| NSString::from_str(h)).collect();
            request.setContextualStrings(&NSArray::from_retained_slice(&hints));

            let engine = AVAudioEngine::new();
            let input = engine.inputNode();
            let format = input.outputFormatForBus(0);
            if format.channelCount() == 0 {
                return Err("Vspk can't find a microphone.".into());
            }
            let keep = crate::sarvam::on();
            if keep {
                crate::sarvam::begin(format.sampleRate());
            }

            let feed = request.clone();
            let loud_app = app.clone();
            let loud = heard.clone();
            let tap: Tap = RcBlock::new(move |buffer: NonNull<AVAudioPCMBuffer>, _when: NonNull<AVAudioTime>| {
                let buffer = buffer.as_ref();
                feed.appendAudioPCMBuffer(buffer);
                let heard = samples(buffer);
                if keep {
                    crate::sarvam::push(heard);
                }
                super::level_now(&loud_app, &loud, rms(heard));
            });
            input.installTapOnBus_bufferSize_format_block(0, 1024, Some(&format), RcBlock::as_ptr(&tap));

            engine.prepare();
            if let Err(e) = engine.startAndReturnError() {
                input.removeTapOnBus(0);
                return Err(format!("Vspk couldn't start the microphone: {}", e.localizedDescription()));
            }

            let words_app = app.clone();
            let handler = RcBlock::new(move |result: *mut SFSpeechRecognitionResult, error: *mut NSError| {
                let main = words_app.clone();
                if let Some(result) = result.as_ref() {
                    let words = result.bestTranscription().formattedString().to_string();
                    let last = result.isFinal();
                    let _ = words_app.run_on_main_thread(move || {
                        super::words_now(&main, id, words);
                        if last {
                            super::deliver(&main, id);
                        }
                    });
                } else if !error.is_null() {
                    let _ = words_app.run_on_main_thread(move || super::deliver(&main, id));
                }
            });
            let task = recognizer.recognitionTaskWithRequest_resultHandler(&request, &handler);

            LIVE.with(|live| {
                *live.borrow_mut() = Some(Live {
                    engine,
                    request,
                    task,
                    _recognizer: recognizer,
                    _tap: tap,
                    stopped: false,
                })
            });
            Ok(())
        }
    }

    pub fn stop_hearing() {
        LIVE.with(|live| {
            let mut live = live.borrow_mut();
            let Some(live) = live.as_mut() else { return };
            if live.stopped {
                return;
            }
            live.stopped = true;
            unsafe {
                live.engine.stop();
                live.engine.inputNode().removeTapOnBus(0);
                live.request.endAudio();
            }
        });
    }

    pub fn drop_live() {
        stop_hearing();
        let live = LIVE.with(|live| live.borrow_mut().take());
        if let Some(live) = live {
            unsafe { live.task.cancel() };
        }
    }

    fn talk_chord(flags: NSEventModifierFlags) -> bool {
        flags.contains(NSEventModifierFlags::Control)
            && flags.contains(NSEventModifierFlags::Option)
            && !flags.contains(NSEventModifierFlags::Command)
            && !flags.contains(NSEventModifierFlags::Shift)
    }

    pub fn watch_chord(app: &AppHandle) {
        let handle = app.clone();
        let global = RcBlock::new(move |event: NonNull<NSEvent>| {
            let flags = unsafe { event.as_ref() }.modifierFlags();
            super::chord(&handle, talk_chord(flags));
        });
        let local_handle = app.clone();
        let local = RcBlock::new(move |event: NonNull<NSEvent>| -> *mut NSEvent {
            let flags = unsafe { event.as_ref() }.modifierFlags();
            super::chord(&local_handle, talk_chord(flags));
            event.as_ptr()
        });
        unsafe {
            let _ = NSEvent::addGlobalMonitorForEventsMatchingMask_handler(NSEventMask::FlagsChanged, &global);
            let _ = NSEvent::addLocalMonitorForEventsMatchingMask_handler(NSEventMask::FlagsChanged, &local);
        }
        std::mem::forget(global);
        std::mem::forget(local);
    }

    pub fn watch_other_keys(app: &AppHandle) {
        let handle = app.clone();
        let block = RcBlock::new(move |_event: NonNull<NSEvent>| super::chord_spoiled(&handle));
        let monitor = NSEvent::addGlobalMonitorForEventsMatchingMask_handler(
            NSEventMask::KeyDown | NSEventMask::LeftMouseDown | NSEventMask::RightMouseDown,
            &block,
        );
        OTHER_KEYS.with(|k| *k.borrow_mut() = monitor);
    }

    pub fn unwatch_other_keys() {
        let monitor = OTHER_KEYS.with(|k| k.borrow_mut().take());
        if let Some(monitor) = monitor {
            unsafe { NSEvent::removeMonitor(&monitor) };
        }
    }

    pub fn chime(name: &str) {
        if let Some(sound) = objc2_app_kit::NSSound::soundNamed(&NSString::from_str(name)) {
            sound.setVolume(0.35);
            sound.play();
        }
    }
}

#[cfg(not(target_os = "macos"))]
mod mac {
    use super::{Ears, Heard};
    use std::sync::{Arc, Mutex};
    use tauri::AppHandle;

    pub fn ears() -> Ears {
        Ears::Refused
    }
    pub fn ask_for_ears(_app: &AppHandle) {}
    pub fn listen(_app: &AppHandle, _id: u64, _heard: &Arc<Mutex<Heard>>, _hints: &[String]) -> Result<(), String> {
        Err("Talking to Vspk works on a Mac.".into())
    }
    pub fn stop_hearing() {}
    pub fn drop_live() {}
    pub fn watch_chord(_app: &AppHandle) {}
    pub fn watch_other_keys(_app: &AppHandle) {}
    pub fn unwatch_other_keys() {}
    pub fn chime(_name: &str) {}
}

pub fn install(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    load_settings(app);
    let win = WebviewWindowBuilder::new(app, LABEL, WebviewUrl::App("voice.html".into()))
        .title("Vspk")
        .decorations(false)
        .transparent(true)
        .always_on_top(true)
        .skip_taskbar(true)
        .focused(false)
        .focusable(false)
        .shadow(false)
        .resizable(false)
        .inner_size(WIDTH, BODY)
        .visible(false)
        .build()?;
    let _ = win.set_ignore_cursor_events(true);

    app.global_shortcut().on_shortcut(hands_free_key(), |app, _, event| {
        if event.state() == ShortcutState::Pressed {
            hands_free(app);
        }
    })?;
    eprintln!(
        "[vans] hotkey Alt+Shift+Space registered = {}",
        app.global_shortcut().is_registered(hands_free_key())
    );
    mac::watch_chord(app);

    let handle = app.clone();
    app.listen(crate::coach::STOP, move |_| {
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || {
            cancel(&main);
            crate::speak::hush(&main);
            quiet(&main);
            set_aloud(false);
        });
    });
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn holding_the_key_listens_until_it_is_let_go() {
        assert_eq!(end_for(Mode::Held, Duration::from_secs(10), Some(Duration::from_secs(5))), None);
        assert_eq!(end_for(Mode::Held, Duration::from_secs(10), None), None);
        assert_eq!(end_for(Mode::Held, LONGEST, None), Some(End::TooLong));
    }

    #[test]
    fn hands_free_listening_ends_when_you_stop_talking() {
        assert_eq!(end_for(Mode::HandsFree, Duration::from_secs(3), Some(Duration::from_millis(500))), None);
        assert_eq!(end_for(Mode::HandsFree, Duration::from_secs(3), Some(QUIET_ENDS)), Some(End::Done));
        assert_eq!(end_for(Mode::HandsFree, Duration::from_secs(2), None), None);
        assert_eq!(end_for(Mode::HandsFree, NOTHING_SAID, None), Some(End::NothingSaid));
        assert_eq!(end_for(Mode::HandsFree, LONGEST, Some(Duration::ZERO)), Some(End::TooLong));
    }

    #[test]
    fn a_guide_hears_next_again_and_stop_and_nothing_longer() {
        assert_eq!(command("Next."), Some(Command::Next));
        assert_eq!(command("next step"), Some(Command::Next));
        assert_eq!(command("Okay, done"), Some(Command::Next));
        assert_eq!(command("Stop."), Some(Command::Stop));
        assert_eq!(command("cancel that"), Some(Command::Stop));
        assert_eq!(command("Again?"), Some(Command::Again));
        assert_eq!(command("say that again"), Some(Command::Again));
        assert_eq!(command("repeat"), Some(Command::Again));
        assert_eq!(command("how do I stop the music playing here"), None);
        assert_eq!(command("crop this"), None);
        assert_eq!(command(""), None);
    }

    #[test]
    fn a_step_is_said_the_way_the_caption_reads() {
        assert_eq!(step_words("click", "Tools", "", true), "Click Tools");
        assert_eq!(step_words("", "", "", true), "Click here");
        assert_eq!(step_words("type", "Search", "groceries", true), "Type groceries");
        assert_eq!(step_words("type", "Search", "", true), "Type in Search");
        assert_eq!(step_words("press", "", "Command S", true), "Press Command S");
        assert_eq!(step_words("scroll", "", "down", true), "Scroll down");
        assert_eq!(step_words("set", "Volume", "30%", true), "Set Volume to 30%");
        assert_eq!(step_words("drag", "Photo", "the album", true), "Drag Photo to the album");
        assert_eq!(step_words("click", "Open System Settings", "", false), "Open System Settings");
    }

    #[test]
    fn the_app_and_window_name_help_hearing_without_repeats() {
        assert_eq!(hints("Preview", "Holiday.jpg"), vec!["Vspk", "Preview", "Holiday.jpg"]);
        assert_eq!(hints("", ""), vec!["Vspk"]);
        assert_eq!(hints("Notes", "notes"), vec!["Vspk", "Notes"]);
        assert_eq!(hints("A", &"x".repeat(200))[2].chars().count(), HINT_CHARS);
    }

    #[test]
    fn the_level_follows_loudness_and_speech_is_above_the_room() {
        assert_eq!(level_of(0.0), 0.0);
        assert_eq!(level_of(1.0), 1.0);
        assert!(level_of(0.05) > level_of(0.005));
        assert!(!speaking_rms(0.001));
        assert!(speaking_rms(0.05));
    }

    #[test]
    fn the_level_is_sent_at_most_every_little_while() {
        let mut heard = Heard::new();
        heard.started -= WARM_UP;
        assert!(heard.sound(0.05).is_some());
        assert!(heard.sound(0.05).is_none());
        assert!(heard.spoke.is_some());
    }

    #[test]
    fn the_first_moment_of_sound_is_not_taken_for_talking() {
        let mut heard = Heard::new();
        assert!(heard.sound(0.2).is_some());
        assert!(heard.spoke.is_none());
    }

    #[test]
    fn hands_free_shows_how_close_the_quiet_is_to_ending_it() {
        assert_eq!(quiet_for(Mode::HandsFree, None), 0.0);
        assert_eq!(quiet_for(Mode::Held, Some(Duration::from_secs(1))), 0.0);
        assert!((quiet_for(Mode::HandsFree, Some(QUIET_ENDS / 2)) - 0.5).abs() < 0.01);
        assert_eq!(quiet_for(Mode::HandsFree, Some(QUIET_ENDS * 3)), 1.0);
    }

    #[test]
    fn settings_start_on_with_option_space_and_read_old_files() {
        let s = Settings::default();
        assert!(s.on && s.aloud && s.sounds && s.key == Key::OptionSpace);
        let old: Settings = serde_json::from_str(r#"{"on":false}"#).unwrap();
        assert!(!old.on && old.aloud && old.sounds && old.key == Key::OptionSpace);
        let chord: Settings = serde_json::from_str(r#"{"key":"control-option"}"#).unwrap();
        assert_eq!(chord.key, Key::ControlOption);
    }
}
