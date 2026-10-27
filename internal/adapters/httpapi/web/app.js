// EinCast device player. Plain browser JavaScript, no build step.
//
// File names come from the user's disk and may contain anything, so every piece
// of text is inserted with textContent / setAttribute, never as HTML. A test in
// the Go package fails if an HTML-injecting API appears in this file.
'use strict';

const $ = (id) => document.getElementById(id);

// ---- Words -------------------------------------------------------------------

// The page's text in the device's language, from /api/strings. The server picks
// the language (and fills any gap with English), so there is nothing to register
// here when a language is added. The text in index.html is English until this
// arrives; a key that is missing shows as itself, which is easy to spot.
let strings = {};
let lang = 'en';

function fill(text, params) {
  if (!params) return text;
  return text.replace(/\{(\w+)\}/g, (whole, name) => (name in params ? String(params[name]) : whole));
}

function t(key, params) {
  return key in strings ? fill(strings[key], params) : key;
}

// The right form of a plural for the language: English has "one" and "other",
// Burmese only "other".
function tn(base, count, params) {
  const form = new Intl.PluralRules(lang).select(count);
  const key = base + '.' + form;
  return t(key in strings ? key : base + '.other', Object.assign({ count: count }, params));
}

// Fills the text that is written in index.html.
function translatePage() {
  document.documentElement.lang = lang;
  for (const el of document.querySelectorAll('[data-i18n]')) el.textContent = t(el.dataset.i18n);
  for (const el of document.querySelectorAll('[data-i18n-placeholder]')) el.placeholder = t(el.dataset.i18nPlaceholder);
  for (const el of document.querySelectorAll('[data-i18n-label]')) el.setAttribute('aria-label', t(el.dataset.i18nLabel));
}

// ?lang=my asks for a language by hand, for a TV whose browser language cannot
// be changed. Without it the server goes by the browser's own list.
async function loadStrings() {
  const asked = new URLSearchParams(location.search).get('lang');
  try {
    const found = await getJSON('/api/strings' + (asked ? '?lang=' + encodeURIComponent(asked) : ''));
    strings = found.messages;
    lang = found.lang;
  } catch (err) {
    // The server is not answering; the English in index.html stays, and the
    // requests that follow report the problem.
  }
  translatePage();
}

let items = [];
let current = null;
// What the page shows below the player: the grid of posters, or the detail page
// of one title or one series. Kept in the browser's history, so the back button
// of a phone or a TV remote returns from a detail page instead of leaving.
let view = { type: 'grid' };
// The subtitle picked on a detail page, as a position in subtitleChoices(item)
// (sidecar files first, then those stored inside the video); -1 is off. It
// belongs to one item: playing anything else starts with none.
let subtitleChoice = -1;
let subtitleItem = '';
// The audio track picked on a detail page, as a position in the video's audio
// tracks; -1 is "as the file has it". It belongs to one item, like the subtitle.
let audioChoice = -1;
let audioItem = '';
// What the library is made of, rebuilt when it loads.
let catalog = { movies: [], shows: [], music: [], photos: [] };
// Set from /api/health: whether the server can make thumbnails. When it cannot,
// the page does not ask for dozens of pictures that would all be refused.
let thumbnails = false;
// Set from /api/health: whether the server can convert videos to HLS.
let transcoding = false;
// Set from /api/health: whether the server can list the tracks inside a video.
let tracksListed = false;
// True while the current video is played through the converted (HLS) stream.
let converted = false;
// The hls.js player of the current video, for browsers that cannot play HLS by
// themselves (Chrome, Firefox, Edge), and whether its playlist is still growing.
let hlsPlayer = null;
let hlsGrowing = false;
// Counts videos started, so a late answer (the script, the track list) for a
// video that was replaced since is ignored.
let playToken = 0;
// Where to continue after a source was replaced (a new audio track), in seconds.
let pendingSeek = 0;
// What is inside each video, once asked: item id -> {audio, subtitles}.
const trackData = new Map();
const trackAsked = new Map();
// The subtitle choices that are <track> elements of the current video, in the
// order the elements are in: the browser's list of text tracks has that order.
let shownChoices = [];
const NO_TRACKS = { audio: [], subtitles: [] };

// An HTTP error with the status and the server's error code, so the page can tell
// "choose a profile first" from "the server is down".
function httpError(url, status, code) {
  const err = new Error(url + ' returned ' + status);
  err.status = status;
  err.code = code || '';
  return err;
}

async function errorCode(res) {
  try {
    const body = await res.json();
    return body && body.error ? body.error.code : '';
  } catch (err) {
    return '';
  }
}

async function getJSON(url) {
  // ngrok's free plan puts a warning page in front of browser requests unless
  // this header is present. It is harmless anywhere else, and sending it always
  // means the page works behind any tunnel.
  const res = await fetch(url, {
    headers: { Accept: 'application/json', 'ngrok-skip-browser-warning': '1' },
  });
  if (!res.ok) throw httpError(url, res.status, await errorCode(res));
  return res.json();
}

