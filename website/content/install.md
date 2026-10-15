# Installing VANS

VANS is in private testing. If you have a download link and an invite code from Vedansh, this page takes you from the download to your first question.

## What you need

- A Mac. The test build runs on Apple silicon and on Intel.
- The download link and the invite code. The link works for seven days; ask for a new one if it has run out.

## Install it

1. Open the link. It downloads a file called `VANS_` followed by the version, ending in `.dmg`.
2. Open that file and drag VANS into Applications.
3. Eject the VANS disk in the Finder sidebar.

## Open it the first time

The test build is signed by its developer but not yet checked by Apple, so the first time you open it macOS stops it. You allow it once.

1. Open VANS from Applications. macOS says it could not verify VANS. Press Done.
2. Open System Settings, go to Privacy & Security, and scroll down to Security. Next to the line saying VANS was blocked, press Open Anyway and confirm with your password or Touch ID.
3. Open VANS again, and press Open Anyway.

On macOS 14 or earlier you can instead Control-click VANS in Applications, choose Open, and press Open.

From then on VANS opens like any other app.

## Set it up

VANS opens its Setup screen the first time.

1. **Permissions.** Press Allow all and answer each macOS prompt as it comes up. Accessibility and Screen Recording can only be switched on by you, in System Settings: VANS takes you there and comes back when they are on.
2. **Invite code.** Paste the code under Invite code and press Save and check.

VANS sits at the top of your screen, as a small dot in the middle of the menu bar. Click the dot, or press `⌥Space`, and ask how to do something.

## Updates

VANS checks for a new test build a few times a day and installs it in the background. When one is ready, a line below the dot says so: press Restart, or Later to keep working.

## Learning

VANS learns what you do often, so it can offer it at the moment you usually do it. It is on from the start; one switch in Setup, under Learning, turns it off. The [privacy page](/privacy) says what it notes and what it never does.

## Removing VANS

1. Press Quit VANS in Setup, under Start.
2. Drag VANS from Applications to the Bin.
3. Delete the folder `.vans` in your home folder, which holds everything VANS kept on your Mac. In Finder, press `⇧⌘G`, enter `~` and press Return; press `⇧⌘.` to show hidden folders, and drag `.vans` to the Bin.
4. Your invite code stays in the Keychain until you delete it: open Keychain Access and search for `dev.vedansh.vans`.

Tell Vedansh when you stop testing, and your invite code is switched off.
