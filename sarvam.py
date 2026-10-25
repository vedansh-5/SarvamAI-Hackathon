import httpx
import base64
import json
import os
import re
from pathlib import Path

from sarvamai import SarvamAI

from brand import APP_NAME

HERE = Path(__file__).parent
env_file = HERE / ".env"
if env_file.exists():
    for line in env_file.read_text().splitlines():
        key, sep, value = line.partition("=")
        if sep and key.strip() and not key.startswith("#"):
            os.environ.setdefault(key.strip(), value.strip())

CHAT_MODEL = os.environ.get("APP_CHAT_MODEL", "sarvam-105b-conversations")
STT_MODEL = os.environ.get("APP_STT_MODEL", "saaras:v3")
TTS_MODEL = os.environ.get("APP_TTS_MODEL", "bulbul:v3")
TTS_SPEAKER = os.environ.get("APP_TTS_SPEAKER", "priya")
CHAT_TEMPERATURE = float(os.environ.get("APP_CHAT_TEMPERATURE", "0.2"))
CHAT_TRIES = int(os.environ.get("APP_CHAT_TRIES", "2"))
MAX_STEPS = 6
MAX_TURNS = 4
MIN_AUDIO_BYTES = 1200
SPEAK_LIMIT = 1400
DEFAULT_LANGUAGE = "en-IN"
PROMPT_FILE = HERE / "data" / "prompt.txt"

LANGUAGES = {
    "en-IN": "English",
    "hi-IN": "Hindi",
    "bn-IN": "Bengali",
    "gu-IN": "Gujarati",
    "kn-IN": "Kannada",
    "ml-IN": "Malayalam",
    "mr-IN": "Marathi",
    "od-IN": "Odia",
    "pa-IN": "Punjabi",
    "ta-IN": "Tamil",
    "te-IN": "Telugu",
}
TTS_LANGUAGES = set(LANGUAGES)

SCRIPTS = [
    ("bn-IN", "ঀ", "৿"),
    ("pa-IN", "਀", "੿"),
    ("gu-IN", "઀", "૿"),
    ("od-IN", "଀", "୿"),
    ("ta-IN", "஀", "௿"),
    ("te-IN", "ఀ", "౿"),
    ("kn-IN", "ಀ", "೿"),
    ("ml-IN", "ഀ", "ൿ"),
    ("hi-IN", "ऀ", "ॿ"),
]
DEVANAGARI = ("hi-IN", "mr-IN")
LIST_MARK = r"^\s*(?:step\s*)?(?:\d+\s*[.):-]|[-*•])\s*"
MAX_LABEL = 48
JUNK_LABELS = {"", "null", "none", "n/a", "na", "nil", "-", "--", "?", "...", "undefined", "target", "app", "label", "button"}
KEY_NAMES = r"(?:cmd|command|ctrl|control|shift|option|opt|alt|fn|tab|esc|escape|return|enter|space|delete)"
BARE_KEYS = {"cmd", "command", "ctrl", "control", "shift", "option", "opt", "alt", "fn", "esc", "escape"}

_client = None


def client():
    global _client
    if _client is None:
        key = os.environ.get("SARVAM_API_KEY")
        if not key:
            raise RuntimeError("SARVAM_API_KEY is not set. Add it to .env.")
        _client = SarvamAI(api_subscription_key=key)
    return _client


def speakable(language):
    return language if language in TTS_LANGUAGES else DEFAULT_LANGUAGE


def script_language(text, given=None):
    counts = {}
    for ch in text or "":
        for code, lo, hi in SCRIPTS:
            if lo <= ch <= hi:
                counts[code] = counts.get(code, 0) + 1
                break
    if not counts:
        return given if given in LANGUAGES else DEFAULT_LANGUAGE
    code = max(counts, key=counts.get)
    if code == "hi-IN" and given in DEVANAGARI:
        return given
    return code


def hear(audio, mime="audio/wav"):
    if len(audio) < MIN_AUDIO_BYTES:
        return "", DEFAULT_LANGUAGE
    ext = (mime.split("/")[-1] or "wav").split(";")[0]
    out = client().speech_to_text.transcribe(
        file=(f"speech.{ext}", audio, mime),
        model=STT_MODEL,
        language_code="unknown",
    )
    return (out.transcript or "").strip(), out.language_code or DEFAULT_LANGUAGE


def speak(text, language):
    out = client().text_to_speech.convert(
        text=text[:SPEAK_LIMIT],
        language_code=speakable(language),
        speaker=TTS_SPEAKER,
        model=TTS_MODEL,
    )
    return b"".join(base64.b64decode(a) for a in out.audios or [])


def _clean(s):
    return re.sub(LIST_MARK, "", str(s), flags=re.I).strip()


def _label(value):
    if not isinstance(value, str):
        return None
    t = value.strip().strip("\"'`“”‘’").strip()
    t = re.split(r"\s*(?:>|→|»|->|›)\s*", t)[0].strip()
    t = re.sub(r"\s*(?:button|menu item|menu)$", "", t, flags=re.I).strip() if " " in t else t
    t = t.rstrip(".,;:!").strip()
    if t.lower() in JUNK_LABELS or t.lower() in BARE_KEYS or len(t) < 2 or len(t) > MAX_LABEL or "\n" in t:
        return None
    if re.search(r"[⌘⇧⌥⌃]", t) or re.search(rf"\b{KEY_NAMES}\s*\+|\+\s*{KEY_NAMES}\b", t, flags=re.I):
        return None
    return t


