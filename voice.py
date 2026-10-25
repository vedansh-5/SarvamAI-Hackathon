import base64

from flask import Blueprint, jsonify, request

from brand import APP_NAME
from sarvam import DEFAULT_LANGUAGE, hear, speak

voice_bp = Blueprint("voice", __name__)


@voice_bp.post("/api/listen")
def listen():
    audio = request.files.get("audio")
    if not audio:
        return jsonify(error="No audio was sent."), 400
    mime = (audio.mimetype or "audio/wav").split(";")[0]
    try:
        text, language = hear(audio.read(), mime)
    except Exception as e:
        return jsonify(error=f"{APP_NAME} couldn't hear that: {e}"), 502
    return jsonify(text=text, language=language)


@voice_bp.post("/api/speak")
def speak_route():
    body = request.get_json(force=True, silent=True) or {}
    text = str(body.get("text") or "").strip()
    if not text:
        return jsonify(error="Nothing to say."), 400
    try:
        wav = speak(text, body.get("language") or DEFAULT_LANGUAGE)
    except Exception as e:
        return jsonify(error=f"{APP_NAME} couldn't speak that: {e}"), 502
    if not wav:
        return jsonify(error="No audio came back."), 502
    return jsonify(audio=base64.b64encode(wav).decode(), mime="audio/wav")
