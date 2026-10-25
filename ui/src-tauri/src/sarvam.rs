use std::io::Write;
use std::os::unix::fs::OpenOptionsExt;
use std::process::Command;
use std::sync::atomic::{AtomicU32, Ordering};
use std::sync::Mutex;

const HEAR_URL: &str = "https://api.sarvam.ai/speech-to-text";
const HEAR_MODEL: &str = "saaras:v3";
const MOST_SECONDS: usize = 28;
const SEND_RATE: u32 = 16_000;

static PCM: Mutex<Vec<f32>> = Mutex::new(Vec::new());
static RATE: AtomicU32 = AtomicU32::new(SEND_RATE);
static LANGUAGE: Mutex<String> = Mutex::new(String::new());

pub struct Heard {
    pub text: String,
    pub language: String,
}

pub fn key() -> Option<String> {
    if let Ok(k) = std::env::var("SARVAM_API_KEY") {
        if !k.trim().is_empty() {
            return Some(k.trim().to_string());
        }
    }
    let path = std::path::Path::new(env!("CARGO_MANIFEST_DIR")).join("../../.env");
    let text = std::fs::read_to_string(path).ok()?;
    text.lines()
        .find_map(|l| l.strip_prefix("SARVAM_API_KEY="))
        .map(|v| v.trim().trim_matches('"').to_string())
        .filter(|v| !v.is_empty())
}

pub fn on() -> bool {
    key().is_some()
}

pub fn language() -> String {
    LANGUAGE.lock().unwrap().clone()
}

pub fn begin(rate: f64) {
    RATE.store(rate.round().max(1.0) as u32, Ordering::SeqCst);
    PCM.lock().unwrap().clear();
}

pub fn push(samples: &[f32]) {
    let most = RATE.load(Ordering::SeqCst) as usize * MOST_SECONDS;
    let mut pcm = PCM.lock().unwrap();
    if pcm.len() < most {
        pcm.extend_from_slice(samples);
    }
}

fn wav() -> Option<Vec<u8>> {
    let pcm = std::mem::take(&mut *PCM.lock().unwrap());
    let rate = RATE.load(Ordering::SeqCst);
    let (step, out_rate) = if rate % SEND_RATE == 0 { ((rate / SEND_RATE) as usize, SEND_RATE) } else { (1, rate) };
    let samples: Vec<i16> = pcm.iter().step_by(step.max(1)).map(|s| (s.clamp(-1.0, 1.0) * 32767.0) as i16).collect();
    if samples.len() < out_rate as usize / 4 {
        return None;
    }
    let data = (samples.len() * 2) as u32;
    let mut out = Vec::with_capacity(44 + data as usize);
    out.extend_from_slice(b"RIFF");
    out.extend_from_slice(&(36 + data).to_le_bytes());
    out.extend_from_slice(b"WAVEfmt ");
    out.extend_from_slice(&16u32.to_le_bytes());
    out.extend_from_slice(&1u16.to_le_bytes());
    out.extend_from_slice(&1u16.to_le_bytes());
    out.extend_from_slice(&out_rate.to_le_bytes());
    out.extend_from_slice(&(out_rate * 2).to_le_bytes());
    out.extend_from_slice(&2u16.to_le_bytes());
    out.extend_from_slice(&16u16.to_le_bytes());
    out.extend_from_slice(b"data");
    out.extend_from_slice(&data.to_le_bytes());
    for s in samples {
        out.extend_from_slice(&s.to_le_bytes());
    }
    Some(out)
}

fn private_file(path: &std::path::Path, bytes: &[u8]) -> std::io::Result<()> {
    let mut f = std::fs::OpenOptions::new().write(true).create(true).truncate(true).mode(0o600).open(path)?;
    f.write_all(bytes)
}

pub fn hear() -> Result<Heard, String> {
    let key = key().ok_or("No Sarvam key.")?;
    let audio = wav().ok_or("Nothing was recorded.")?;
    let dir = std::env::temp_dir().join(format!("vans-hear-{}", std::process::id()));
    std::fs::create_dir_all(&dir).map_err(|e| e.to_string())?;
    let sound = dir.join("speech.wav");
    let header = dir.join("header");
    private_file(&sound, &audio).map_err(|e| e.to_string())?;
    private_file(&header, format!("api-subscription-key: {key}\n").as_bytes()).map_err(|e| e.to_string())?;
    let out = Command::new("/usr/bin/curl")
        .args(["-sS", "--max-time", "20", "-H"])
        .arg(format!("@{}", header.display()))
        .arg("-F")
        .arg(format!("file=@{};type=audio/wav", sound.display()))
        .args(["-F", &format!("model={HEAR_MODEL}"), "-F", "mode=translate", HEAR_URL])
        .output();
    let _ = std::fs::remove_dir_all(&dir);
    let out = out.map_err(|e| e.to_string())?;
    let body: serde_json::Value = serde_json::from_slice(&out.stdout)
        .map_err(|_| format!("Sarvam answered oddly: {}", String::from_utf8_lossy(&out.stderr)))?;
    let Some(text) = body.get("transcript").and_then(|t| t.as_str()) else {
        return Err(format!("Sarvam couldn't hear that: {body}"));
    };
    let language = body.get("language_code").and_then(|l| l.as_str()).unwrap_or("en-IN").to_string();
    *LANGUAGE.lock().unwrap() = language.clone();
    Ok(Heard { text: text.trim().to_string(), language })
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn a_recording_becomes_a_16k_mono_wav() {
        begin(48_000.0);
        push(&vec![0.1; 48_000]);
        let w = wav().unwrap();
        assert_eq!(&w[0..4], b"RIFF");
        assert_eq!(u32::from_le_bytes(w[24..28].try_into().unwrap()), 16_000);
        assert_eq!(w.len(), 44 + 16_000 * 2);
    }

    #[test]
    fn a_click_is_not_sent() {
        begin(48_000.0);
        push(&[0.1; 100]);
        assert!(wav().is_none());
    }
}
