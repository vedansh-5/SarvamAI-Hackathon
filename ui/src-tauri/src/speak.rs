use serde_json::json;
use std::sync::atomic::{AtomicU64, Ordering};
use tauri::{AppHandle, Emitter};

const MOST: usize = 280;
const SPOKEN: &str = "voice://spoken";

static SAYING: AtomicU64 = AtomicU64::new(0);

fn without_links(text: &str) -> String {
    let mut out = String::with_capacity(text.len());
    let mut rest = text;
    while let Some(open) = rest.find('[') {
        let after = &rest[open + 1..];
        let Some(close) = after.find("](") else { break };
        let Some(end) = after[close + 2..].find(')') else { break };
        out.push_str(&rest[..open]);
        out.push_str(&after[..close]);
        rest = &after[close + 2 + end + 1..];
    }
    out.push_str(rest);
    out.split_whitespace()
        .filter(|w| !w.starts_with("http://") && !w.starts_with("https://") && !w.starts_with("www."))
        .filter(|w| !matches!(*w, "-" | "•" | "*" | "–"))
        .collect::<Vec<_>>()
        .join(" ")
}

pub fn for_the_ear(text: &str) -> String {
    let plain: String = without_links(&text.replace('\n', ". "))
        .chars()
        .filter(|c| !matches!(c, '*' | '#' | '`' | '_' | '>' | '|' | '~'))
        .collect();
    let mut words = plain.split_whitespace().collect::<Vec<_>>().join(" ");
    while words.contains(". .") || words.contains("..") {
        words = words.replace(". .", ".").replace("..", ".");
    }
    let words = words.trim_matches(|c: char| c == '.' || c.is_whitespace()).to_string();
    if words.chars().count() <= MOST {
        return words;
    }
    let mut said = String::new();
    for sentence in words.split_inclusive(['.', '?', '!']) {
        if !said.is_empty() && said.chars().count() + sentence.chars().count() > MOST {
            break;
        }
        said.push_str(sentence);
    }
    if said.chars().count() > MOST {
        let mut cut = String::new();
        for word in said.split_whitespace() {
            if cut.chars().count() + word.chars().count() + 1 > MOST {
                break;
            }
            if !cut.is_empty() {
                cut.push(' ');
            }
            cut.push_str(word);
        }
        said = cut;
    }
    said.trim().to_string()
}

pub fn say(app: &AppHandle, text: &str) {
    let words = for_the_ear(text);
    if words.is_empty() {
        return;
    }
    let n = SAYING.fetch_add(1, Ordering::SeqCst) + 1;
    let handle = app.clone();
    let _ = app.run_on_main_thread(move || {
        if SAYING.load(Ordering::SeqCst) != n {
            return;
        }
        if mac::on_a_call() {
            eprintln!("[vans] another app has the microphone; showing instead of saying");
            crate::voice::shown_not_said(&handle, &words);
            return;
        }
        crate::voice::speaking(&handle, &words);
        mac::speak(&handle, &words);
    });
}

pub fn hush(app: &AppHandle) {
    SAYING.fetch_add(1, Ordering::SeqCst);
    let _ = app.run_on_main_thread(mac::stop);
}

fn word_at(app: &AppHandle, from: usize, to: usize) {
    let _ = app.emit_to(crate::voice::LABEL, SPOKEN, json!({ "from": from, "to": to }));
}

fn finished(app: &AppHandle) {
    let main = app.clone();
    let _ = app.run_on_main_thread(move || crate::voice::quiet(&main));
}

#[cfg(target_os = "macos")]
mod mac {
    use objc2::rc::Retained;
    use objc2::runtime::ProtocolObject;
    use objc2::{define_class, msg_send, AnyThread, DefinedClass};
    use objc2_avf_audio::{AVSpeechBoundary, AVSpeechSynthesizer, AVSpeechSynthesizerDelegate, AVSpeechUtterance};
    use objc2_foundation::{NSObject, NSObjectProtocol, NSRange, NSString};
    use std::cell::RefCell;
    use std::sync::atomic::{AtomicUsize, Ordering};
    use tauri::AppHandle;

    static CURRENT: AtomicUsize = AtomicUsize::new(0);

    fn is_current(utterance: &AVSpeechUtterance) -> bool {
        CURRENT.load(Ordering::SeqCst) == utterance as *const AVSpeechUtterance as usize
    }

    pub struct Ivars {
        app: AppHandle,
    }

