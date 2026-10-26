// EinCast device player. Plain browser JavaScript, no build step.
//
// File names come from the user's disk and may contain anything, so every piece
// of text is inserted with textContent / setAttribute, never as HTML. A test in
// the Go package fails if an HTML-injecting API appears in this file.
'use strict';

const $ = (id) => document.getElementById(id);

const GROUPS = [
  { kind: 'video', title: 'Videos' },
  { kind: 'audio', title: 'Music' },
  { kind: 'image', title: 'Photos' },
];

let items = [];
let current = null;
// Set from /api/health: whether the server can make thumbnails. When it cannot,
// the page does not ask for dozens of pictures that would all be refused.
let thumbnails = false;
// Set from /api/health: whether the server can convert videos to HLS.
let transcoding = false;
// True while the current video is played through the converted (HLS) stream.
let converted = false;

async function getJSON(url) {
  // ngrok's free plan puts a warning page in front of browser requests unless
  // this header is present. It is harmless anywhere else, and sending it always
  // means the page works behind any tunnel.
  const res = await fetch(url, {
    headers: { Accept: 'application/json', 'ngrok-skip-browser-warning': '1' },
  });
  if (!res.ok) throw new Error(url + ' returned ' + res.status);
  return res.json();
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

function streamURL(item) {
  return '/stream/' + encodeURIComponent(item.id);
}

function thumbURL(item) {
  return '/thumb/' + encodeURIComponent(item.id);
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

function loadResume(item) {
  try {
    const saved = JSON.parse(localStorage.getItem(RESUME_PREFIX + item.id));
    // A file that changed since is a different film as far as a position goes.
    if (saved && saved.m === item.modTime && saved.t >= RESUME_MIN) return saved;
  } catch (err) {
    // No storage, or a damaged value: nothing to resume.
  }
  return null;
}

function saveResume(item, time, duration) {
  // Without a known duration the player is empty or still loading (this also
  // happens to the "pause" that resetting a player fires), or it is a stream
  // still being converted and so of unknown length: not a position.
  if (!(Number.isFinite(duration) && duration > 0)) return;
  try {
    if (time < RESUME_MIN || time > duration - RESUME_END_MARGIN) {
      localStorage.removeItem(RESUME_PREFIX + item.id);
      return;
    }
    localStorage.setItem(
      RESUME_PREFIX + item.id,
      JSON.stringify({ t: Math.floor(time), d: Math.floor(duration), m: item.modTime }),
    );
  } catch (err) {
    // Resume is a convenience; losing it is fine.
  }
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
  saveResume(current, player.currentTime, player.duration);
}

function onPauseOrEnd() {
  if (!current || current.kind === 'image') return;
  const player = activePlayer();
  // Finished (or nearly): forget the position, so it starts over next time.
  saveResume(current, player.ended ? 0 : player.currentTime, player.duration);
  if (player.ended) render();
}

function onLoadedMetadata() {
  if (!current || current.kind === 'image') return;
  const saved = loadResume(current);
  if (!saved) return;
  const player = activePlayer();
  if (saved.t < player.duration - RESUME_END_MARGIN) {
    player.currentTime = saved.t;
    $('resume-text').textContent = 'Resumed from ' + formatTime(saved.t) + '.';
    $('resume').hidden = false;
  }
}

function startOver() {
  if (!current) return;
  activePlayer().currentTime = 0;
  try {
    localStorage.removeItem(RESUME_PREFIX + current.id);
  } catch (err) {
    // Nothing was stored, or nothing can be.
  }
  $('resume').hidden = true;
}

// Containers browsers play as they are. Anything else (MKV, AVI, …) is offered
// as a converted stream when the device can play one.
const PLAYS_DIRECTLY = ['mp4', 'm4v', 'mov', 'webm'];

function hlsURL(item) {
  return '/hls/' + encodeURIComponent(item.id) + '/index.m3u8';
}

// Safari, iOS, and many TVs play HLS natively; Chrome and Firefox on a computer
// do not, and for them a converted stream would not play at all.
function canPlayHLS() {
  return $('video').canPlayType('application/vnd.apple.mpegurl') !== '';
}

function wantsConversion(item) {
  if (!transcoding || item.kind !== 'video' || !canPlayHLS()) return false;
  const dot = item.name.lastIndexOf('.');
  const ext = dot < 0 ? '' : item.name.slice(dot + 1).toLowerCase();
  return !PLAYS_DIRECTLY.includes(ext);
}

function showConverting() {
  const hint = $('hint');
  hint.textContent = 'Converting this video for your device. It can take a few seconds to start.';
  hint.hidden = false;
}

function resetPlayers() {
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
  saveResume(current, player.currentTime, player.duration);
}

function play(item) {
  flushResume();
  current = item;
  resetPlayers();

  $('stage').hidden = false;
  $('now-playing').textContent = item.name;

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
    if (item.kind === 'video') addSubtitleTracks(player, item);
    converted = wantsConversion(item);
    player.src = converted ? hlsURL(item) : streamURL(item);
    if (converted) showConverting();
    player.hidden = false;
    player.play().catch(() => {
      // Autoplay can be refused; the controls are visible, so just wait.
    });
  }

  $('stage').scrollIntoView({ behavior: 'smooth', block: 'start' });
  render();
}

// One <track> per sidecar subtitle file. They stay off until the viewer picks
// one in the player's own menu, so nothing is shown that was not asked for.
function addSubtitleTracks(player, item) {
  (item.subtitles || []).forEach((sub, index) => {
    const track = document.createElement('track');
    track.kind = 'subtitles';
    track.label = sub.label;
    if (sub.lang) track.srclang = sub.lang;
    track.src = '/subtitle/' + encodeURIComponent(item.id) + '/' + index;
    player.append(track);
  });
}

function onPlaybackError() {
  if (!current) return;
  const hint = $('hint');

  // The file as it is would not play (an MP4 with a codec this device lacks, say):
  // try the converted stream once before giving up.
  if (!converted && wantsConversionAfterError(current)) {
    converted = true;
    const player = $('video');
    player.src = hlsURL(current);
    player.play().catch(() => {});
    showConverting();
    return;
  }

  hint.textContent = converted
    ? "This video couldn't be converted for this device. " +
      'Use the playlist button to open the library in VLC or Infuse.'
    : "This browser can't play this file (common for MKV or HEVC). " +
      'Use the playlist button to open the library in VLC or Infuse.';
  hint.hidden = false;
}

function wantsConversionAfterError(item) {
  return transcoding && item.kind === 'video' && canPlayHLS();
}

// Playback began, so any "converting" notice is out of date.
function onPlaying() {
  if (converted) $('hint').hidden = true;
}

// "12:34 in" for a title the viewer stopped part-way, or "".
function resumeLabel(item) {
  if (item.kind === 'image') return '';
  const saved = loadResume(item);
  return saved ? formatTime(saved.t) + ' in' : '';
}

function matches(item, needle) {
  return needle === '' || item.name.toLowerCase().includes(needle);
}

function render() {
  const needle = $('search').value.trim().toLowerCase();
  const root = $('groups');
  root.replaceChildren();

  let shown = 0;
  for (const group of GROUPS) {
    const list = items.filter((i) => i.kind === group.kind && matches(i, needle));
    if (list.length === 0) continue;
    shown += list.length;

    const heading = document.createElement('h2');
    heading.textContent = group.title + ' (' + list.length + ')';
    const ul = document.createElement('ul');

    for (const item of list) {
      const li = document.createElement('li');
      const btn = document.createElement('button');
      btn.type = 'button';
      btn.className = 'item';
      if (current && current.id === item.id) btn.setAttribute('aria-current', 'true');

      const name = document.createElement('span');
      name.className = 'name';
      name.textContent = item.name;
      const size = document.createElement('span');
      size.className = 'size';
      size.textContent = formatSize(item.size);

      if (thumbnails) btn.append(thumbnail(item));
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
      li.append(btn);
      ul.append(li);
    }
    root.append(heading, ul);
  }

  const status = $('status');
  if (items.length === 0) status.textContent = 'This library has no media yet.';
  else if (shown === 0) status.textContent = 'Nothing matches your search.';
  else status.textContent = '';
}

async function init() {
  $('video').addEventListener('error', onPlaybackError);
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

  try {
    const [health, library] = await Promise.all([getJSON('/api/health'), getJSON('/api/library')]);
    if (health.name) {
      $('server-name').textContent = health.name;
      document.title = health.name + ' · EinCast';
    }
    thumbnails = health.thumbnails === true;
    transcoding = health.transcoding === true;
    items = library.slice().sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
    render();
  } catch (err) {
    $('status').textContent = "Couldn't load the library. Check that EinCast is still running.";
  }
}

init();