// Sends a small JSON body (or none) and returns the parsed answer, or null for 204.
async function sendJSON(method, url, body) {
  const res = await fetch(url, {
    method: method,
    headers: { 'Content-Type': 'application/json', 'ngrok-skip-browser-warning': '1' },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  if (!res.ok) throw httpError(url, res.status, await errorCode(res));
  return res.status === 204 ? null : res.json();
}

function formatSize(bytes) {
  const units = ['B', 'KB', 'MB', 'GB', 'TB'];
  let value = bytes;
  let unit = 0;
  while (value >= 1024 && unit < units.length - 1) {
    value /= 1024;
    unit++;
  }
  return (unit === 0 || value >= 10 ? Math.round(value) : value.toFixed(1)) + ' ' + units[unit];
}

// What to call an item: the title read from its name (or a .nfo file), else the
// file name. The file name itself stays visible on the detail page.
function titleOf(item) {
  return item.title || item.name;
}

function isEpisode(item) {
  return item.kind === 'video' && !!item.show && item.episode > 0;
}

function pad2(n) {
  return String(n).padStart(2, '0');
}

// "S02E05", "E05" when the season is unknown, or "".
function episodeCode(item) {
  if (!isEpisode(item)) return '';
  return (item.season > 0 ? 'S' + pad2(item.season) : '') + 'E' + pad2(item.episode);
}

function titleWithYear(item) {
  return item.kind === 'video' && item.year > 0 ? titleOf(item) + ' (' + item.year + ')' : titleOf(item);
}

// The line above the player: "Fargo · S02E05 · Mandala" or "Heat (1995)".
function nowPlayingLabel(item) {
  if (!isEpisode(item)) return titleWithYear(item);
  const code = episodeCode(item);
  return item.show + ' · ' + code + (titleOf(item) !== code ? ' · ' + titleOf(item) : '');
}

function streamURL(item) {
  return '/stream/' + encodeURIComponent(item.id);
}

function thumbURL(item) {
  return '/thumb/' + encodeURIComponent(item.id);
}

// Whether to ask for this item's picture. A poster found next to the file is
// served even when the server cannot make thumbnails (no ffmpeg).
function hasPicture(item) {
  return thumbnails || item.poster === true;
}

// A small picture for a list entry, or nothing. A file with no picture (a song
// without cover art) answers 404; the image then removes itself and the entry
// is just text. Lazy loading keeps a long library from asking for everything.
function thumbnail(item) {
  const img = document.createElement('img');
  img.className = 'thumb';
  img.alt = '';
  img.loading = 'lazy';
  img.decoding = 'async';
  img.addEventListener('error', () => img.remove());
  img.src = thumbURL(item);
  return img;
}

// Resume position. It is kept in this browser's own storage, so it is per
// device by construction: no account, no server state. Storage can be missing or
// refuse writes (private windows), and then playback simply starts at 0.
const RESUME_PREFIX = 'eincast.resume.';
const SAVE_EVERY_MS = 5000;
const RESUME_MIN = 10; // seconds: not worth resuming a few seconds in
const RESUME_END_MARGIN = 30; // seconds: near the end counts as finished

// Where each title stopped, as the server remembers it for the profile in use:
// id -> { t, d, m (the file's modification time in ms), s (when it was saved) }.
// Positions follow the person between devices because they live there; this
// browser's own storage stays as the fallback when the server cannot be reached.
let serverResume = new Map();
// True once /api/resume answered, so positions are also sent to the server.
let serverResumeOn = false;

// The file's modification time as the server stores it (milliseconds).
function modMs(item) {
  return Date.parse(item.modTime);
}

function loadLocalResume(item) {
  try {
    const saved = JSON.parse(localStorage.getItem(RESUME_PREFIX + item.id));
    // A file that changed since is a different film as far as a position goes.
    if (saved && saved.m === item.modTime && saved.t >= RESUME_MIN) return saved;
  } catch (err) {
    // No storage, or a damaged value: nothing to resume.
  }
  return null;
}

function loadServerResume(item) {
  const saved = serverResume.get(item.id);
  if (saved && saved.m === modMs(item) && saved.t >= RESUME_MIN) {
    return { t: saved.t, d: saved.d, m: item.modTime, s: saved.s };
  }
  return null;
}

// The newer of what the server and this browser remember.
function loadResume(item) {
  const local = loadLocalResume(item);
  const remote = loadServerResume(item);
  if (remote && (!local || (remote.s || 0) >= (local.s || 0))) return remote;
  return local;
}

// Tells the server where a title stopped, or forgets it (finished). Failing is
// fine: this browser has it, and the next save tries again.
function pushResume(item, time, duration) {
  if (!serverResumeOn) return;
  const url = '/api/resume/' + encodeURIComponent(item.id);
  if (time === null) {
    serverResume.delete(item.id);
    sendJSON('DELETE', url).catch(() => {});
    return;
  }
  const t = Math.floor(time);
  const d = Math.floor(duration);
  serverResume.set(item.id, { t: t, d: d, m: modMs(item), s: Date.now() });
  sendJSON('PUT', url, { t: t, d: d, m: modMs(item) }).catch(() => {});
}

// Before libraries existed an item's ID was the hash of its path alone, and
// browsers kept positions under it. The server sends that old ID as legacyId; a
// position found under one moves to the item's current ID, once.
function migrateLegacyResume(list) {
  const current = new Set(list.map((i) => i.id));
  const legacy = new Map();
  for (const item of list) {
    if (item.legacyId && item.legacyId !== item.id) legacy.set(item.legacyId, item);
  }
  try {
    const keys = [];
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i);
      if (key && key.startsWith(RESUME_PREFIX)) keys.push(key);
    }
    for (const key of keys) {
      const id = key.slice(RESUME_PREFIX.length);
      const item = current.has(id) ? null : legacy.get(id);
      if (!item) continue;
      const raw = localStorage.getItem(key);
      if (raw !== null && localStorage.getItem(RESUME_PREFIX + item.id) === null) {
        localStorage.setItem(RESUME_PREFIX + item.id, raw);
      }
      localStorage.removeItem(key);
    }
  } catch (err) {
    // No storage: nothing to carry over.
  }
}

// Positions this browser has and the server lacks (or has older) go up once, so
// what was watched before the server remembered anything is not lost.
function uploadLocalResume(list) {
  let sent = 0;
  for (const item of list) {
    if (sent >= 100) break;
    const local = loadLocalResume(item);
    if (!local) continue;
    const remote = serverResume.get(item.id);
    if (remote && remote.m === modMs(item) && (remote.s || 0) >= (local.s || 0)) continue;
    sent++;
    pushResume(item, local.t, local.d);
  }
}

function saveResume(item, time, duration) {
  // Without a known duration the player is empty or still loading (this also
  // happens to the "pause" that resetting a player fires), or it is a stream
  // still being converted and so of unknown length: not a position.
  if (!(Number.isFinite(duration) && duration > 0)) return;
  try {
    if (time < RESUME_MIN || time > duration - RESUME_END_MARGIN) {
      localStorage.removeItem(RESUME_PREFIX + item.id);
      pushResume(item, null);
      return;
    }
    localStorage.setItem(
      RESUME_PREFIX + item.id,
      JSON.stringify({ t: Math.floor(time), d: Math.floor(duration), m: item.modTime, s: Date.now() }),
    );
  } catch (err) {
    // Resume is a convenience; losing it is fine.
  }
  pushResume(item, time, duration);
}

