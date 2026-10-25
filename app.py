from pathlib import Path

from flask import Blueprint, Flask, jsonify, render_template, request
from werkzeug.exceptions import HTTPException

import sarvam  # noqa: F401
from brand import APP_NAME, EMOJI, TAGLINE, brand
from guide import guide_bp
from voice import voice_bp

SITE = Path(__file__).parent / "site"

app = Flask(__name__)
app.register_blueprint(voice_bp)
app.register_blueprint(guide_bp)
app.register_blueprint(Blueprint("site", __name__, template_folder="site", static_folder="site", static_url_path="/site"))


@app.context_processor
def inject_brand():
    return {"app_name": APP_NAME, "tagline": TAGLINE, "emoji": EMOJI}


@app.errorhandler(HTTPException)
def http_error(e):
    if request.path.startswith("/api/"):
        return jsonify(error=e.description), e.code
    return e


@app.errorhandler(Exception)
def server_error(e):
    if request.path.startswith("/api/"):
        return jsonify(error=f"{APP_NAME} hit a problem: {e}"), 500
    raise e


@app.get("/")
def index():
    if (SITE / "index.html").exists():
        return render_template("index.html")
    return render_template("try.html")


@app.get("/try")
def try_page():
    return render_template("try.html")


@app.get("/api/brand")
def brand_route():
    return jsonify(brand())


@app.get("/api/health")
def health():
    return {"ok": True}


if __name__ == "__main__":
    app.run(host="127.0.0.1", port=5050, debug=True)
