import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import sarvam  # noqa: E402
from guide import commands_by_name  # noqa: E402



def parse(raw):
    say, steps = sarvam.parse(raw)
    return say, [x["text"] for x in steps]


def plain(*texts):
    return [{"text": t, "target": None, "app": None} for t in texts]


GOOD = {"say": "Here is how.", "steps": ["Open Finder", "Click File", "Choose New Folder"]}


class ParseTest(unittest.TestCase):
    def test_plain_json(self):
        self.assertEqual(parse(json.dumps(GOOD)), ("Here is how.", GOOD["steps"]))

    def test_code_fence(self):
        self.assertEqual(parse("```json\n" + json.dumps(GOOD) + "\n```"), ("Here is how.", GOOD["steps"]))

    def test_text_around_json(self):
        raw = "Sure! Here you go:\n" + json.dumps(GOOD) + "\nHope that helps."
        self.assertEqual(parse(raw), ("Here is how.", GOOD["steps"]))

    def test_braces_after_json(self):
        raw = json.dumps(GOOD) + "\nNote: shortcuts look like {Cmd}+{S}."
        self.assertEqual(parse(raw), ("Here is how.", GOOD["steps"]))

    def test_think_block(self):
        raw = "<think>The user wants {steps}. Let me plan.</think>\n" + json.dumps(GOOD)
        self.assertEqual(parse(raw), ("Here is how.", GOOD["steps"]))

    def test_think_without_opening_tag(self):
        raw = "Planning with {braces} first.</think>" + json.dumps(GOOD)
        self.assertEqual(parse(raw), ("Here is how.", GOOD["steps"]))

    def test_trailing_comma(self):
        raw = '{"say": "Here is how.", "steps": ["Open Finder", "Click File", "Choose New Folder",],}'
        self.assertEqual(parse(raw), ("Here is how.", GOOD["steps"]))

    def test_numbered_steps_inside_json(self):
        raw = json.dumps({"say": "Ok.", "steps": ["1. Open Finder", "Step 2: Click File", "- Done"]})
        self.assertEqual(parse(raw), ("Ok.", ["Open Finder", "Click File", "Done"]))

    def test_steps_as_objects(self):
        raw = json.dumps({"say": "Ok.", "steps": [{"step": "Open Finder"}, {"text": "Click File"}]})
        self.assertEqual(parse(raw), ("Ok.", ["Open Finder", "Click File"]))

    def test_at_most_six_steps(self):
        raw = json.dumps({"say": "Ok.", "steps": [f"Step {i}" for i in range(1, 10)]})
        self.assertEqual(len(parse(raw)[1]), 6)

    def test_plain_numbered_list_with_intro(self):
        raw = "Here is how.\n1. Open Finder\n2. Click File\n3. Choose New Folder"
        self.assertEqual(parse(raw), ("Here is how.", GOOD["steps"]))

    def test_plain_numbered_list_without_intro(self):
        raw = "1. Open Finder\n2. Click File\n3. Choose New Folder"
        self.assertEqual(parse(raw), ("", GOOD["steps"]))

    def test_inline_numbered_list(self):
        raw = "Sure, rename it. 1. Right-click the folder. 2. Click Rename. 3. Type the name."
        self.assertEqual(parse(raw), ("Sure, rename it.", ["Right-click the folder.", "Click Rename.", "Type the name."]))

    def test_empty(self):
        self.assertEqual(parse(None), ("", []))

    def test_hindi(self):
        raw = "```\n" + json.dumps({"say": "ऐसे करें।", "steps": ["फाइंडर खोलें"]}, ensure_ascii=False) + "\n```"
        self.assertEqual(parse(raw), ("ऐसे करें।", ["फाइंडर खोलें"]))