function formatTime(seconds) {
  const s = Math.max(0, Math.floor(seconds));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const pad = (n) => String(n).padStart(2, '0');
  return (h > 0 ? h + ':' + pad(m) : String(m)) + ':' + pad(s % 60);
}

let lastSave = 0;

function activePlayer() {
  return current && current.kind === 'audio' ? $('audio') : $('video');
}

function onTimeUpdate() {
  if (!current || current.kind === 'image') return;
  const now = Date.now();
  if (now - lastSave < SAVE_EVERY_MS) return;
  lastSave = now;
  const player = activePlayer();
  saveResume(current, player.currentTime, lengthOf(player));
}

function onPauseOrEnd() {
  if (!current || current.kind === 'image') return;
  const player = activePlayer();
  // Finished (or nearly): forget the position, so it starts over next time.
  saveResume(current, player.ended ? 0 : player.currentTime, lengthOf(player));
  if (player.ended) render();
}

// How long the current title is, or NaN while that is not known. hls.js reports
// the length of the playlist so far while a video is still being converted,
// which is not a length: saving it would lose the viewer's place.
function lengthOf(player) {
  return hlsPlayer !== null && hlsGrowing ? NaN : player.duration;
}

function onLoadedMetadata() {
  if (!current || current.kind === 'image') return;
  // hls.js hands the video element a new source of its own after the subtitle
  // tracks were added, and the cues they had loaded are gone with it. Adding the
  // tracks again, now that the source is in place, loads them again.
  if (hlsPlayer !== null && current.kind === 'video') replaceSubtitleTracks($('video'), current);
  settleSubtitles();
  const player = activePlayer();
  if (pendingSeek > 0) {
    // The source was replaced (another audio track): carry on from the same place.
    seekWhenPossible(player, pendingSeek);
    pendingSeek = 0;
    return;
  }
  const saved = loadResume(current);
  if (!saved) return;
  const length = hlsPlayer !== null && hlsGrowing ? Infinity : player.duration;
  if (saved.t < length - RESUME_END_MARGIN) {
    if (converted) seekWhenPossible(player, saved.t);
    else player.currentTime = saved.t;
    $('resume-text').textContent = t('player.resumed', { time: formatTime(saved.t) });
    $('resume').hidden = false;
  }
}

// Seeks to t, waiting if the converted video has not got that far yet: a video
// being converted can only be played up to where the conversion has reached.
function seekWhenPossible(player, t) {
  const token = playToken;
  const deadline = Date.now() + 90000;
  const attempt = () => {
    if (token !== playToken) return true; // another video since
    const ranges = player.seekable;
    if (ranges.length > 0 && ranges.end(ranges.length - 1) >= t) {
      player.currentTime = t;
      return true;
    }
    return false;
  };
  if (attempt()) return;
  const timer = setInterval(() => {
    if (attempt() || Date.now() > deadline) clearInterval(timer);
  }, 1000);
}

function startOver() {
  if (!current) return;
  activePlayer().currentTime = 0;
  try {
    localStorage.removeItem(RESUME_PREFIX + current.id);
  } catch (err) {
    // Nothing was stored, or nothing can be.
  }
  pushResume(current, null);
  $('resume').hidden = true;
}

// Containers browsers play as they are. Anything else (MKV, AVI, …) is offered
// as a converted stream when the device can play one.
const PLAYS_DIRECTLY = ['mp4', 'm4v', 'mov', 'webm'];

// Where the converted stream of a video is. Another audio track has its own
// folder, so its segments are not mixed up with the first track's.
function hlsURL(item) {
  const base = '/hls/' + encodeURIComponent(item.id) + '/';
  return needsAudioSwitch(item) ? base + 'audio/' + audioChoice + '/index.m3u8' : base + 'index.m3u8';
}

// Safari, iOS, and many TVs play HLS by themselves.
function nativeHLS() {
  return $('video').canPlayType('application/vnd.apple.mpegurl') !== '';
}

// Chrome, Firefox and Edge do not, but they can with hls.js, which needs the
// browser's Media Source Extensions. Whether it really works is only known once
// the script has loaded; this is the cheap question asked before.
function hlsJsPossible() {
  return typeof window.MediaSource === 'function' || typeof window.ManagedMediaSource === 'function';
}

function canPlayHLS() {
  return nativeHLS() || hlsJsPossible();
}

function wantsConversion(item) {
  if (!transcoding || item.kind !== 'video' || !canPlayHLS()) return false;
  // A browser cannot pick the audio track of a file it plays as it is.
  if (needsAudioSwitch(item)) return true;
  const dot = item.name.lastIndexOf('.');
  const ext = dot < 0 ? '' : item.name.slice(dot + 1).toLowerCase();
  return !PLAYS_DIRECTLY.includes(ext);
}

function showConverting() {
  const hint = $('hint');
  hint.textContent = t('player.converting');
  hint.hidden = false;
}

function resetPlayers() {
  playToken++;
  pendingSeek = 0;
  shownChoices = [];
  destroyHls();
  const video = $('video');
  const audio = $('audio');
  for (const media of [video, audio]) {
    media.pause();
    for (const track of media.querySelectorAll('track')) track.remove();
    media.removeAttribute('src');
    media.load();
    media.hidden = true;
  }
  $('resume').hidden = true;
  $('image').removeAttribute('src');
  $('image').hidden = true;
  $('hint').hidden = true;
}

// Records where the current title is, before something else replaces it.
function flushResume() {
  if (!current || current.kind === 'image') return;
  const player = activePlayer();
  saveResume(current, player.currentTime, lengthOf(player));
}

function play(item) {
  flushResume();
  current = item;
  resetPlayers();

  $('stage').hidden = false;
  $('now-playing').textContent = nowPlayingLabel(item);

  const link = $('direct-link');
  link.href = streamURL(item);
  link.hidden = false;

  if (item.kind === 'image') {
    const img = $('image');
    img.alt = item.name;
    img.src = streamURL(item);
    img.hidden = false;
  } else {
    const player = $(item.kind === 'video' ? 'video' : 'audio');
    if (item.kind === 'video') {
      addSubtitleTracks(player, item);
      // What is inside the video arrives a moment later; its subtitles join then.
      const token = playToken;
      loadTracks(item).then(() => {
        if (token === playToken) addSubtitleTracks(player, item);
      });
    }
    converted = item.kind === 'video' && wantsConversion(item);
    if (converted) showConverting();
    startSource(player, converted ? hlsURL(item) : streamURL(item), converted, true);
    player.hidden = false;
  }

  $('stage').scrollIntoView({ behavior: 'smooth', block: 'start' });
  render();
}

