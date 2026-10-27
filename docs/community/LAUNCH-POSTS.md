# Launch posts (Phase 20.17)

One factual post per channel: what it is, what it is **not**, a screenshot, the download, the issue link.
Offer to help install. **Be present for the first week** and answer every comment; a launch with no one
answering is worse than no launch.

Do not post until: the [five-user test](../FIVE-USER-TEST.md) is done and its top three problems are fixed; the
README has a demo GIF and screenshots; a release with passing checksums is published; the Discussions space exists.

Replace `<version>` and attach the screenshot. The Burmese posts are drafts for a native speaker to review.

---

## English: short (Reddit r/selfhosted-style, forums, Hacker News "Show HN")

**Title:** EinCast: turn a folder on your Mac/PC into a media server for your TV and phones (no accounts, free, MIT)

I built EinCast because I wanted to watch a folder on my laptop on the living-room TV without installing
a server or an app on the TV.

**What it is:** a desktop app for macOS and Windows. Pick a folder, scan a QR code on any screen, and play
in the browser. It converts MKV/HEVC for iPhone/iPad (needs ffmpeg), shows subtitles, remembers where you stopped, has
household profiles with optional PINs, can show up as a media server on smart TVs (DLNA), and can be reached from
outside through Tailscale (or a Cloudflare/ngrok tunnel behind a PIN). The window and the player page are in English and Burmese.

**What it is not:** a replacement for Jellyfin or Plex. It does not build a catalogue from the internet, has no
live TV or plugins, runs while the app is open, and has no native TV apps. [When to use which](https://github.com/bkjonathan/ein-cast/blob/main/docs/WHY-NOT-JELLYFIN.md).

**Privacy:** no accounts, no analytics. The only request it makes by itself is a once-a-day look at the project's
release list on GitHub, which can be turned off. [Details](https://github.com/bkjonathan/ein-cast/blob/main/docs/PRIVACY.md).

**Honest status:** young. Builds are not code-signed yet (your OS will warn). Only some devices have been tried:
[the list](https://github.com/bkjonathan/ein-cast/blob/main/docs/TESTING-DEVICES.md), and I would love your reports.

Download: https://github.com/bkjonathan/ein-cast/releases/latest · Source: https://github.com/bkjonathan/ein-cast ·
Problems: https://github.com/bkjonathan/ein-cast/issues

Happy to help anyone get it running.

---

## English: friends and family message

> I made an app that lets you watch the videos on my computer on your phone or the TV, no accounts. If you have ten
> minutes I would love to watch you try it and see where it confuses you. That helps more than anything. 🙂

---

## မြန်မာ (Facebook / Telegram), draft

**EinCast: Mac / Windows ကွန်ပျူတာရှိ ဖိုလ်ဒါကို တီဗီနှင့် ဖုန်းတွင် ကြည့်နိုင်သော အခမဲ့အက်ပ် (အကောင့်မလို)**

ကျွန်တော် သင့်အိမ်ရှိ ကွန်ပျူတာထဲက ဇာတ်ကား၊ သီချင်း၊ ဓာတ်ပုံတွေကို တီဗီ၊ iPad၊ ဖုန်းတွေမှာ လွယ်လွယ်ကူကူ ကြည့်နိုင်ရန် EinCast ကို ဖန်တီးထားပါတယ်။

- ဖိုလ်ဒါတစ်ခုရွေး → ဖုန်းနဲ့ QR ကုဒ်ကို စကင်ဖတ် → ကြည့်ပါ။ တီဗီမှာ app ထည့်စရာ မလိုပါ။
- အကောင့်မလို၊ cloud မလို၊ သင့်အကြောင်း ဘာမှ မပို့ပါ။
- ဗမာ/အင်္ဂလိပ် နှစ်ဘာသာ။ MIT လိုင်စင်၊ အခမဲ့။

**မဟုတ်တာ:** Jellyfin / Plex အစားထိုး မဟုတ်ပါ။ ကွန်ပျူတာမှာ app ဖွင့်ထားမှသာ အလုပ်လုပ်ပါတယ်။

**အခြေအနေ:** အသစ်ဖြစ်ပါသေးတယ်။ လက်မှတ်မထိုးရသေးလို့ ပထမအကြိမ် ဖွင့်ရင် စနစ်က သတိပေးပါမယ်။ စမ်းကြည့်ပြီး ဘယ်လိုရလဲ ပြောပြပေးပါ။ ဖွင့်လို့မရရင် ကျွန်တော် ကူညီပေးပါမယ်။

ဒေါင်းလုဒ်: https://github.com/bkjonathan/ein-cast/releases/latest
ပြဿနာ/အကြံပြုချက်: https://github.com/bkjonathan/ein-cast/issues

---

## After posting

- Answer every comment within a day for a week. Say "I do not know yet" when that is true.
- File each report as an issue, label it, thank the person, and link back.
- Collect what people ask for and update the public roadmap ([PUBLIC-ROADMAP.md](PUBLIC-ROADMAP.md)).
- Do not argue about Jellyfin. If it is the right tool for someone, say so.