class StepShapeTest(unittest.TestCase):
    def test_object_steps_keep_target_and_app(self):
        raw = json.dumps({"say": "Ok.", "steps": [
            {"text": "Open System Settings", "target": "System Settings", "app": "System Settings"},
            {"text": "Click Wallpaper", "target": "Wallpaper", "app": "System Settings"},
        ]})
        self.assertEqual(sarvam.parse(raw)[1], [
            {"text": "Open System Settings", "target": "System Settings", "app": "System Settings"},
            {"text": "Click Wallpaper", "target": "Wallpaper", "app": "System Settings"},
        ])

    def test_hindi_text_english_target(self):
        raw = json.dumps({"say": "ऐसे करें।", "steps": [{"text": "ब्लूटूथ पर क्लिक करें", "target": "Bluetooth", "app": "System Settings"}]}, ensure_ascii=False)
        self.assertEqual(sarvam.parse(raw)[1], [{"text": "ब्लूटूथ पर क्लिक करें", "target": "Bluetooth", "app": "System Settings"}])

    def test_old_string_steps_become_objects(self):
        self.assertEqual(sarvam.parse(json.dumps(GOOD))[1], plain(*GOOD["steps"]))

    def test_plain_list_becomes_objects(self):
        self.assertEqual(sarvam.parse("1. Open Finder\n2. Click File")[1], plain("Open Finder", "Click File"))

    def test_mixed_steps(self):
        raw = json.dumps({"say": "Ok.", "steps": ["Open Finder", {"text": "Click Go", "target": "Go", "app": "Finder"}]})
        self.assertEqual(sarvam.parse(raw)[1], [*plain("Open Finder"), {"text": "Click Go", "target": "Go", "app": "Finder"}])

    def test_junk_targets_dropped(self):
        for junk in ["", "   ", "null", "None", "N/A", "-", 5, ["Wallpaper"], {"x": 1}, "x" * 60, "Cmd + Shift + 4", "⌘⇧4", "Line one\nline two", "Command", "shift", "5"]:
            raw = json.dumps({"say": "Ok.", "steps": [{"text": "Do it", "target": junk, "app": junk}]})
            self.assertEqual(sarvam.parse(raw)[1], plain("Do it"), junk)

    def test_target_tidied(self):
        cases = {
            '"Wallpaper"': "Wallpaper",
            " Bluetooth. ": "Bluetooth",
            "Apple menu > System Settings": "Apple",
            "File → Save": "File",
            "Empty Trash button": "Empty Trash",
            "View menu": "View",
        }
        for given, want in cases.items():
            raw = json.dumps({"say": "Ok.", "steps": [{"text": "Do it", "target": given, "app": "Finder"}]})
            self.assertEqual(sarvam.parse(raw)[1][0]["target"], want, given)

    def test_step_without_text_dropped(self):
        raw = json.dumps({"say": "Ok.", "steps": [{"target": "Wallpaper"}, {"text": "  "}, None, "Open Finder"]})
        self.assertEqual(sarvam.parse(raw)[1], plain("Open Finder"))

    def test_object_steps_capped_at_six(self):
        raw = json.dumps({"say": "Ok.", "steps": [{"text": f"Step {i}", "target": "Go"} for i in range(1, 10)]})
        self.assertEqual(len(sarvam.parse(raw)[1]), 6)

    def test_wrong_script_reads_object_text(self):
        steps = [{"text": "இதைத் திறக்கவும்", "target": "Finder", "app": "Finder"}]
        self.assertFalse(sarvam.wrong_script("இப்படி செய்யுங்கள்.", steps, "ta-IN"))
        self.assertTrue(sarvam.wrong_script("Here is how.", plain("Open it"), "ta-IN"))


class LanguageTest(unittest.TestCase):
    def test_script_beats_wrong_tag(self):
        self.assertEqual(sarvam.script_language("ஸ்கிரீன்ஷாட் எப்படி எடுப்பது?", "en-IN"), "ta-IN")
        self.assertEqual(sarvam.script_language("স্ক্রিনশট কিভাবে নেব?", "en-IN"), "bn-IN")
        self.assertEqual(sarvam.script_language("స్క్రీన్‌షాట్ ఎలా తీయాలి?", None), "te-IN")

    def test_devanagari_keeps_marathi(self):
        self.assertEqual(sarvam.script_language("स्क्रीनशॉट कसा घ्यायचा?", "mr-IN"), "mr-IN")
        self.assertEqual(sarvam.script_language("स्क्रीनशॉट कैसे लें?", "en-IN"), "hi-IN")

    def test_latin_keeps_given(self):
        self.assertEqual(sarvam.script_language("screenshot kaise le?", "hi-IN"), "hi-IN")
        self.assertEqual(sarvam.script_language("how?", "xx"), "en-IN")

    def test_wrong_script(self):
        self.assertTrue(sarvam.wrong_script("Here is how.", plain("Open it"), "ta-IN"))
        self.assertFalse(sarvam.wrong_script("இப்படி செய்யுங்கள்.", plain("Finder திறக்கவும்"), "ta-IN"))
        self.assertFalse(sarvam.wrong_script("Here is how.", [], "en-IN"))