// The player is given its source: the file as it is, or the converted stream.
// Playback starts once the source is in place; with hls.js that is a moment after
// this returns, and asking the player to play before then would be cancelled when
// hls.js gives it the stream.
function startSource(player, url, isConverted, autoplay) {
  destroyHls();
  const begin = () => {
    if (autoplay) {
      player.play().catch(() => {
        // Autoplay can be refused; the controls are visible, so just wait.
      });
    }
  };
  if (!isConverted || nativeHLS()) {
    player.src = url;
    begin();
    return;
  }
  const token = playToken;
  loadHlsJs()
    .then((Hls) => {
      if (token !== playToken) return;
      if (!Hls.isSupported()) throw new Error('This browser cannot play converted video.');
      attachHlsJs(Hls, player, url, token, begin);
    })
    .catch(() => {
      if (token === playToken) giveUp();
    });
}

let hlsScript = null;

// hls.js is large and most pages never need it, so it is fetched the first time
// a converted video is played in a browser that has no HLS of its own.
function loadHlsJs() {
  if (typeof window.Hls === 'function') return Promise.resolve(window.Hls);
  if (hlsScript === null) {
    hlsScript = new Promise((resolve, reject) => {
      const script = document.createElement('script');
      script.src = '/static/hls.min.js';
      script.onload = () => (typeof window.Hls === 'function' ? resolve(window.Hls) : reject(new Error('hls.js did not start')));
      script.onerror = () => {
        hlsScript = null; // a later try may succeed
        reject(new Error('hls.js could not be loaded'));
      };
      document.head.append(script);
    });
  }
  return hlsScript;
}

function attachHlsJs(Hls, player, url, token, begin) {
  const instance = new Hls({
    // No worker: the page's policy allows scripts and media from itself only.
    enableWorker: false,
    // The playlist grows while a video is converted, which hls.js takes for a live
    // stream and would start at its end. This is a film: start at the beginning.
    startPosition: 0,
    enableCEA708Captions: false,
    // Same reason as getJSON: without it ngrok's free plan answers with a warning page.
    xhrSetup: (xhr) => xhr.setRequestHeader('ngrok-skip-browser-warning', '1'),
  });
  hlsPlayer = instance;
  hlsGrowing = true;
  let recoveredMedia = false;
  instance.on(Hls.Events.MANIFEST_PARSED, () => {
    if (token === playToken) begin();
  });
  instance.on(Hls.Events.LEVEL_LOADED, (event, data) => {
    if (instance === hlsPlayer) hlsGrowing = data.details.live === true;
  });
  instance.on(Hls.Events.ERROR, (event, data) => {
    if (!data.fatal || token !== playToken) return;
    if (data.type === Hls.ErrorTypes.MEDIA_ERROR && !recoveredMedia) {
      recoveredMedia = true;
      instance.recoverMediaError();
      return;
    }
    giveUp();
  });
  instance.loadSource(url);
  instance.attachMedia(player);
}

function destroyHls() {
  if (hlsPlayer === null) return;
  const instance = hlsPlayer;
  hlsPlayer = null;
  hlsGrowing = false;
  instance.destroy();
}

// ---- What is inside a video: audio tracks and subtitles ----------------------

// Asks once per video what it holds. A video that cannot be looked into simply
// has no choices.
function loadTracks(item) {
  if (item.kind !== 'video' || !tracksListed) return Promise.resolve(NO_TRACKS);
  if (!trackAsked.has(item.id)) {
    trackAsked.set(
      item.id,
      getJSON('/api/library/' + encodeURIComponent(item.id) + '/tracks')
        .then((found) => {
          trackData.set(item.id, found);
          return found;
        })
        .catch(() => {
          trackData.set(item.id, NO_TRACKS);
          return NO_TRACKS;
        }),
    );
  }
  return trackAsked.get(item.id);
}

function tracksOf(item) {
  return trackData.get(item.id) || NO_TRACKS;
}

// The audio track the file plays by itself: the one it marks, else the first.
function defaultAudio(item) {
  const found = tracksOf(item).audio.find((a) => a.default);
  return found ? found.index : 0;
}

// Whether the viewer picked a track other than the one the file plays by itself.
function needsAudioSwitch(item) {
  return audioItem === item.id && audioChoice >= 0 && audioChoice !== defaultAudio(item) &&
    tracksOf(item).audio.some((a) => a.index === audioChoice);
}

// Every subtitle the viewer can pick: the files next to the video, then the tracks
// inside it. Tracks made of pictures are listed but cannot be shown.
function subtitleChoices(item) {
  const id = encodeURIComponent(item.id);
  const choices = (item.subtitles || []).map((sub, index) => ({
    label: sub.label, lang: sub.lang, src: '/subtitle/' + id + '/' + index, supported: true,
  }));
  for (const sub of tracksOf(item).subtitles) {
    choices.push({
      label: sub.label, lang: sub.lang, supported: sub.supported,
      src: '/subtitle/' + id + '/embedded/' + sub.index,
    });
  }
  return choices;
}

// One <track> per subtitle that can be shown. They stay off until the viewer
// picks one, on the detail page or in the player's own menu, so nothing is shown
// that was not asked for. Called again when the tracks inside the video are
// known: only the ones not added yet are added.
function addSubtitleTracks(player, item) {
  subtitleChoices(item).forEach((sub, index) => {
    if (!sub.supported || shownChoices.includes(index)) return;
    const track = document.createElement('track');
    track.kind = 'subtitles';
    track.label = sub.label;
    if (sub.lang) track.srclang = sub.lang;
    // The track picked on the detail page starts showing; the others wait for the
    // player's own menu.
    if (item.id === subtitleItem && index === subtitleChoice) track.default = true;
    track.src = sub.src;
    player.append(track);
    shownChoices.push(index);
  });
  // Only when the viewer chose one: otherwise the player's own menu is theirs.
  if (subtitleItem === item.id && subtitleChoice >= 0) applySubtitleChoice(item);
}

