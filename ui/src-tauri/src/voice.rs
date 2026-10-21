use serde_json::json;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex};
use std::time::{Duration, Instant};
use tauri::{AppHandle, Emitter, Listener, LogicalPosition, Manager, WebviewUrl, WebviewWindow, WebviewWindowBuilder};
use tauri_plugin_global_shortcut::{Code, GlobalShortcutExt, Modifiers, Shortcut, ShortcutState};

use crate::screen::ScreenContext;

pub const LABEL: &str = "voice";
pub const HEARD: &str = "voice://heard";
const SAYS: &str = "voice://says";
const WORDS: &str = "voice://words";
const LEVEL: &str = "voice://level";
const LISTENING: &str = "pill://listening";

const WIDTH: f64 = 440.0;
const HEIGHT: f64 = 72.0;
const GAP: f64 = 6.0;

const HOLD: Duration = Duration::from_millis(280);
const QUIET_ENDS: Duration = Duration::from_millis(1400);
const NOTHING_SAID: Duration = Duration::from_secs(6);
const LONGEST: Duration = Duration::from_secs(30);
const LAST_WORDS: Duration = Duration::from_millis(1800);
const LEVEL_EVERY: Duration = Duration::from_millis(60);
const WATCH: Duration = Duration::from_millis(100);
const SPEAKING_DB: f32 = -42.0;

const NO_EARS: &str = "VANS can't hear you. Allow Microphone and Speech Recognition for VANS in System Settings › Privacy & Security.";
const ON_THIS_MAC: &str = "VANS listens on this Mac only. Turn on Dictation in System Settings › Keyboard so your language is on this Mac, then try again.";

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
    if words.iter().any(|w| matches!(*w, "next" | "continue" | "done" | "onward")) {
        return Some(Command::Next);
    }
    None
}

fn level_of(rms: f32) -> f32 {
    let db = 20.0 * rms.max(1e-6).log10();
    ((db + 55.0) / 45.0).clamp(0.0, 1.0)
}

fn speaking(rms: f32) -> bool {
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
        if speaking(rms) {
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
static SAID: AtomicU64 = AtomicU64::new(0);
static HOLDING_ESC: AtomicBool = AtomicBool::new(false);

fn hands_free_key() -> Shortcut {
    Shortcut::new(Some(Modifiers::ALT | Modifiers::SHIFT), Code::Space)
}

fn esc() -> Shortcut {
    Shortcut::new(None, Code::Escape)
}

fn window(app: &AppHandle) -> Option<WebviewWindow> {
    app.get_webview_window(LABEL)
}

fn show_capsule(app: &AppHandle) {
    let Some(win) = window(app) else { return };
    if let Some((x, y)) = crate::notch::under_the_bar(WIDTH, GAP) {
        let _ = win.set_position(LogicalPosition::new(x, y));
    }
    let _ = win.show();
    crate::notch::sit_in_the_menu_bar(&win);
    let _ = win.set_ignore_cursor_events(true);
}

fn hide_capsule(app: &AppHandle) {
    if let Some(win) = window(app) {
        let _ = win.hide();
    }
}

fn tell(app: &AppHandle, state: &str, words: &str) -> u64 {
    let n = SAID.fetch_add(1, Ordering::SeqCst) + 1;
    let _ = app.emit_to(LABEL, SAYS, json!({ "state": state, "words": words }));
    show_capsule(app);
    n
}

fn tell_then_go(app: &AppHandle, state: &str, words: &str, after: Duration) {
    let n = tell(app, state, words);
    let handle = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(after);
        if SAID.load(Ordering::SeqCst) != n || SESSION.lock().unwrap().is_some() {
            return;
        }
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || {
            if SAID.load(Ordering::SeqCst) == n {
                hide_capsule(&main);
            }
        });
    });
}

fn listening_light(app: &AppHandle, on: bool) {
    let _ = app.emit_to(crate::notch::LABEL, LISTENING, on);
}

