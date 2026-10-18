# Privacy

VANS is in private testing, and this page changes as VANS does.

## The short version

- VANS has no account and no sign-up. Testers get an invite code instead.
- In the test builds, your requests go through the VANS server, which passes them to a model provider. The server keeps a short record of each request for 30 days, and nothing from your screen.
- VANS looks at your screen only when you ask it something, and only at what the question is about.
- VANS learns what you do often, so it can offer it at the right moment. It is on from the start, and you can switch it off in Setup. It notes the names of buttons you click, never what you type. The notes stay on your Mac; in the test builds, a summary of the habits found goes to the VANS server.
- To answer, it sends your request and the relevant text to the model provider. The panel tells you when it has done so.
- What VANS remembers is kept in one folder on your Mac. You can delete it at any time.
- This website counts visits without cookies. Only if you allow it, it also uses Microsoft Clarity to see how the pages are read, which sets cookies.

## Who is behind VANS

VANS (Visual Agent Navigation System) is made by an independent developer. It is not yet a registered company.

## This website

- **Counting visits.** Cloudflare Web Analytics counts visits to each page, where visitors came from, and their country and kind of device. It sets no cookies and does not follow you to other sites. Fonts are served from the site itself. Your choice of light or dark is saved in your own browser and never sent anywhere.
- **Microsoft Clarity, only if you allow it.** On your first visit the site asks. If you press Allow, it loads Microsoft Clarity, which records how you move through the pages, such as clicks, scrolling and where you stop, so I can see which parts are unclear. It sets cookies, on this site and on Microsoft's, to tell one visit from the next. What you type into a form is hidden from it. Microsoft keeps recordings for 30 days and processes them under its own privacy statement. If you press No thanks, nothing is loaded. Cookie choice, at the bottom of every page, changes your answer.
- **Hosting.** The site is served by Cloudflare. Like any host, Cloudflare processes technical details of each visit, such as your IP address and browser type, in order to deliver the page and keep it secure. I do not use these to identify you.
- **Email.** If you write to an address at summonvans.in, I receive your email address and whatever you choose to tell me. I use it to reply and to arrange testing. I do not add you to a mailing list. Ask me to delete your emails and I will.
- **The Windows list.** If you leave your email in the Windows form, it is sent to me as an email and kept in my inbox. I use it once, to tell you when there is a Windows version to try. It is not stored anywhere else and not shared. Ask me to remove it and I will.

## The VANS app

### What VANS reads on your Mac

VANS reads only when you ask, and only what the request needs:

- **The window you are asking about:** the names, values and positions of its buttons, fields and menus, so it can point at them or act on them.
- **The page in front, or the text you have selected,** when you ask about it.
- **The names of files** in a folder your request is about. VANS does not open your files or read their contents.
- **The names of the applications** you have installed and which are open, so it can tell where a task is done.
- **A picture of one window,** only for the few applications that do not describe their own controls. VANS tells you before a run that it will look at the window. The words in the picture are found on your Mac, by macOS's own text recognition; nothing is downloaded or installed for it.

Apart from learning, described next, VANS does not read anything between requests, and it never records your keystrokes.

### Watching and learning

VANS learns what you do again and again, so it can offer to teach it or do it at the moment you usually do it. It is on when you install VANS, Setup says so, and one switch in Setup › Learning turns it off.

While it is on, VANS notes:

- the name of each button or menu item you click, and the application it is in, such as "Mail · Archive";
- which application comes to the front, and when;
- when your Mac wakes, unlocks, locks and goes to sleep.

It never notes what you type, keystrokes, window titles, the contents of a window, anything inside a web page, or anything in a password manager, and it takes no pictures. Nothing is noted while VANS itself is guiding you or doing steps.

These notes stay on your Mac, in `~/.vans`, and anything older than seven days is deleted. Setup › Learning shows every note, and Clear deletes them. In the test builds, a summary of the habits found from them, not the notes, goes to the VANS server, and "What the VANS server keeps", below, says what is in it.

From the notes, VANS finds habits by counting, on your Mac, with no model involved: clicks you make in the same order, and things you do at the same time of day, after your Mac is opened, or after a particular application comes forward. It offers one with a short line below its dot. Nothing runs unless you choose Run, or you choose Always and do not press Cancel in the five seconds it gives you.