function onPlaybackError() {
  if (!current) return;

  // The file as it is would not play (an MP4 with a codec this device lacks, say):
  // try the converted stream once before giving up.
  if (!converted && wantsConversionAfterError(current)) {
    converted = true;
    const player = $('video');
    showConverting();
    startSource(player, hlsURL(current), true, true);
    return;
  }
  giveUp();
}

function giveUp() {
  if (!current) return;
  const hint = $('hint');
  hint.textContent = converted ? t('player.cantConvert') : t('player.cantPlay');
  hint.hidden = false;
  if (converted) explainConversion(current);
}

// A failed conversion is only "an error" to the video element. Asking the server
// again says why, in words that help: a full disk or a busy computer is not the
// same problem as a file that cannot be read.
function explainConversion(item) {
  const token = playToken;
  fetch(hlsURL(item), { headers: { 'ngrok-skip-browser-warning': '1' } })
    .then((res) => {
      if (res.ok) {
        if (res.body) res.body.cancel(); // it is only being asked about
        return;
      }
      if (token !== playToken) return;
      const hint = $('hint');
      if (res.status === 507) {
        hint.textContent = t('player.diskFull');
      } else if (res.status === 503) {
        hint.textContent = t('player.busy');
      } else {
        return;
      }
      hint.hidden = false;
    })
    .catch(() => {
      // The generic message stays.
    });
}

function wantsConversionAfterError(item) {
  return transcoding && item.kind === 'video' && canPlayHLS();
}

// Playback began, so any "converting" notice is out of date.
function onPlaying() {
  if (converted) $('hint').hidden = true;
}

// "12:34 in" (how far the viewer got) for a title stopped part-way, or "".
function resumeLabel(item) {
  if (item.kind === 'image') return '';
  const saved = loadResume(item);
  return saved ? t('player.timeIn', { time: formatTime(saved.t) }) : '';
}

// ---- The library: a grid of posters, and a page for each title --------------

function compareText(a, b) {
  return a.localeCompare(b, undefined, { numeric: true, sensitivity: 'base' });
}

// Every word of the search must appear in the title, the file name, the series
// or the year, in any order. A plain substring match: no guessing.
function matches(item, needle) {
  if (needle === '') return true;
  const haystack = (titleOf(item) + ' ' + item.name + ' ' + (item.show || '') + ' ' + (item.year > 0 ? item.year : ''))
    .normalize('NFC')
    .toLowerCase();
  return needle.normalize('NFC').split(/\s+/).every((word) => haystack.includes(word));
}

// Sorts the library into what the grid shows: films and other videos, series
// (one card each), music and photos.
function buildCatalog(list) {
  const movies = [];
  const music = [];
  const photos = [];
  const bySeries = new Map();
  for (const item of list) {
    if (item.kind === 'audio') music.push(item);
    else if (item.kind === 'image') photos.push(item);
    else if (isEpisode(item)) {
      const key = seriesKey(item.show);
      if (!bySeries.has(key)) bySeries.set(key, { key: key, name: item.show, episodes: [] });
      bySeries.get(key).episodes.push(item);
    } else movies.push(item);
  }
  const shows = Array.from(bySeries.values());
  for (const show of shows) {
    show.episodes.sort((a, b) => a.season - b.season || a.episode - b.episode || compareText(a.name, b.name));
    show.cover = show.episodes[0];
    show.seasons = new Set(show.episodes.map((e) => e.season)).size;
  }
  movies.sort((a, b) => compareText(titleOf(a), titleOf(b)) || compareText(a.name, b.name));
  shows.sort((a, b) => compareText(a.name, b.name));
  music.sort((a, b) => compareText(titleOf(a), titleOf(b)));
  photos.sort((a, b) => compareText(a.name, b.name));
  return { movies: movies, shows: shows, music: music, photos: photos };
}

function showMatches(show, needle) {
  return needle === '' || matches({ name: '', title: show.name, show: show.name, year: 0 }, needle) ||
    show.episodes.some((e) => matches(e, needle));
}

// A hue from the title, so a poster that cannot be shown still looks like itself.
function hueOf(text) {
  let h = 0;
  for (let i = 0; i < text.length; i++) h = (h * 31 + text.charCodeAt(i)) % 360;
  return h;
}

// The picture of a card: a coloured square with the title's first letter, and the
// real picture on top when there is one. If the picture is refused (no poster,
// no ffmpeg) it removes itself and the colour stays. CSSOM styling is used
// because the page's policy refuses inline style attributes.
function cover(item, title) {
  const box = document.createElement('div');
  box.className = 'cover';
  const ph = document.createElement('div');
  ph.className = 'ph';
  ph.style.background = 'hsl(' + hueOf(title) + ', 32%, 30%)';
  ph.textContent = Array.from(title)[0] || '?';
  box.append(ph);
  if (item && hasPicture(item)) box.append(thumbnail(item));
  return box;
}

function card(item, title, sub, onOpen, progress) {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'card';
  btn.append(cover(item, title));
  if (progress > 0) {
    const bar = document.createElement('div');
    bar.className = 'progress';
    const fill = document.createElement('div');
    fill.style.width = Math.min(100, Math.round(progress * 100)) + '%';
    bar.append(fill);
    btn.firstChild.append(bar);
  }
  const t = document.createElement('span');
  t.className = 'card-title';
  t.textContent = title;
  btn.append(t);
  if (sub) {
    const s = document.createElement('span');
    s.className = 'card-sub';
    s.textContent = sub;
    btn.append(s);
  }
  btn.addEventListener('click', onOpen);
  return btn;
}

function seriesKey(name) {
  return name.normalize('NFC').trim().toLowerCase();
}

// Where an item is opened: an episode on its series' page, anything else on its own.
function detailOf(item) {
  return isEpisode(item) ? { type: 'show', key: seriesKey(item.show) } : { type: 'item', id: item.id };
}

function movieCard(item) {
  const saved = loadResume(item);
  return card(item, titleOf(item), item.year > 0 ? String(item.year) : '', () => go({ type: 'item', id: item.id }),
    saved && saved.d > 0 ? saved.t / saved.d : 0);
}

// "3 seasons · 24 episodes"; the seasons are left out for a series that has one.
function showSummary(show) {
  const episodes = tn('show.episodes', show.episodes.length);
  return show.seasons > 1 ? tn('show.seasons', show.seasons) + ' · ' + episodes : episodes;
}