fn hold_esc(app: &AppHandle) {
    if app.global_shortcut().is_registered(esc()) || HOLDING_ESC.swap(true, Ordering::SeqCst) {
        return;
    }
    let registered = app.global_shortcut().on_shortcut(esc(), |app, _, event| {
        if event.state() == ShortcutState::Pressed {
            let main = app.clone();
            let _ = app.run_on_main_thread(move || cancel(&main));
        }
    });
    if let Err(e) = registered {
        HOLDING_ESC.store(false, Ordering::SeqCst);
        eprintln!("[vans] could not take Escape while listening: {e}");
    }
}

fn release_esc(app: &AppHandle) {
    if HOLDING_ESC.swap(false, Ordering::SeqCst) {
        let _ = app.global_shortcut().unregister(esc());
    }
}

pub fn key_down(app: &AppHandle) {
    if KEY_DOWN.swap(true, Ordering::SeqCst) {
        return;
    }
    let press = PRESS.fetch_add(1, Ordering::SeqCst) + 1;
    let handle = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(HOLD);
        if !KEY_DOWN.load(Ordering::SeqCst) || PRESS.load(Ordering::SeqCst) != press {
            return;
        }
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || {
            if KEY_DOWN.load(Ordering::SeqCst) && PRESS.load(Ordering::SeqCst) == press {
                HELD.store(true, Ordering::SeqCst);
                start(&main, Mode::Held);
            }
        });
    });
}

pub fn key_up(app: &AppHandle) -> bool {
    KEY_DOWN.store(false, Ordering::SeqCst);
    let hands_free = SESSION.lock().unwrap().as_ref().is_some_and(|s| s.mode == Mode::HandsFree);
    if HELD.swap(false, Ordering::SeqCst) || hands_free {
        let main = app.clone();
        let _ = app.run_on_main_thread(move || finish_current(&main));
        return false;
    }
    true
}

