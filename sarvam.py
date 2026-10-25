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


def _step_text(s):
    if isinstance(s, dict):
        for key in ("step", "text", "action", "title"):
            if s.get(key):
                return _clean(s[key])
        return ""
    return _clean(s)


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
        steps = out.get("steps") or []
        if isinstance(steps, str):
            steps = steps.splitlines()
        steps = [t for t in (_step_text(s) for s in steps) if t]
        return str(out.get("say") or "").strip(), steps[:MAX_STEPS]
    text = re.sub(r"(?<=\S)\s+(?=\d+\s*[.)]\s)", "\n", text)
    lines = [l.strip() for l in text.splitlines() if l.strip()]
    listed = [l for l in lines if re.match(LIST_MARK, l, flags=re.I)]
    if listed:
        say = " ".join(l for l in lines if l not in listed)
        steps = [t for t in (_clean(l) for l in listed) if t]
    else:
        say, steps = (lines[0] if lines else ""), lines[1:]
    return say.strip(), steps[:MAX_STEPS]


def history_messages(history):
    out = []
    if not isinstance(history, list):
        return out
    for t in history[-MAX_TURNS:]:
        if not isinstance(t, dict):
            continue
        q = str(t.get("question") or t.get("text") or "").strip()[:500]
        steps = t.get("steps") if isinstance(t.get("steps"), list) else []
        say = str(t.get("say") or t.get("answer") or "").strip()[:600]
        a = json.dumps({"say": say, "steps": [str(x)[:200] for x in steps[:MAX_STEPS]]}, ensure_ascii=False)
        if q:
            out.append({"role": "user", "content": q})
            out.append({"role": "assistant", "content": a})
    return out


def wrong_script(say, steps, language):
    if language not in LANGUAGES or language == DEFAULT_LANGUAGE:
        return False
    found = script_language(" ".join([say, *steps]), DEFAULT_LANGUAGE)
    return found != language and not (language in DEVANAGARI and found in DEVANAGARI)


def system_prompt(language):
    return PROMPT_FILE.read_text().format(
        name=APP_NAME,
        language=LANGUAGES.get(language, LANGUAGES[DEFAULT_LANGUAGE]),
        max_steps=MAX_STEPS,
    )


def think(question, language, history=None):
    messages = [
        {"role": "system", "content": system_prompt(language)},
        *history_messages(history),
        {"role": "user", "content": question},
    ]
    say, steps = "", []
    for _ in range(max(1, CHAT_TRIES)):
        out = client().chat.completions(model=CHAT_MODEL, messages=messages, temperature=CHAT_TEMPERATURE)
        say, steps = parse(out.choices[0].message.content)
        if (say or steps) and not wrong_script(say, steps, language):
            break
    return say, steps