function showCard(show) {
  return card(show.cover, show.name, showSummary(show), () => go({ type: 'show', key: show.key }), 0);
}

function section(title, count, content) {
  const heading = document.createElement('h2');
  heading.textContent = title + ' (' + count + ')';
  return [heading, content];
}

function cardGrid(className, cards) {
  const div = document.createElement('div');
  div.className = className;
  for (const c of cards) div.append(c);
  return div;
}

// Titles the viewer stopped part-way, most recently watched first.
function continueWatching() {
  const started = [];
  for (const item of items) {
    if (item.kind === 'image') continue;
    const saved = loadResume(item);
    if (saved) started.push({ item: item, saved: saved });
  }
  started.sort((a, b) => (b.saved.s || 0) - (a.saved.s || 0));
  return started.slice(0, 12);
}

// A music or photo entry: a row, as before. Posters would not suit a song.
function rowItem(item) {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'item';
  if (current && current.id === item.id) btn.setAttribute('aria-current', 'true');
  const name = document.createElement('span');
  name.className = 'name';
  name.textContent = item.kind === 'audio' ? titleOf(item) : item.name;
  const size = document.createElement('span');
  size.className = 'size';
  size.textContent = formatSize(item.size);
  if (hasPicture(item)) btn.append(thumbnail(item));
  btn.append(name);
  const left = resumeLabel(item);
  if (left) {
    const tag = document.createElement('span');
    tag.className = 'resume-tag';
    tag.textContent = left;
    btn.append(tag);
  }
  btn.append(size);
  btn.addEventListener('click', () => play(item));
  const li = document.createElement('li');
  li.append(btn);
  return li;
}

function renderGrid(root, needle) {
  let shown = 0;

  if (needle === '') {
    const resumable = continueWatching();
    if (resumable.length > 0) {
      const cards = resumable.map((r) => {
        const it = r.item;
        const title = isEpisode(it) ? it.show + ' ' + episodeCode(it) : titleOf(it);
        return card(it, title, t('player.timeIn', { time: formatTime(r.saved.t) }), () => go(detailOf(it)),
          r.saved.d > 0 ? r.saved.t / r.saved.d : 0);
      });
      root.append(...section(t('library.continue'), resumable.length, cardGrid('shelf', cards)));
    }
  }

  const movies = catalog.movies.filter((i) => matches(i, needle));
  if (movies.length > 0) {
    shown += movies.length;
    root.append(...section(t('library.videos'), movies.length, cardGrid('cards', movies.map(movieCard))));
  }
  const shows = catalog.shows.filter((s) => showMatches(s, needle));
  if (shows.length > 0) {
    shown += shows.length;
    root.append(...section(t('library.shows'), shows.length, cardGrid('cards', shows.map(showCard))));
  }
  for (const group of [{ list: catalog.music, title: t('library.music') }, { list: catalog.photos, title: t('library.photos') }]) {
    const list = group.list.filter((i) => matches(i, needle));
    if (list.length === 0) continue;
    shown += list.length;
    const ul = document.createElement('ul');
    for (const item of list) ul.append(rowItem(item));
    root.append(...section(group.title, list.length, ul));
  }
  return shown;
}

// The poster's height is a percentage of its parent's width, so on a detail page
// it needs a parent of the width it should have.
function artBox(coverEl) {
  const box = document.createElement('div');
  box.className = 'art';
  box.append(coverEl);
  return box;
}

function backButton() {
  const back = document.createElement('button');
  back.type = 'button';
  back.className = 'linkbutton back';
  back.textContent = t('page.back');
  back.addEventListener('click', () => history.back());
  return back;
}

// The detail page of one title: picture, name, year, description, and how to play it.
function renderItem(root, item) {
  root.append(backButton());
  const page = document.createElement('div');
  page.className = 'detail';
  page.append(artBox(cover(item, titleOf(item))));

  const info = document.createElement('div');
  info.className = 'info';
  const h = document.createElement('h2');
  h.textContent = titleWithYear(item);
  info.append(h);
  if (item.plot) {
    const plot = document.createElement('p');
    plot.className = 'plot';
    plot.textContent = item.plot; // from a .nfo file: text, never markup
    info.append(plot);
  }

  const saved = loadResume(item);
  const play1 = document.createElement('button');
  play1.type = 'button';
  play1.className = 'button primary';
  play1.textContent = saved ? t('item.resume', { time: formatTime(saved.t) }) : t('item.play');
  play1.addEventListener('click', () => play(item));
  info.append(play1);
  if (saved) {
    const again = document.createElement('button');
    again.type = 'button';
    again.className = 'button';
    again.textContent = t('page.startOver');
    again.addEventListener('click', () => {
      try {
        localStorage.removeItem(RESUME_PREFIX + item.id);
      } catch (err) {
        // Nothing was stored, or nothing can be.
      }
      pushResume(item, null);
      play(item);
    });
    info.append(again);
  }

  if (item.kind === 'video') trackPickers(info, item);

  const file = document.createElement('p');
  file.className = 'hint file';
  file.textContent = item.name + ' · ' + formatSize(item.size);
  info.append(file);
  page.append(info);
  root.append(page);
}

// The audio and subtitle menus of a video's page. They are drawn at once from
// what is already known and again when what is inside the video arrives, and
// stay out of sight while there is nothing to choose.
function trackPickers(info, item) {
  const audio = audioPicker(item);
  const subtitles = subtitlePicker(item);
  info.append(audio.label, subtitles.label);
  loadTracks(item).then(() => {
    audio.refresh();
    subtitles.refresh();
  });
}

function pickerOf(text) {
  const label = document.createElement('label');
  label.className = 'picker';
  label.hidden = true;
  label.append(text + ' ');
  const select = document.createElement('select');
  label.append(select);
  return { label: label, select: select };
}

function addOption(select, value, text, disabled) {
  const option = document.createElement('option');
  option.value = String(value);
  option.textContent = text;
  option.disabled = disabled === true;
  select.append(option);
}