def _step_text(s):
    if isinstance(s, dict):
        for key in ("text", "step", "action", "title"):
            if isinstance(s.get(key), str) and s[key].strip():
                return _clean(s[key])
        return ""
    if s is None or isinstance(s, (list, bool)):
        return ""
    return _clean(s)


def step(s):
    text = _step_text(s)
    if not text:
        return None
    target = app = None
    if isinstance(s, dict):
        target = _label(s.get("target") or s.get("click") or s.get("label") or s.get("button"))
        app = _label(s.get("app"))
    return {"text": text, "target": target, "app": app}


def steps_of(items):
    if isinstance(items, str):
        items = items.splitlines()
    if not isinstance(items, list):
        return []
    return [x for x in (step(s) for s in items) if x][:MAX_STEPS]


def texts(steps):
    return [s["text"] for s in steps]


def _strip_thinking(raw):
    text = re.sub(r"<think>.*?</think>", "", raw or "", flags=re.S | re.I)
    text = re.split(r"</think>", text, flags=re.I)[-1]
    text = re.split(r"<think>", text, flags=re.I)[0]
    return text.strip()


def _objects(text):
    decoder = json.JSONDecoder()
    for m in re.finditer(r"\{", text):
        chunk = text[m.start():]
        for attempt in (chunk, re.sub(r",\s*([}\]])", r"\1", chunk)):
            try:
                obj, _ = decoder.raw_decode(attempt)
            except json.JSONDecodeError:
                continue
            if isinstance(obj, dict):
                yield obj
            break


def parse(raw):
    text = re.sub(r"```[a-zA-Z]*", "", _strip_thinking(raw))
    for out in _objects(text):
        if "say" not in out and "steps" not in out:
            continue
        return str(out.get("say") or "").strip(), steps_of(out.get("steps") or [])
    text = re.sub(r"(?<=\S)\s+(?=\d+\s*[.)]\s)", "\n", text)
    lines = [l.strip() for l in text.splitlines() if l.strip()]
    listed = [l for l in lines if re.match(LIST_MARK, l, flags=re.I)]
    if listed:
        say = " ".join(l for l in lines if l not in listed)
        steps = [t for t in (_clean(l) for l in listed) if t]
    else:
        say, steps = (lines[0] if lines else ""), lines[1:]
    return say.strip(), steps_of(steps)


def history_messages(history):
    out = []
    if not isinstance(history, list):
        return out
    for t in history[-MAX_TURNS:]:
        if not isinstance(t, dict):
            continue
        q = str(t.get("question") or t.get("text") or "").strip()[:500]
        steps = steps_of(t.get("steps") if isinstance(t.get("steps"), list) else [])
        say = str(t.get("say") or t.get("answer") or "").strip()[:600]
        steps = [{**x, "text": x["text"][:200]} for x in steps]
        a = json.dumps({"say": say, "steps": steps}, ensure_ascii=False)
        if q:
            out.append({"role": "user", "content": q})
            out.append({"role": "assistant", "content": a})
    return out


def wrong_script(say, steps, language):
    if language not in LANGUAGES or language == DEFAULT_LANGUAGE:
        return False
    found = script_language(" ".join([say, *texts(steps)]), DEFAULT_LANGUAGE)
    return found != language and not (language in DEVANAGARI and found in DEVANAGARI)


def system_prompt(language):
    return PROMPT_FILE.read_text().format(
        name=APP_NAME,
        language=LANGUAGES.get(language, LANGUAGES[DEFAULT_LANGUAGE]),
        max_steps=MAX_STEPS,
    )


THINK_ORDER = [p.strip() for p in os.environ.get("APP_THINK_ORDER", "gemini,groq").split(",") if p.strip()]
THINK_TIMEOUT = float(os.environ.get("APP_THINK_TIMEOUT", "6"))


def _remote_chat(provider, messages):
    prefix = provider.upper()
    key = os.environ.get(f"{prefix}_API_KEY")
    base = os.environ.get(f"{prefix}_BASE_URL")
    model = (os.environ.get(f"{prefix}_MODELS") or "").split(",")[0].strip()
    if not (key and base and model):
        raise RuntimeError(f"{provider} is not set up")
    r = httpx.post(
        base.rstrip("/") + "/chat/completions",
        headers={"Authorization": f"Bearer {key}"},
        json={"model": model, "messages": messages, "temperature": CHAT_TEMPERATURE},
        timeout=THINK_TIMEOUT,
    )
    r.raise_for_status()
    return r.json()["choices"][0]["message"]["content"]


def _chat(provider, messages):
    if provider == "sarvam":
        out = client().chat.completions(model=CHAT_MODEL, messages=messages, temperature=CHAT_TEMPERATURE)
        return out.choices[0].message.content
    return _remote_chat(provider, messages)


def think(question, language, history=None):
    messages = [
        {"role": "system", "content": system_prompt(language)},
        *history_messages(history),
        {"role": "user", "content": question},
    ]
    say, steps, last = "", [], None
    for provider in THINK_ORDER:
        try:
            say, steps = parse(_chat(provider, messages))
        except Exception as e:
            last = e
            continue
        if (say or steps) and not wrong_script(say, steps, language):
            return say, steps
    if not (say or steps) and last:
        raise last
    return say, steps
