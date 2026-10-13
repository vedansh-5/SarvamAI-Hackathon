# Privacy

VANS is in private testing, and this page changes as VANS does.

## The short version

- VANS has no account, no sign-up and no server of its own. There is nothing of yours on a VANS computer, because there isn't one.
- VANS looks at your screen only when you ask it something, and only at what the question is about.
- To answer, it sends your request and the relevant text to a model provider, straight from your Mac. The panel tells you when it has done so.
- What VANS remembers is kept in one folder on your Mac. You can delete it at any time.
- This website uses no cookies, no analytics and no trackers.

## Who is behind VANS

VANS (Visual Agent Navigation System) is made by an independent developer. It is not yet a registered company.

## This website

- **No cookies and no tracking.** The site sets no cookies and loads nothing from other companies. Fonts are served from the site itself. Your choice of light or dark is saved in your own browser and never sent anywhere.
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
- **A picture of one window,** only for the few applications that do not describe their own controls. VANS tells you before a run that it will look at the window.

VANS does not watch your screen in the background, record your keystrokes, or read anything between requests.

### What leaves your Mac

To understand a request and plan the steps, VANS sends these to a model provider over an encrypted connection:

- the words of your request;
- the part of what it read that the request needs: the window's controls, the page text, the selected text, the file names, or the picture of the window.

This goes directly from your Mac to the model provider. It does not pass through any server run by VANS. When the text a window was showing has been sent to answer you, the panel says so in the answer.

In the current build the model provider is Google, through the Gemini API. The provider processes what it receives under its own terms and privacy policy. If the provider changes, this page will say so before the change reaches you.

Nothing is sold, and nothing is used for advertising.

### What VANS keeps, and where

Everything VANS remembers is in one folder on your Mac, `~/.vans`. It holds:

- **Your last three finished requests,** so you can run them again. Older ones are not kept.
- **A record of what VANS changed,** such as files it moved, so that each change can be undone.
- **What you asked it to keep:** routines, facts, schedules, and your rules about what it must never touch.
- **How a kind of request was handled,** so the same kind is faster next time. These are stored as patterns with your own words, names and numbers taken out.
- **What it has learned about where things are in an application,** so a guide does not have to work it out twice.

The key that lets VANS call the model provider is kept in the macOS Keychain, not in that folder.

### Removing it

- A remembered request, a routine or a fact can each be removed from the panel.
- Deleting the folder `~/.vans` removes everything VANS has kept. Nothing remains anywhere else, because nothing was stored anywhere else.

### Permissions VANS asks macOS for

- **Accessibility,** to read a window's controls and, when you confirm, to click and type.
- **Automation,** to work with applications that support it, such as Notes and Reminders.
- **Screen Recording,** only if you want VANS to be able to look at windows that do not describe their own controls.

You can withdraw any of these in System Settings, under Privacy & Security.

## What VANS will not do

- Change a file, a setting or an application without you holding the key to confirm.
- Open or switch applications on its own.
- Run anything in the background that you did not ask for.
- Collect usage statistics. The app contains no analytics.

## Children

VANS is not directed at children under 13, and I do not knowingly collect information from them.

## Your rights

What VANS keeps is on your own Mac and under your control. For anything I hold myself, which is only email you have sent me and your address if you joined the Windows list, you can ask to see it, correct it or have it deleted by writing to the address on the main page.

## Changes to this page

When what VANS reads, sends or keeps changes, this page changes with it.