### What leaves your Mac

To understand a request and plan the steps, VANS sends these to a model provider over an encrypted connection:

- the words of your request;
- the part of what it read that the request needs: the window's controls, the page text, the selected text, the file names, or the picture of the window and the words found in it.

In the test builds this goes from your Mac to the VANS server, and from there to a model provider. It passes what you sent to the model and the answer back to you; it does not keep the window's controls, the page text, the selected text, the file names or the picture. When the text a window was showing has been sent to answer you, the panel says so in the answer.

The VANS server runs on Amazon Web Services in Mumbai, India. The model providers are Google (the Gemini API), Groq and Cloudflare (Workers AI). Each provider processes what it receives under its own terms and privacy policy. If the providers change, this page will say so before the change reaches you.

Nothing is sold, and nothing is used for advertising.

### What VANS keeps, and where

Everything VANS remembers is in one folder on your Mac, `~/.vans`. It holds:

- **Your last three finished requests,** so you can run them again. Older ones are not kept.
- **A record of what VANS changed,** such as files it moved, so that each change can be undone.
- **What you asked it to keep:** routines, facts, schedules, and your rules about what it must never touch.
- **How a kind of request was handled,** so the same kind is faster next time. These are stored as patterns with your own words, names and numbers taken out.
- **What it has learned about where things are in an application,** so a guide does not have to work it out twice.

The key that lets VANS call the model provider is kept in the macOS Keychain, not in that folder. In the test builds there is no such key on your Mac; there is your invite code instead.

### What the VANS server keeps

In the test builds the VANS server keeps:

- **Your name and your invite code,** and how many requests the code may make in a day.
- **How many requests you made each day,** so one tester cannot use up everyone's share.
- **A short record of each request:** when, the words you typed (at most 500 characters), which application it was for, whether it worked, how long it took and which model provider answered. It never includes what was on your screen, text you selected, file names or a picture. These records are deleted automatically after 30 days.
- **The habits VANS found, and how you answered them:** for example "Start in Clockify, after you open Mail, seen on 4 days, run 3 times". It is sent when the list changes, and replaces the last one. The notes it was found from are never sent. It is deleted when you ask me, or when testing ends.
- **Feedback you choose to send** from the panel, exactly as shown to you before you press Send. It is also emailed to me.

The server's own logs hold your name and the kind of request, not its words, and are deleted after 14 days. Only I can see any of this. It is used to make VANS work better and to keep within the providers' limits; it is not sold or shared.

### Removing it

- A remembered request, a routine or a fact can each be removed from the panel.
- Switching off Setup › Learning stops new notes at once; Clear deletes the notes and any habit you have not answered.
- Deleting the folder `~/.vans` removes everything VANS has kept on your Mac.
- The records of your requests on the VANS server are deleted on their own after 30 days. Anything the server keeps is deleted sooner if you ask me.

### Permissions VANS asks macOS for

- **Accessibility,** to read a window's controls and, when you confirm, to click and type.
- **Automation,** to work with applications that support it, such as Notes and Reminders.
- **Screen Recording,** only if you want VANS to be able to look at windows that do not describe their own controls.

You can withdraw any of these in System Settings, under Privacy & Security.

## What VANS will not do

- Change a file, a setting or an application without you holding the key to confirm.
- Open or switch applications on its own. The one exception is a habit you chose Always for: VANS says what it is about to do and gives you five seconds to cancel.
- Run anything in the background that you did not ask for, other than the learning described above, which you can switch off.
- Collect usage statistics on your Mac. The app contains no analytics. The only record of use is the one the VANS server keeps, described above.

## Children

VANS is not directed at children under 13, and I do not knowingly collect information from them.

## Your rights

What VANS keeps on your Mac is under your control. For anything I hold myself, which is email you have sent me, your address if you joined the Windows list, and, if you are a tester, what the VANS server keeps, you can ask to see it, correct it or have it deleted by writing to the address on the main page.

## Changes to this page

When what VANS reads, sends or keeps changes, this page changes with it.
