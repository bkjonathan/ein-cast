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

async function getJSON(url) {
  const res = await fetch(url, { headers: { Accept: 'application/json' } });
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

function resetPlayers() {
  const video = $('video');
  const audio = $('audio');
  for (const media of [video, audio]) {
    media.pause();
    media.removeAttribute('src');
    media.load();
    media.hidden = true;
  }
  $('image').removeAttribute('src');
  $('image').hidden = true;
  $('hint').hidden = true;
}

function play(item) {
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
    player.src = streamURL(item);
    player.hidden = false;
    player.play().catch(() => {
      // Autoplay can be refused; the controls are visible, so just wait.
    });
  }

  $('stage').scrollIntoView({ behavior: 'smooth', block: 'start' });
  render();
}

function onPlaybackError() {
  if (!current) return;
  const hint = $('hint');
  hint.textContent =
    "This browser can't play this file (common for MKV or HEVC). " +
    'Use the playlist button to open the library in VLC or Infuse.';
  hint.hidden = false;
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

      btn.append(name, size);
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

  try {
    const [health, library] = await Promise.all([getJSON('/api/health'), getJSON('/api/library')]);
    if (health.name) {
      $('server-name').textContent = health.name;
      document.title = health.name + ' · EinCast';
    }
    items = library.slice().sort((a, b) => a.name.localeCompare(b.name, undefined, { numeric: true }));
    render();
  } catch (err) {
    $('status').textContent = "Couldn't load the library. Check that EinCast is still running.";
  }
}

init();
