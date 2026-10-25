# Demo script

Three questions, about two minutes. Hindi first, then English, then Tamil.

## Before you start

- MongoDB running (`docker start vspk-mongo`) and the server running
  (`.venv/bin/python app.py`).
- Mac app open, its icon in the menu bar. Sound on, mic allowed.
- Backup: the same questions work on http://127.0.0.1:5050/try (hold Space or
  type them in the box).

## 1. Hindi

**Say (hold ⌥K):** "मैक पर स्क्रीनशॉट कैसे लूँ?"

**What should happen**
- The panel at the top shows "listening", then your words, then the steps.
- A short Hindi sentence, then 2 to 4 Hindi steps (for example
  "Command + Shift + 3 दबाएं").
- The first step is read out in Hindi.

**Then say:** "अगला" and the next step is read out. Say "फिर से" to hear it
again, "रुको" to stop.

## 2. English, with a follow-up

**Say:** "How do I make a new folder on my Mac desktop?"

**What should happen**
- Up to 6 short English steps: right-click the desktop, choose New Folder,
  type a name, press Return.
- The first step is read out.

**Then say:** "And how do I rename it?"

**What should happen:** the answer carries on from the folder you just made
instead of starting over. This shows the app remembers the last few answers.
On `/try`, "Start fresh" clears that memory.

## 3. Tamil

**Say:** "மேக்கில் வைஃபை எப்படி இணைப்பது?" (How do I connect to Wi-Fi on
a Mac?)

**What should happen**
- The language is picked up as Tamil with nothing to set.
- A Tamil sentence and Tamil steps, read out in Tamil.
- Say "அடுத்து" for the next step, "நிறுத்து" to stop.

## If something goes wrong

- Nothing heard: hold the key a moment longer and speak after the panel
  shows "listening".
- Slow answer: the panel shows the thinking dots; answers usually take under
  a second, speech about one second more.
- Press Esc to stop speaking and close the panel at any time.