function audioPicker(item) {
  const { label, select } = pickerOf(t('item.audio'));
  const refresh = () => {
    const audio = tracksOf(item).audio;
    select.replaceChildren();
    audio.forEach((track) => addOption(select, track.index, track.label, false));
    label.hidden = audio.length < 2;
    select.value = String(audioItem === item.id && audioChoice >= 0 ? audioChoice : defaultAudio(item));
  };
  select.addEventListener('change', () => {
    audioChoice = Number(select.value);
    audioItem = item.id;
    applyAudioChoice(item);
  });
  refresh();
  return { label: label, refresh: refresh };
}

function subtitlePicker(item) {
  const { label, select } = pickerOf(t('item.subtitles'));
  const refresh = () => {
    const choices = subtitleChoices(item);
    select.replaceChildren();
    addOption(select, -1, t('item.subtitlesOff'), false);
    choices.forEach((sub, index) => {
      const text = sub.supported ? sub.label : t('item.subtitlePictures', { label: sub.label });
      addOption(select, index, text, !sub.supported);
    });
    label.hidden = choices.length === 0;
    select.value = String(subtitleItem === item.id ? subtitleChoice : -1);
  };
  select.addEventListener('change', () => {
    subtitleChoice = Number(select.value);
    subtitleItem = item.id;
    applySubtitleChoice(item);
  });
  refresh();
  return { label: label, refresh: refresh };
}

// Switches the subtitle of what is playing now, if it is this item. The browser
// lists the text tracks in the order of the <track> elements, which shownChoices
// follows; any track hls.js or the browser adds itself is turned off.
function applySubtitleChoice(item) {
  if (!current || current.id !== item.id || current.kind !== 'video') return;
  const wanted = subtitleItem === item.id ? subtitleChoice : -1;
  const list = $('video').textTracks;
  for (let i = 0; i < list.length; i++) {
    list[i].mode = i < shownChoices.length && shownChoices[i] === wanted ? 'showing' : 'disabled';
  }
}

function replaceSubtitleTracks(player, item) {
  for (const track of player.querySelectorAll('track')) track.remove();
  shownChoices = [];
  addSubtitleTracks(player, item);
}

// Chrome turns on every subtitle track when a source is set, whatever the track's
// attributes say. The choice is applied again as the source starts, so what is
// shown is what the viewer picked, and nothing when they picked nothing.
function settleSubtitles() {
  if (current && current.kind === 'video') applySubtitleChoice(current);
}

// Switches the audio of what is playing now, if it is this item: a new source,
// carrying on from the same place. The browser cannot change the audio track of
// a file it plays as it is, so the server converts the video with the track chosen.
function applyAudioChoice(item) {
  if (!current || current.id !== item.id || current.kind !== 'video') return;
  const player = $('video');
  const at = player.currentTime;
  const wasPlaying = !player.paused;
  playToken++;
  pendingSeek = at > 1 ? at : 0;
  shownChoices = [];
  for (const track of player.querySelectorAll('track')) track.remove();
  converted = wantsConversion(item);
  if (converted) showConverting();
  startSource(player, converted ? hlsURL(item) : streamURL(item), converted, wasPlaying);
  addSubtitleTracks(player, item);
}

// The detail page of a series: its seasons, each with its episodes.
function renderShow(root, show) {
  root.append(backButton());
  const page = document.createElement('div');
  page.className = 'detail';
  page.append(artBox(cover(show.cover, show.name)));

  const info = document.createElement('div');
  info.className = 'info';
  const h = document.createElement('h2');
  h.textContent = show.name;
  info.append(h);
  const count = document.createElement('p');
  count.className = 'hint';
  count.textContent = showSummary(show);
  info.append(count);
  page.append(info);
  root.append(page);

  let season = -1;
  let ul = null;
  for (const ep of show.episodes) {
    if (ep.season !== season || ul === null) {
      season = ep.season;
      const heading = document.createElement('h3');
      heading.textContent = season > 0 ? t('show.season', { number: season }) : t('show.noSeason');
      ul = document.createElement('ul');
      root.append(heading, ul);
    }
    ul.append(episodeRow(ep));
  }
}

function episodeRow(ep) {
  const btn = document.createElement('button');
  btn.type = 'button';
  btn.className = 'item';
  if (current && current.id === ep.id) btn.setAttribute('aria-current', 'true');
  const num = document.createElement('span');
  num.className = 'num';
  num.textContent = String(ep.episode);
  const name = document.createElement('span');
  name.className = 'name';
  name.textContent = titleOf(ep) === episodeCode(ep) ? ep.name : titleOf(ep);
  btn.append(num, name);
  const left = resumeLabel(ep);
  if (left) {
    const tag = document.createElement('span');
    tag.className = 'resume-tag';
    tag.textContent = left;
    btn.append(tag);
  }
  btn.addEventListener('click', () => play(ep));
  const li = document.createElement('li');
  li.append(btn);
  return li;
}

function render() {
  const needle = $('search').value.normalize('NFC').trim().toLowerCase();
  const root = $('groups');
  root.replaceChildren();
  $('search').hidden = view.type !== 'grid';

  const status = $('status');
  if (view.type === 'item') {
    const item = items.find((i) => i.id === view.id);
    if (item) {
      renderItem(root, item);
      status.textContent = '';
      return;
    }
  } else if (view.type === 'show') {
    const show = catalog.shows.find((s) => s.key === view.key);
    if (show) {
      renderShow(root, show);
      status.textContent = '';
      return;
    }
  }
  view = { type: 'grid' }; // the title is gone (the library changed): back to the grid
  $('search').hidden = false;

  const shown = renderGrid(root, needle);
  if (items.length === 0) status.textContent = t('library.empty');
  else if (shown === 0) status.textContent = t('library.noMatch');
  else status.textContent = '';
}

// Moves to a detail page, or back to the grid, and records it in the history.
function go(next) {
  history.pushState(next, '');
  view = next;
  render();
  document.querySelector('main').scrollIntoView();
}

window.addEventListener('popstate', (event) => {
  view = event.state && event.state.type ? event.state : { type: 'grid' };
  render();
});

// ---- Who is watching ---------------------------------------------------------

// The avatar colours, by the names the server uses. The server accepts only these.
const PROFILE_COLORS = {
  blue: '#3b82f6', green: '#22c55e', red: '#ef4444', orange: '#f97316',
  purple: '#a855f7', pink: '#ec4899', teal: '#14b8a6', gray: '#6b7280',
};

