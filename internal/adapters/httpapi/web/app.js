// EinCast device player. Plain browser JavaScript, no build step.
//
// File names come from the user's disk and may contain anything, so every piece
// of text is inserted with textContent / setAttribute, never as HTML. A test in
// the Go package fails if an HTML-injecting API appears in this file.
'use strict';

const $ = (id) => document.getElementById(id);

let items = [];
let current = null;
// What the page shows below the player: the grid of posters, or the detail page
// of one title or one series. Kept in the browser's history, so the back button
// of a phone or a TV remote returns from a detail page instead of leaving.
let view = { type: 'grid' };
// The subtitle picked on a detail page, as a position in item.subtitles; -1 is
// off. It belongs to one item: playing anything else starts with none.
let subtitleChoice = -1;
let subtitleItem = '';
// What the library is made of, rebuilt when it loads.
let catalog = { movies: [], shows: [], music: [], photos: [] };
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
      JSON.stringify({ t: Math.floor(time), d: Math.floor(duration), m: item.modTime, s: Date.now() }),
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
// one, on the detail page or in the player's own menu, so nothing is shown that
// was not asked for.
function addSubtitleTracks(player, item) {
  (item.subtitles || []).forEach((sub, index) => {
    const track = document.createElement('track');
    track.kind = 'subtitles';
    track.label = sub.label;
    if (sub.lang) track.srclang = sub.lang;
    // The track picked on the detail page starts showing; the others wait for the
    // player's own menu.
    if (item.id === subtitleItem && index === subtitleChoice) track.default = true;
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

function showCard(show) {
  const n = show.episodes.length;
  const sub = (show.seasons > 1 ? show.seasons + ' seasons · ' : '') + n + (n === 1 ? ' episode' : ' episodes');
  return card(show.cover, show.name, sub, () => go({ type: 'show', key: show.key }), 0);
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
        return card(it, title, formatTime(r.saved.t) + ' in', () => go(detailOf(it)),
          r.saved.d > 0 ? r.saved.t / r.saved.d : 0);
      });
      root.append(...section('Continue watching', resumable.length, cardGrid('shelf', cards)));
    }
  }

  const movies = catalog.movies.filter((i) => matches(i, needle));
  if (movies.length > 0) {
    shown += movies.length;
    root.append(...section('Videos', movies.length, cardGrid('cards', movies.map(movieCard))));
  }
  const shows = catalog.shows.filter((s) => showMatches(s, needle));
  if (shows.length > 0) {
    shown += shows.length;
    root.append(...section('TV shows', shows.length, cardGrid('cards', shows.map(showCard))));
  }
  for (const group of [{ list: catalog.music, title: 'Music' }, { list: catalog.photos, title: 'Photos' }]) {
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
  back.textContent = '← Back to the library';
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
  play1.textContent = saved ? 'Resume from ' + formatTime(saved.t) : 'Play';
  play1.addEventListener('click', () => play(item));
  info.append(play1);
  if (saved) {
    const again = document.createElement('button');
    again.type = 'button';
    again.className = 'button';
    again.textContent = 'Start over';
    again.addEventListener('click', () => {
      try {
        localStorage.removeItem(RESUME_PREFIX + item.id);
      } catch (err) {
        // Nothing was stored, or nothing can be.
      }
      play(item);
    });
    info.append(again);
  }

  if ((item.subtitles || []).length > 0) info.append(subtitlePicker(item));

  const file = document.createElement('p');
  file.className = 'hint file';
  file.textContent = item.name + ' · ' + formatSize(item.size);
  info.append(file);
  page.append(info);
  root.append(page);
}

function subtitlePicker(item) {
  const label = document.createElement('label');
  label.className = 'picker';
  label.append('Subtitles ');
  const select = document.createElement('select');
  const off = document.createElement('option');
  off.value = '-1';
  off.textContent = 'Off';
  select.append(off);
  item.subtitles.forEach((sub, index) => {
    const option = document.createElement('option');
    option.value = String(index);
    option.textContent = sub.label;
    select.append(option);
  });
  select.value = String(subtitleItem === item.id ? subtitleChoice : -1);
  select.addEventListener('change', () => {
    subtitleChoice = Number(select.value);
    subtitleItem = item.id;
    applySubtitleChoice(item);
  });
  label.append(select);
  return label;
}

// Switches the subtitle of what is playing now, if it is this item.
function applySubtitleChoice(item) {
  if (!current || current.id !== item.id || current.kind !== 'video') return;
  const tracks = $('video').textTracks;
  for (let i = 0; i < tracks.length; i++) tracks[i].mode = i === subtitleChoice ? 'showing' : 'disabled';
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
  const n = show.episodes.length;
  const count = document.createElement('p');
  count.className = 'hint';
  count.textContent = (show.seasons > 1 ? show.seasons + ' seasons · ' : '') + n + (n === 1 ? ' episode' : ' episodes');
  info.append(count);
  page.append(info);
  root.append(page);

  let season = -1;
  let ul = null;
  for (const ep of show.episodes) {
    if (ep.season !== season || ul === null) {
      season = ep.season;
      const heading = document.createElement('h3');
      heading.textContent = season > 0 ? 'Season ' + season : 'Episodes';
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
  if (items.length === 0) status.textContent = 'This library has no media yet.';
  else if (shown === 0) status.textContent = 'Nothing matches your search.';
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
    items = library.slice();
    catalog = buildCatalog(items);
    render();
  } catch (err) {
    $('status').textContent = "Couldn't load the library. Check that EinCast is still running.";
  }
}

init();
