import json
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

import sarvam  # noqa: E402
from guide import commands_by_name  # noqa: E402

parse = sarvam.parse
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
        self.assertTrue(sarvam.wrong_script("Here is how.", ["Open it"], "ta-IN"))
        self.assertFalse(sarvam.wrong_script("இப்படி செய்யுங்கள்.", ["Finder திறக்கவும்"], "ta-IN"))
        self.assertFalse(sarvam.wrong_script("Here is how.", [], "en-IN"))


class HistoryTest(unittest.TestCase):
    def test_turns_become_messages(self):
        turns = [{"question": "How do I copy?", "say": "Like this.", "steps": ["Select text", "Press Cmd C"]}]
        self.assertEqual(
            sarvam.history_messages(turns),
            [
                {"role": "user", "content": "How do I copy?"},
                {"role": "assistant", "content": '{"say": "Like this.", "steps": ["Select text", "Press Cmd C"]}'},
            ],
        )

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
        self.assertNotIn("{", p.replace('{"say"', ""))


class CommandsTest(unittest.TestCase):
    def test_shape_and_languages(self):
        words = commands_by_name()
        self.assertEqual(sorted(words), ["again", "back", "next", "stop"])
        for w in ["next", "अगला", "அடுத்து", "తర్వాత", "পরের", "पुढे"]:
            self.assertIn(w, words["next"])


if __name__ == "__main__":
    unittest.main()