    define_class!(
        #[unsafe(super(NSObject))]
        #[name = "VansSpeechListener"]
        #[ivars = Ivars]
        pub struct Listener;

        unsafe impl NSObjectProtocol for Listener {}

        unsafe impl AVSpeechSynthesizerDelegate for Listener {
            #[unsafe(method(speechSynthesizer:willSpeakRangeOfSpeechString:utterance:))]
            fn will_speak(&self, _synth: &AVSpeechSynthesizer, range: NSRange, utterance: &AVSpeechUtterance) {
                if is_current(utterance) {
                    super::word_at(&self.ivars().app, range.location, range.location + range.length);
                }
            }

            #[unsafe(method(speechSynthesizer:didFinishSpeechUtterance:))]
            fn did_finish(&self, _synth: &AVSpeechSynthesizer, utterance: &AVSpeechUtterance) {
                if is_current(utterance) {
                    super::finished(&self.ivars().app);
                }
            }

            #[unsafe(method(speechSynthesizer:didCancelSpeechUtterance:))]
            fn did_cancel(&self, _synth: &AVSpeechSynthesizer, utterance: &AVSpeechUtterance) {
                if is_current(utterance) {
                    super::finished(&self.ivars().app);
                }
            }
        }
    );

    impl Listener {
        fn new(app: &AppHandle) -> Retained<Self> {
            let this = Self::alloc().set_ivars(Ivars { app: app.clone() });
            unsafe { msg_send![super(this), init] }
        }
    }

    struct Voice {
        synth: Retained<AVSpeechSynthesizer>,
        _listener: Retained<Listener>,
    }

    thread_local! {
        static VOICE: RefCell<Option<Voice>> = const { RefCell::new(None) };
    }

    fn synth(app: &AppHandle) -> Retained<AVSpeechSynthesizer> {
        VOICE.with(|v| {
            v.borrow_mut()
                .get_or_insert_with(|| {
                    let synth = unsafe { AVSpeechSynthesizer::new() };
                    let listener = Listener::new(app);
                    unsafe { synth.setDelegate(Some(ProtocolObject::from_ref(&*listener))) };
                    Voice { synth, _listener: listener }
                })
                .synth
                .clone()
        })
    }

    pub fn speak(app: &AppHandle, words: &str) {
        let synth = synth(app);
        unsafe {
            let utterance = AVSpeechUtterance::speechUtteranceWithString(&NSString::from_str(words));
            utterance.setPrefersAssistiveTechnologySettings(true);
            CURRENT.store(&*utterance as *const AVSpeechUtterance as usize, Ordering::SeqCst);
            synth.stopSpeakingAtBoundary(AVSpeechBoundary::Immediate);
            synth.speakUtterance(&utterance);
        }
    }

    pub fn stop() {
        CURRENT.store(0, Ordering::SeqCst);
        VOICE.with(|v| {
            if let Some(voice) = v.borrow().as_ref() {
                unsafe { voice.synth.stopSpeakingAtBoundary(AVSpeechBoundary::Immediate) };
            }
        });
    }

    pub fn on_a_call() -> bool {
        use objc2_av_foundation::{AVCaptureDevice, AVMediaTypeAudio};
        unsafe {
            AVMediaTypeAudio
                .and_then(|audio| AVCaptureDevice::defaultDeviceWithMediaType(audio))
                .is_some_and(|mic| mic.isInUseByAnotherApplication())
        }
    }
}

#[cfg(not(target_os = "macos"))]
mod mac {
    use tauri::AppHandle;

    pub fn speak(_app: &AppHandle, _words: &str) {}
    pub fn stop() {}
    pub fn on_a_call() -> bool {
        false
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn speech_leaves_out_links_and_marks() {
        assert_eq!(for_the_ear("See **the docs** at https://example.com now."), "See the docs at now");
        assert_eq!(for_the_ear("Open [the guide](https://x.y/z) first."), "Open the guide first");
        assert_eq!(for_the_ear("# Title\n- one\n- two"), "Title. one. two");
    }

    #[test]
    fn a_long_answer_is_cut_at_a_sentence() {
        let long = "First sentence here. ".repeat(30);
        let said = for_the_ear(&long);
        assert!(said.chars().count() <= MOST, "{} chars", said.chars().count());
        assert!(said.ends_with('.'));
        let one = "word ".repeat(200);
        assert!(for_the_ear(&one).chars().count() <= MOST);
    }

    #[test]
    fn nothing_to_say_says_nothing() {
        assert_eq!(for_the_ear("  \n "), "");
        assert_eq!(for_the_ear("https://only.a.link"), "");
    }
}