pub fn hands_free(app: &AppHandle) {
    let main = app.clone();
    let _ = app.run_on_main_thread(move || {
        if SESSION.lock().unwrap().is_some() {
            finish_current(&main);
        } else {
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
            tell(app, "note", "Allow VANS to hear you, then try again.");
            mac::ask_for_ears(app);
            return;
        }
        Ears::Refused => {
            tell_then_go(app, "note", NO_EARS, Duration::from_secs(6));
            return;
        }
        Ears::Ready => {}
    }

    let guide = crate::coach::is_showing();
    let context = crate::screen::capture_active_context(false);
    let id = NEXT_ID.fetch_add(1, Ordering::SeqCst);
    let heard = Arc::new(Mutex::new(Heard::new()));
    if let Err(why) = mac::listen(app, id, &heard, &context.app_name) {
        tell_then_go(app, "note", &why, Duration::from_secs(6));
        return;
    }
    *SESSION.lock().unwrap() = Some(Session { id, mode, guide, context, heard: heard.clone() });
    eprintln!("[vans] listening ({mode:?}{})", if guide { ", during a guide" } else { "" });
    hold_esc(app);
    listening_light(app, true);
    tell(app, "listening", if guide { "Say next or stop" } else { "" });
    watch(app, id, mode, heard);
}

fn watch(app: &AppHandle, id: u64, mode: Mode, heard: Arc<Mutex<Heard>>) {
    let handle = app.clone();
    std::thread::spawn(move || loop {
        std::thread::sleep(WATCH);
        if SESSION.lock().unwrap().as_ref().map(|s| s.id) != Some(id) {
            return;
        }
        let end = {
            let h = heard.lock().unwrap();
            if h.ending {
                return;
            }
            end_for(mode, h.started.elapsed(), h.spoke.map(|t| t.elapsed()))
        };
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
    release_esc(app);
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

    let words = session.heard.lock().unwrap().words.trim().to_string();
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
            None => tell_then_go(app, "note", "During a guide, say \"next\" or \"stop\".", Duration::from_millis(2500)),
        }
        return;
    }
    let _ = app.emit_to("main", HEARD, json!({ "text": words, "context": session.context }));
    tell_then_go(app, "heard", &words, Duration::from_millis(1200));
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
    hide_capsule(app);
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

pub enum Ears {
    Ready,
    Ask,
    Refused,
}

fn after_asking(app: &AppHandle) {
    match mac::ears() {
        Ears::Ready => tell_then_go(app, "note", "VANS can hear you now. Hold ⌥Space and talk.", Duration::from_secs(3)),
        _ => tell_then_go(app, "note", NO_EARS, Duration::from_secs(6)),
    }
}

#[cfg(target_os = "macos")]
mod mac {
    use super::{Ears, Heard, ON_THIS_MAC};
    use block2::RcBlock;
    use objc2::rc::Retained;
    use objc2::runtime::Bool;
    use objc2::AnyThread;
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

    fn rms(buffer: &AVAudioPCMBuffer) -> f32 {
        unsafe {
            let frames = buffer.frameLength() as usize;
            let channels = buffer.floatChannelData();
            if channels.is_null() || frames == 0 {
                return 0.0;
            }
            let samples = std::slice::from_raw_parts((*channels).as_ptr(), frames);
            (samples.iter().map(|s| s * s).sum::<f32>() / frames as f32).sqrt()
        }
    }

    pub fn listen(app: &AppHandle, id: u64, heard: &Arc<Mutex<Heard>>, app_name: &str) -> Result<(), String> {
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
            let mut hints = vec![NSString::from_str("VANS")];
            if !app_name.is_empty() {
                hints.push(NSString::from_str(app_name));
            }
            request.setContextualStrings(&NSArray::from_retained_slice(&hints));

            let engine = AVAudioEngine::new();
            let input = engine.inputNode();
            let format = input.outputFormatForBus(0);
            if format.channelCount() == 0 {
                return Err("VANS can't find a microphone.".into());
            }

            let feed = request.clone();
            let loud_app = app.clone();
            let loud = heard.clone();
            let tap: Tap = RcBlock::new(move |buffer: NonNull<AVAudioPCMBuffer>, _when: NonNull<AVAudioTime>| {
                let buffer = buffer.as_ref();
                feed.appendAudioPCMBuffer(buffer);
                super::level_now(&loud_app, &loud, rms(buffer));
            });
            input.installTapOnBus_bufferSize_format_block(0, 1024, Some(&format), RcBlock::as_ptr(&tap));

            engine.prepare();
            if let Err(e) = engine.startAndReturnError() {
                input.removeTapOnBus(0);
                return Err(format!("VANS couldn't start the microphone: {}", e.localizedDescription()));
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
    pub fn listen(_app: &AppHandle, _id: u64, _heard: &Arc<Mutex<Heard>>, _app_name: &str) -> Result<(), String> {
        Err("Talking to VANS works on a Mac.".into())
    }
    pub fn stop_hearing() {}
    pub fn drop_live() {}
}

pub fn install(app: &AppHandle) -> Result<(), Box<dyn std::error::Error>> {
    let win = WebviewWindowBuilder::new(app, LABEL, WebviewUrl::App("voice.html".into()))
        .title("VANS")
        .decorations(false)
        .transparent(true)
        .always_on_top(true)
        .skip_taskbar(true)
        .focused(false)
        .focusable(false)
        .shadow(false)
        .resizable(false)
        .inner_size(WIDTH, HEIGHT)
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

    let handle = app.clone();
    app.listen(crate::coach::STOP, move |_| {
        let main = handle.clone();
        let _ = handle.run_on_main_thread(move || cancel(&main));
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
    fn a_guide_hears_next_and_stop_and_nothing_longer() {
        assert_eq!(command("Next."), Some(Command::Next));
        assert_eq!(command("next step"), Some(Command::Next));
        assert_eq!(command("Okay, done"), Some(Command::Next));
        assert_eq!(command("Stop."), Some(Command::Stop));
        assert_eq!(command("cancel that"), Some(Command::Stop));
        assert_eq!(command("how do I stop the music playing here"), None);
        assert_eq!(command("crop this"), None);
        assert_eq!(command(""), None);
    }

    #[test]
    fn the_level_follows_loudness_and_speech_is_above_the_room() {
        assert_eq!(level_of(0.0), 0.0);
        assert_eq!(level_of(1.0), 1.0);
        assert!(level_of(0.05) > level_of(0.005));
        assert!(!speaking(0.001));
        assert!(speaking(0.05));
    }

    #[test]
    fn the_level_is_sent_at_most_every_little_while() {
        let mut heard = Heard::new();
        assert!(heard.sound(0.05).is_some());
        assert!(heard.sound(0.05).is_none());
        assert!(heard.spoke.is_some());
    }
}