// What /api/profiles last said: whether there is a choice to make (more than one
// profile), who this browser chose, and the people to pick from.
let profiles = { active: false, current: '', profiles: [] };
// The profile whose PIN is being asked for.
let pinFor = null;

function avatar(profile) {
  const el = document.createElement('span');
  el.className = 'avatar';
  el.style.background = PROFILE_COLORS[profile.color] || PROFILE_COLORS.gray;
  el.textContent = (Array.from(profile.name)[0] || '?').toUpperCase();
  return el;
}

function currentProfile() {
  return profiles.profiles.find((p) => p.id === profiles.current) || null;
}

// Shows the library view and the button that goes back to the picker.
function showLibrary() {
  $('picker').hidden = true;
  $('library-view').hidden = false;
  $('playlist-link').hidden = false;
  const button = $('profile-button');
  const who = profiles.active ? currentProfile() : null;
  button.hidden = who === null;
  if (who) {
    button.replaceChildren(avatar(who), document.createTextNode(who.name));
    button.setAttribute('aria-label', t('profile.switch', { name: who.name }));
  }
}

// Shows "Who's watching?" in place of the library.
function showPicker() {
  $('library-view').hidden = true;
  $('profile-button').hidden = true;
  $('playlist-link').hidden = true; // a playlist is per profile: there is none to offer yet
  $('picker').hidden = false;
  $('pin-form').hidden = true;
  pinFor = null;

  const list = $('picker-list');
  list.replaceChildren();
  for (const p of profiles.profiles) {
    const btn = document.createElement('button');
    btn.type = 'button';
    btn.className = 'card';
    const name = document.createElement('span');
    name.className = 'card-title';
    name.textContent = p.name;
    btn.append(avatar(p), name);
    if (p.hasPin) {
      const lock = document.createElement('span');
      lock.className = 'card-sub';
      lock.textContent = t('profile.pin');
      btn.append(lock);
    }
    btn.addEventListener('click', () => (p.hasPin ? askPin(p) : choose(p, '')));
    list.append(btn);
  }
}

function askPin(profile) {
  pinFor = profile;
  $('pin-for').textContent = t('profile.pinFor', { name: profile.name });
  $('pin-error').textContent = '';
  $('pin-input').value = '';
  $('pin-form').hidden = false;
  $('pin-input').focus();
}

// Picks a profile. The server checks the PIN and answers with the cookie that
// makes this browser that profile; the page only shows what the server then lets
// through, so nothing here is a lock.
async function choose(profile, pin) {
  try {
    await sendJSON('POST', '/api/profile', { id: profile.id, pin: pin });
  } catch (err) {
    if (!profile.hasPin) {
      $('status').textContent = t('profile.chooseFailed');
      return;
    }
    $('pin-error').textContent =
      err.status === 429 ? t('profile.tooManyPins')
        : err.status === 401 ? t('profile.wrongPin')
          : t('profile.signInFailed');
    $('pin-input').value = '';
    return;
  }
  profiles.current = profile.id;
  await openLibrary();
}

function submitPin() {
  if (pinFor) choose(pinFor, $('pin-input').value.trim());
}

// Forgets the choice and shows the picker, e.g. for someone else to take over.
async function leaveProfile() {
  flushResume();
  resetPlayers();
  current = null;
  $('stage').hidden = true;
  try {
    await sendJSON('DELETE', '/api/profile');
    profiles = await getJSON('/api/profiles');
  } catch (err) {
    $('status').textContent = t('profile.switchFailed');
    return;
  }
  profiles.current = '';
  items = [];
  catalog = { movies: [], shows: [], music: [], photos: [] };
  serverResume = new Map();
  serverResumeOn = false;
  // What a track list said for one profile (or that a video was not found) is not
  // true for the next.
  trackData.clear();
  trackAsked.clear();
  view = { type: 'grid' };
  showPicker();
}

// Loads what this browser's profile may see, and where it stopped in each title.
async function openLibrary() {
  try {
    const library = await getJSON('/api/library');
    try {
      const saved = await getJSON('/api/resume');
      serverResume = new Map(saved.map((e) => [e.id, e]));
      serverResumeOn = true;
    } catch (err) {
      // No server-side resume (or it is unreachable): this browser's own storage serves.
      serverResume = new Map();
      serverResumeOn = false;
    }
    items = library.slice();
    catalog = buildCatalog(items);
    migrateLegacyResume(items);
    uploadLocalResume(items);
    view = { type: 'grid' };
    showLibrary();
    render();
  } catch (err) {
    if (err.status === 401 && err.code === 'profile_required') {
      // The choice was never made, or the profile's PIN changed since.
      try {
        profiles = await getJSON('/api/profiles');
      } catch (err2) {
        // Keep what we had.
      }
      showPicker();
      return;
    }
    $('status').textContent = t('library.loadFailed');
  }
}

async function init() {
  $('video').addEventListener('error', onPlaybackError);
  $('video').addEventListener('loadstart', settleSubtitles);
  $('audio').addEventListener('error', onPlaybackError);
  $('search').addEventListener('input', render);
  for (const media of [$('video'), $('audio')]) {
    media.addEventListener('loadedmetadata', onLoadedMetadata);
    media.addEventListener('playing', onPlaying);
    media.addEventListener('timeupdate', onTimeUpdate);
    media.addEventListener('pause', onPauseOrEnd);
    media.addEventListener('ended', onPauseOrEnd);
  }
  $('restart').addEventListener('click', startOver);
  $('profile-button').addEventListener('click', leaveProfile);
  $('pin-cancel').addEventListener('click', () => {
    $('pin-form').hidden = true;
    pinFor = null;
  });
  $('pin-form').addEventListener('submit', (event) => {
    event.preventDefault(); // the page never submits a form: its policy forbids it
    submitPin();
  });

  await loadStrings();
  try {
    const [health, who] = await Promise.all([getJSON('/api/health'), getJSON('/api/profiles')]);
    if (health.name) {
      $('server-name').textContent = health.name;
      document.title = health.name + ' · EinCast';
    }
    thumbnails = health.thumbnails === true;
    transcoding = health.transcoding === true;
    tracksListed = health.tracks === true;
    profiles = who;
    if (who.active && !who.current) {
      showPicker();
      return;
    }
    await openLibrary();
  } catch (err) {
    $('status').textContent = t('library.loadFailed');
  }
}

init();
