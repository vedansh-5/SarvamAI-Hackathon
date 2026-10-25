import shutil
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(ROOT))

try:
    from jinja2 import Environment, FileSystemLoader
except ModuleNotFoundError:
    for p in ROOT.glob(".venv/lib/python*/site-packages"):
        if p.exists():
            sys.path.append(str(p))
            break
    from jinja2 import Environment, FileSystemLoader

from brand import APP_NAME, EMOJI, TAGLINE

site_dir = ROOT / "site"
dist_dir = ROOT / "dist"
dist_site = dist_dir / "site"

dist_dir.mkdir(exist_ok=True)
dist_site.mkdir(exist_ok=True)

env = Environment(loader=FileSystemLoader(str(site_dir)), autoescape=True)
template = env.get_template("index.html")

html = template.render(
    app_name=APP_NAME,
    tagline=TAGLINE,
    emoji=EMOJI,
    static_build=True,
)

(dist_dir / "index.html").write_text(html, encoding="utf-8")

shutil.copy2(site_dir / "site.css", dist_site / "site.css")

dist_img = dist_site / "img"
if (site_dir / "img").exists():
    if dist_img.exists():
        shutil.rmtree(dist_img)
    shutil.copytree(site_dir / "img", dist_img)
