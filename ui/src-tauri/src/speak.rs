use std::sync::atomic::{AtomicU64, Ordering};
use std::sync::mpsc;
use std::time::Duration;
use tauri::AppHandle;

const MOST: usize = 280;
const FIRST_LOOK: Duration = Duration::from_millis(450);
const LOOK: Duration = Duration::from_millis(150);

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
        mac::speak(&words);
        crate::voice::speaking(&handle, &words);
        until_quiet(handle, n);
    });
}

pub fn hush(app: &AppHandle) {
    SAYING.fetch_add(1, Ordering::SeqCst);
    let _ = app.run_on_main_thread(mac::stop);
}

fn until_quiet(app: AppHandle, n: u64) {
    std::thread::spawn(move || {
        std::thread::sleep(FIRST_LOOK);
        loop {
            if SAYING.load(Ordering::SeqCst) != n {
                return;
            }
            let (tx, rx) = mpsc::channel();
            let _ = app.run_on_main_thread(move || {
                let _ = tx.send(mac::speaking());
            });
            if !rx.recv_timeout(Duration::from_secs(1)).unwrap_or(false) {
                break;
            }
            std::thread::sleep(LOOK);
        }
        if SAYING.load(Ordering::SeqCst) == n {
            crate::voice::quiet(&app);
        }
    });
}

#[cfg(target_os = "macos")]
mod mac {
    use objc2::rc::Retained;
    use objc2_avf_audio::{AVSpeechBoundary, AVSpeechSynthesizer, AVSpeechUtterance};
    use objc2_foundation::NSString;
    use std::cell::RefCell;

    thread_local! {
        static SYNTH: RefCell<Option<Retained<AVSpeechSynthesizer>>> = const { RefCell::new(None) };
    }

    fn synth() -> Retained<AVSpeechSynthesizer> {
        SYNTH.with(|s| s.borrow_mut().get_or_insert_with(|| unsafe { AVSpeechSynthesizer::new() }).clone())
    }

    pub fn speak(words: &str) {
        let synth = synth();
        unsafe {
            synth.stopSpeakingAtBoundary(AVSpeechBoundary::Immediate);
            let utterance = AVSpeechUtterance::speechUtteranceWithString(&NSString::from_str(words));
            utterance.setPrefersAssistiveTechnologySettings(true);
            synth.speakUtterance(&utterance);
        }
    }

    pub fn stop() {
        SYNTH.with(|s| {
            if let Some(synth) = s.borrow().as_ref() {
                unsafe { synth.stopSpeakingAtBoundary(AVSpeechBoundary::Immediate) };
            }
        });
    }

    pub fn speaking() -> bool {
        SYNTH.with(|s| s.borrow().as_ref().is_some_and(|synth| unsafe { synth.isSpeaking() }))
    }
}

#[cfg(not(target_os = "macos"))]
mod mac {
    pub fn speak(_words: &str) {}
    pub fn stop() {}
    pub fn speaking() -> bool {
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
