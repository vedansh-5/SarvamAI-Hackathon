import json
from datetime import datetime, timezone
from pathlib import Path

from flask import Blueprint, jsonify, request

from brand import APP_NAME
from db import db
from sarvam import DEFAULT_LANGUAGE, script_language, think

guide_bp = Blueprint("guide", __name__)

COMMANDS_FILE = Path(__file__).parent / "data" / "commands.json"
COMMAND_NAMES = ("next", "back", "again", "stop")


def commands_by_name():
    words = {name: [] for name in COMMAND_NAMES}
    for lang in json.loads(COMMANDS_FILE.read_text()).values():
        for name in COMMAND_NAMES:
            for w in lang.get(name, []):
                if w not in words[name]:
                    words[name].append(w)
    return words


@guide_bp.post("/api/ask")
def ask():
    body = request.get_json(force=True, silent=True) or {}
    question = str(body.get("text") or "").strip()
    if not question:
        return jsonify(error="Ask something first."), 400
    language = script_language(question, body.get("language") or DEFAULT_LANGUAGE)
    try:
        say, steps = think(question, language, body.get("turns"))
    except Exception as e:
        return jsonify(error=f"{APP_NAME} couldn't think that through: {e}"), 502
    if not say and not steps:
        return jsonify(error=f"{APP_NAME} couldn't think that through. Try asking again."), 502
    doc = {
        "question": question,
        "language": language,
        "say": say,
        "steps": steps,
        "at": datetime.now(timezone.utc),
    }
    doc["id"] = str(db().asks.insert_one(dict(doc)).inserted_id)
    doc["at"] = doc["at"].isoformat()
    return jsonify(doc)


@guide_bp.get("/api/history")
def history():
    rows = db().asks.find({}, {"_id": 0}).sort("at", -1).limit(20)
    return jsonify([{**r, "at": r["at"].isoformat()} for r in rows])


@guide_bp.get("/api/commands")
def commands():
    return jsonify(commands_by_name())