class HistoryTest(unittest.TestCase):
    def test_turns_become_messages(self):
        turns = [{"question": "How do I copy?", "say": "Like this.", "steps": ["Select text", "Press Cmd C"]}]
        self.assertEqual(
            sarvam.history_messages(turns),
            [
                {"role": "user", "content": "How do I copy?"},
                {"role": "assistant", "content": json.dumps({"say": "Like this.", "steps": plain("Select text", "Press Cmd C")})},
            ],
        )

    def test_turns_with_object_steps(self):
        turns = [{"question": "Wallpaper?", "say": "Ok.", "steps": [
            {"text": "Open System Settings", "target": "System Settings", "app": "System Settings"},
            "Click Wallpaper",
        ]}]
        a = json.loads(sarvam.history_messages(turns)[1]["content"])
        self.assertEqual(a["steps"], [
            {"text": "Open System Settings", "target": "System Settings", "app": "System Settings"},
            *plain("Click Wallpaper"),
        ])

    def test_bad_turns_ignored(self):
        self.assertEqual(sarvam.history_messages("nope"), [])
        self.assertEqual(sarvam.history_messages([1, {"answer": "x"}]), [])

    def test_plain_answer_turn_is_json(self):
        msgs = sarvam.history_messages([{"question": "q", "answer": "Do it."}])
        self.assertEqual(json.loads(msgs[1]["content"]), {"say": "Do it.", "steps": []})

    def test_only_last_turns(self):
        turns = [{"question": f"q{i}", "answer": "a"} for i in range(10)]
        self.assertEqual(len(sarvam.history_messages(turns)), 2 * sarvam.MAX_TURNS)

    def test_prompt_has_name_and_language(self):
        p = sarvam.system_prompt("ta-IN")
        self.assertIn(sarvam.APP_NAME, p)
        self.assertIn("Tamil", p)
        self.assertNotIn("{", p.replace('{"say"', "").replace('{"text"', ""))

    def test_prompt_asks_for_target_and_app(self):
        p = sarvam.system_prompt("hi-IN")
        self.assertIn('"target"', p)
        self.assertIn('"app"', p)
        self.assertIn("Mac", p)
        self.assertIn("JSON", p)


class AskRouteTest(unittest.TestCase):
    def setUp(self):
        import guide

        self.guide = guide
        self.saved = (guide.think, guide.db)
        inserted = self.inserted = []

        class Asks:
            def insert_one(self, doc):
                inserted.append(doc)
                return type("R", (), {"inserted_id": "abc"})()

            def find(self, *a):
                rows = [{"question": "old", "language": "en-IN", "say": "Ok.", "steps": ["Open Finder"], "at": __import__("datetime").datetime(2026, 1, 1)}]
                return type("C", (), {"sort": lambda s, *a: s, "limit": lambda s, n: rows})()

        guide.db = lambda: type("D", (), {"asks": Asks()})()
        guide.think = lambda q, lang, turns=None: ("Ok.", [{"text": "Click Wallpaper", "target": "Wallpaper", "app": "System Settings"}])
        from app import app

        self.client = app.test_client()

    def tearDown(self):
        self.guide.think, self.guide.db = self.saved

    def test_ask_returns_objects_and_text(self):
        out = self.client.post("/api/ask", json={"text": "wallpaper?", "language": "en-IN"}).get_json()
        self.assertEqual(out["steps"], [{"text": "Click Wallpaper", "target": "Wallpaper", "app": "System Settings"}])
        self.assertEqual(out["steps_text"], ["Click Wallpaper"])
        self.assertEqual(sorted(out), ["at", "id", "language", "question", "say", "steps", "steps_text"])

    def test_history_turns_old_rows_into_objects(self):
        rows = self.client.get("/api/history").get_json()
        self.assertEqual(rows[0]["steps"], plain("Open Finder"))
        self.assertEqual(rows[0]["steps_text"], ["Open Finder"])


class CommandsTest(unittest.TestCase):
    def test_shape_and_languages(self):
        words = commands_by_name()
        self.assertEqual(sorted(words), ["again", "back", "next", "stop"])
        for w in ["next", "अगला", "அடுத்து", "తర్వాత", "পরের", "पुढे"]:
            self.assertIn(w, words["next"])


if __name__ == "__main__":
    unittest.main()
