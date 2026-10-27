# The five-user test (Phase 20.16)

**This matters more than any feature.** Before announcing EinCast, sit with five real people, say nothing,
and write down every moment they hesitate. Fix the top three problems, then announce.

It cannot be done by the maintainer alone, or by software. This page is the script.

## Who

Five people who have **not** seen EinCast: family, friends, and **at least two from the Burmese community**.
At least one who is not technical. At least one who will use the TV or tablet rather than the computer.
Do not choose people who will be polite instead of honest.

## Before

- A current release build on a clean user account (or after **Settings → Reset EinCast…**), so the
  first-run wizard shows.
- A folder with ~10 ordinary files: a couple of MP4s, one MKV, a song, a few photos, one subtitle file.
- A phone or tablet and, if possible, a TV, on the same Wi-Fi.
- Pen and paper, or a timer. No screen recording unless they agree.

## The rules for you

1. **Say almost nothing.** Read the task aloud. If they ask "what do I do?", answer "what would you try?"
2. Do not point, do not click for them, do not explain while they are stuck. Count to ten first.
3. Note **what they do**, not what they say they would do. Write down every pause longer than five seconds,
   every wrong click, every "huh?", every time they read something twice.
4. Do not defend the app. Say thank you.

## The tasks (read one at a time)

| # | Task | You are watching for |
|---|---|---|
| 1 | "This is a program that lets you watch videos from this computer on other screens. Open it." | Do they understand the Gatekeeper/SmartScreen warning? Is the first screen clear? |
| 2 | "Make it share the folder called *(name)*." | Do they find the folder button? Do they understand scanning vs sharing? |
| 3 | "Now watch one of the videos on this phone." | QR code: do they know to scan it? Do they type the address instead? Does the token page confuse them? |
| 4 | "Play the video that is an MKV." | Waiting for conversion; is it clear something is happening? |
| 5 | "Turn on the subtitles." | Can they find them? |
| 6 | "Close the app's window. Can the phone still play?" | Do they understand it keeps sharing? Can they get the window back? (On macOS closing hides the window by default; on Windows it quits unless they turned the setting on. Note what they *expected*.) |
| 7 | "Imagine you want to stop sharing for the night. How?" | Start/Stop clarity. |
| 8 | *(optional, TV)* "Now watch on the TV." | Browser or DLNA path; was either understood? |
| 9 | "Something is broken. What would you do to tell me?" | Do they find Settings → Help and feedback? |

For the Burmese-speaking participants, run it once in Burmese and note every string that reads wrong or
unnatural. **Their corrections to `my.json` are as valuable as the usability notes.**

## After each session (two minutes)

Ask three questions, then stop:
1. What was the most confusing moment?
2. What did you expect to happen that did not?
3. Would you use this? What for? (Or why not?)

## Record

Copy this table for each person.

| | |
|---|---|
| Date, OS, version | |
| Person (age band, technical level, language) | |
| Task where they hesitated longest | |
| Wrong clicks / dead ends | |
| Words they did not understand | |
| Said "I would use it for…" | |

## Then

- Tally across the five. A problem **two or more** people hit is real.
- Pick the **top three**, file each as an issue with the notes, and fix them before launch.
- Record the rest as issues without fixing them.
- Put the outcome in [PLAN-v1.1.md](PLAN-v1.1.md) under Phase 20: who, when, what changed. Be honest if fewer than five were done.
