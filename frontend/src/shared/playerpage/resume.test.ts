/// <reference types="node" />
// Tests for the resume logic in the device player page. That page is plain
// browser JavaScript embedded in the Go binary (no build step, no module
// system), so these tests load the real file into a Node `vm` with a minimal
// fake DOM and exercise its functions. They belong beside the page's rules, but
// the page has no JS toolchain of its own; this is where Vitest lives.
import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import vm from "node:vm";
import { beforeEach, describe, expect, it } from "vitest";

const source = readFileSync(
  resolve(__dirname, "../../../../internal/adapters/httpapi/web/app.js"),
  "utf8",
);

interface Item {
  id: string;
  name: string;
  kind: "video" | "audio" | "image";
  size: number;
  modTime: string;
  subtitles?: { label: string; lang: string }[];
  // Read from the name or a .nfo file by the server.
  title?: string;
  year?: number;
  show?: string;
  season?: number;
  episode?: number;
  plot?: string;
  poster?: boolean;
}

interface Series {
  key: string;
  name: string;
  episodes: Item[];
  cover: Item;
  seasons: number;
}
interface Catalog {
  movies: Item[];
  shows: Series[];
  music: Item[];
  photos: Item[];
}

interface FakeElement {
  hidden: boolean;
  textContent: string;
  currentTime: number;
  duration: number;
  ended: boolean;
  children: FakeElement[];
  tagName?: string;
  [key: string]: unknown;
}

interface Page {
  loadResume: (item: Item) => { t: number; d: number; m: string; s?: number } | null;
  titleOf: (item: Item) => string;
  isEpisode: (item: Item) => boolean;
  episodeCode: (item: Item) => string;
  titleWithYear: (item: Item) => string;
  nowPlayingLabel: (item: Item) => string;
  matches: (item: Item, needle: string) => boolean;
  buildCatalog: (items: Item[]) => Catalog;
  hasPicture: (item: Item) => boolean;
  setThumbnails: (on: boolean) => void;
  setSubtitleChoice: (item: string, index: number) => void;
  detailOf: (item: Item) => { type: string; id?: string; key?: string };
  saveResume: (item: Item, time: number, duration: number) => void;
  formatTime: (s: number) => string;
  resumeLabel: (item: Item) => string;
  flushResume: () => void;
  onPauseOrEnd: () => void;
  onLoadedMetadata: () => void;
  startOver: () => void;
  addSubtitleTracks: (player: FakeElement, item: Item) => void;
  wantsConversion: (item: Item) => boolean;
  hlsURL: (item: Item) => string;
  onPlaybackError: () => void;
  onPlaying: () => void;
  setCurrent: (item: Item | null) => void;
  setTranscoding: (on: boolean) => void;
  setConverted: (on: boolean) => void;
  isConverted: () => boolean;
  el: (id: string) => FakeElement;
  store: Map<string, string>;
}

function loadPage(storageThrows = false): Page {
  const store = new Map<string, string>();
  const elements = new Map<string, FakeElement>();
  const el = (id: string): FakeElement => {
    let e = elements.get(id);
    if (!e) {
      e = {
        hidden: false,
        textContent: "",
        currentTime: 0,
        duration: NaN,
        ended: false,
        children: [],
        addEventListener: () => undefined,
        append(this: FakeElement, ...nodes: FakeElement[]) {
          this.children.push(...nodes);
        },
        querySelectorAll: () => [],
        pause: () => undefined,
        load: () => undefined,
        play: () => Promise.resolve(),
        canPlayType: () => "",
        src: "",
        removeAttribute: () => undefined,
        scrollIntoView: () => undefined,
        replaceChildren: () => undefined,
        value: "",
      };
      elements.set(id, e);
    }
    return e;
  };

  const localStorage = {
    getItem: (k: string) => {
      if (storageThrows) throw new Error("blocked");
      return store.get(k) ?? null;
    },
    setItem: (k: string, v: string) => {
      if (storageThrows) throw new Error("blocked");
      store.set(k, v);
    },
    removeItem: (k: string) => {
      if (storageThrows) throw new Error("blocked");
      store.delete(k);
    },
  };

  const context = vm.createContext({
    document: {
      getElementById: el,
      createElement: (tagName: string): FakeElement => ({
        ...el(`__new_${Math.random()}`),
        children: [],
        tagName,
      }),
      title: "",
      querySelector: () => el("__main"),
    },
    // The page listens for the back button and records navigation; neither does
    // anything in these tests.
    window: { addEventListener: () => undefined, scrollTo: () => undefined },
    history: { pushState: () => undefined, back: () => undefined },
    localStorage,
    fetch: () => Promise.reject(new Error("no network in tests")),
    Date,
    JSON,
    Math,
    String,
    encodeURIComponent,
    Promise,
    console,
  });
  vm.runInContext(source, context);
  // Top-level `let`/`const` of the page are visible to later scripts in the same context.
  vm.runInContext(
    `this.__page = { loadResume, saveResume, formatTime, resumeLabel, flushResume,
       titleOf, isEpisode, episodeCode, titleWithYear, nowPlayingLabel, matches, buildCatalog,
       hasPicture, detailOf,
       setThumbnails: (on) => { thumbnails = on; },
       setSubtitleChoice: (item, index) => { subtitleItem = item; subtitleChoice = index; },
       onPauseOrEnd, onLoadedMetadata, startOver, addSubtitleTracks,
       wantsConversion, hlsURL, onPlaybackError, onPlaying,
       setCurrent: (i) => { current = i; },
       setTranscoding: (on) => { transcoding = on; },
       setConverted: (on) => { converted = on; },
       isConverted: () => converted };`,
    context,
  );
  const page = (context as unknown as { __page: Omit<Page, "el" | "store"> }).__page;
  return { ...page, el, store };
}

const video = (id: string, modTime = "2024-01-01T00:00:00Z"): Item => ({
  id,
  name: `${id}.mkv`,
  kind: "video",
  size: 1,
  modTime,
});

let page: Page;
beforeEach(() => {
  page = loadPage();
});

describe("resume storage", () => {
  it("remembers a position and gives it back", () => {
    page.saveResume(video("a"), 754.9, 3600);
    // `s` is when it was saved, which orders "Continue watching".
    expect(page.loadResume(video("a"))).toEqual({ t: 754, d: 3600, m: "2024-01-01T00:00:00Z", s: expect.any(Number) });
  });

  it("does not remember the first seconds or the last half minute", () => {
    page.saveResume(video("a"), 5, 3600);
    expect(page.loadResume(video("a"))).toBeNull();
    page.saveResume(video("a"), 3590, 3600);
    expect(page.loadResume(video("a"))).toBeNull();
  });

  it("forgets an earlier position when the title is finished", () => {
    page.saveResume(video("a"), 1000, 3600);
    page.saveResume(video("a"), 3590, 3600);
    expect(page.store.size).toBe(0);
  });

  it("ignores a position saved for an older version of the file", () => {
    page.saveResume(video("a", "old"), 1000, 3600);
    expect(page.loadResume(video("a", "new"))).toBeNull();
  });

  it("a stream of unknown length (still being converted) is not a position either", () => {
    page.saveResume(video("a"), 1000, 3600);
    page.saveResume(video("a"), 1200, Infinity);
    expect(page.loadResume(video("a"))?.t).toBe(1000);
  });

  it("an unknown duration is not a position and erases nothing", () => {
    page.saveResume(video("a"), 1000, 3600);
    page.saveResume(video("a"), 0, NaN);
    expect(page.loadResume(video("a"))?.t).toBe(1000);
  });

  it("copes with damaged values and with storage that throws", () => {
    page.store.set("eincast.resume.a", "{not json");
    expect(page.loadResume(video("a"))).toBeNull();

    const blocked = loadPage(true);
    expect(() => blocked.saveResume(video("a"), 1000, 3600)).not.toThrow();
    expect(blocked.loadResume(video("a"))).toBeNull();
  });
});

describe("formatTime and resumeLabel", () => {
  it.each([
    [0, "0:00"],
    [65, "1:05"],
    [3599, "59:59"],
    [3600, "1:00:00"],
    [3725, "1:02:05"],
    [-4, "0:00"],
  ])("formats %d seconds as %s", (seconds, text) => {
    expect(page.formatTime(seconds)).toBe(text);
  });

  it("labels a half-watched title, and nothing else", () => {
    page.saveResume(video("a"), 754, 3600);
    expect(page.resumeLabel(video("a"))).toBe("12:34 in");
    expect(page.resumeLabel(video("b"))).toBe("");
    expect(page.resumeLabel({ ...video("a"), kind: "image" })).toBe("");
  });
});

describe("while playing", () => {
  it("switching titles saves the one being left, at its own position", () => {
    const a = video("a");
    page.setCurrent(a);
    const player = page.el("video");
    player.currentTime = 900;
    player.duration = 3600;

    page.flushResume();
    expect(page.loadResume(a)?.t).toBe(900);
  });

  it("the pause fired by resetting a player does not erase the next title's position", () => {
    const b = video("b");
    page.saveResume(b, 1200, 3600); // saved in an earlier session
    page.setCurrent(b);
    const player = page.el("video"); // just reset: nothing loaded yet
    player.currentTime = 0;
    player.duration = NaN;

    page.onPauseOrEnd();
    expect(page.loadResume(b)?.t).toBe(1200);
  });

  it("finishing a title clears its position", () => {
    const a = video("a");
    page.saveResume(a, 1000, 3600);
    page.setCurrent(a);
    const player = page.el("video");
    player.currentTime = 3600;
    player.duration = 3600;
    player.ended = true;

    page.onPauseOrEnd();
    expect(page.loadResume(a)).toBeNull();
  });

  it("opening a title jumps to the saved position and says so", () => {
    const a = video("a");
    page.saveResume(a, 754, 3600);
    page.setCurrent(a);
    const player = page.el("video");
    player.duration = 3600;

    page.onLoadedMetadata();
    expect(player.currentTime).toBe(754);
    expect(page.el("resume").hidden).toBe(false);
    expect(page.el("resume-text").textContent).toBe("Resumed from 12:34.");
  });

  it("does not jump when the saved position is near the end of what loaded", () => {
    const a = video("a");
    page.saveResume(a, 1000, 3600);
    page.setCurrent(a);
    const player = page.el("video");
    player.duration = 1010; // a shorter cut of the same file name

    page.onLoadedMetadata();
    expect(player.currentTime).toBe(0);
  });

  it("start over rewinds and forgets", () => {
    const a = video("a");
    page.saveResume(a, 754, 3600);
    page.setCurrent(a);
    const player = page.el("video");
    player.currentTime = 754;

    page.startOver();
    expect(player.currentTime).toBe(0);
    expect(page.loadResume(a)).toBeNull();
    expect(page.el("resume").hidden).toBe(true);
  });
});

describe("subtitle tracks", () => {
  it("adds one track per subtitle, addressed by position, off by default", () => {
    const player = page.el("video");
    page.addSubtitleTracks(player, {
      ...video("a b"),
      subtitles: [
        { label: "English", lang: "en" },
        { label: "Subtitles", lang: "" },
      ],
    });
    expect(player.children).toHaveLength(2);
    expect(player.children[0]).toMatchObject({
      kind: "subtitles",
      label: "English",
      srclang: "en",
      src: "/subtitle/a%20b/0",
    });
    expect(player.children[1]).toMatchObject({ label: "Subtitles", src: "/subtitle/a%20b/1" });
    expect(player.children[1]).not.toHaveProperty("srclang");
    expect(player.children[0]).not.toHaveProperty("default");
  });

  it("copes with an item that has no list", () => {
    const player = page.el("video");
    page.addSubtitleTracks(player, video("a"));
    expect(player.children).toHaveLength(0);
  });
});

describe("converted (HLS) playback", () => {
  const withHLS = (supported: boolean, serverCan = true) => {
    page.el("video").canPlayType = () => (supported ? "maybe" : "");
    page.setTranscoding(serverCan);
  };
  const named = (name: string, kind: Item["kind"] = "video"): Item => ({ ...video("a"), name, kind });

  it("converts what browsers cannot play directly, when the device plays HLS", () => {
    withHLS(true);
    for (const name of ["film.mkv", "old.AVI", "clip.wmv", "x.ts", "noextension"]) {
      expect(page.wantsConversion(named(name)), name).toBe(true);
    }
    for (const name of ["film.mp4", "clip.M4V", "movie.mov", "web.webm"]) {
      expect(page.wantsConversion(named(name)), name).toBe(false);
    }
  });

  it("never converts without server support, device support, or for audio and photos", () => {
    withHLS(true, false);
    expect(page.wantsConversion(named("film.mkv"))).toBe(false);
    withHLS(false);
    expect(page.wantsConversion(named("film.mkv"))).toBe(false);
    withHLS(true);
    expect(page.wantsConversion(named("song.flac", "audio"))).toBe(false);
    expect(page.wantsConversion(named("pic.png", "image"))).toBe(false);
  });

  it("addresses the stream by id only", () => {
    expect(page.hlsURL({ ...video("a b") })).toBe("/hls/a%20b/index.m3u8");
  });

  it("a file that fails to play directly is retried once as a converted stream", () => {
    withHLS(true);
    const item = named("film.mp4"); // plays directly in principle
    page.setCurrent(item);
    page.setConverted(false);

    page.onPlaybackError();
    expect(page.isConverted()).toBe(true);
    expect(page.el("video").src).toBe("/hls/a/index.m3u8");
    expect(page.el("hint").textContent).toMatch(/Converting/);

    // ... and if the converted stream fails too, the page gives up with advice.
    page.onPlaybackError();
    expect(page.el("hint").textContent).toMatch(/couldn't be converted.*VLC/);
  });

  it("without conversion available a failure just suggests another player", () => {
    withHLS(true, false);
    page.setCurrent(named("film.mkv"));
    page.setConverted(false);

    page.onPlaybackError();
    expect(page.isConverted()).toBe(false);
    expect(page.el("hint").textContent).toMatch(/can't play this file.*VLC/);
  });

  it("the converting notice goes away once playback starts", () => {
    page.setCurrent(named("film.mkv"));
    page.setConverted(true);
    page.el("hint").hidden = false;
    page.onPlaying();
    expect(page.el("hint").hidden).toBe(true);
  });

  it("playing something that is not converted leaves other notices alone", () => {
    page.setCurrent(named("film.mp4"));
    page.setConverted(false);
    page.el("hint").hidden = false;
    page.onPlaying();
    expect(page.el("hint").hidden).toBe(false);
  });
});

const film = (name: string, over: Partial<Item> = {}): Item => ({
  id: name, name, kind: "video", size: 1, modTime: "m", title: name, year: 0, show: "", season: 0, episode: 0, plot: "", poster: false, ...over,
});
const ep = (show: string, season: number, episode: number, over: Partial<Item> = {}): Item =>
  film(`${show}.S${season}E${episode}.mkv`, { show, season, episode, ...over });

describe("titles", () => {
  it("shows the title read from the name, else the file name", () => {
    expect(page.titleOf(film("Heat.1995.mkv", { title: "Heat" }))).toBe("Heat");
    expect(page.titleOf({ ...film("a.mkv"), title: "" })).toBe("a.mkv");
    const old = film("old.mkv");
    delete old.title; // a server that does not send titles yet
    expect(page.titleOf(old)).toBe("old.mkv");
  });

  it("adds the year to a video only", () => {
    expect(page.titleWithYear(film("x", { title: "Heat", year: 1995 }))).toBe("Heat (1995)");
    expect(page.titleWithYear(film("x", { title: "Heat" }))).toBe("Heat");
    expect(page.titleWithYear(film("x", { kind: "audio", title: "Song", year: 2001 }))).toBe("Song");
  });

  it("codes episodes, and only episodes", () => {
    expect(page.episodeCode(ep("Fargo", 2, 5))).toBe("S02E05");
    expect(page.episodeCode(ep("Fargo", 0, 5))).toBe("E05");
    expect(page.episodeCode(film("Heat"))).toBe("");
    expect(page.isEpisode(film("x", { show: "Fargo" }))).toBe(false); // a series but no number
    expect(page.isEpisode(film("x", { episode: 3 }))).toBe(false); // a number but no series
  });

  it("labels what is playing", () => {
    expect(page.nowPlayingLabel(film("x", { title: "Heat", year: 1995 }))).toBe("Heat (1995)");
    expect(page.nowPlayingLabel(ep("Fargo", 2, 5, { title: "Mandala" }))).toBe("Fargo · S02E05 · Mandala");
    expect(page.nowPlayingLabel(ep("Fargo", 2, 5, { title: "S02E05" }))).toBe("Fargo · S02E05"); // no name of its own
  });
});

describe("search", () => {
  const heat = film("Heat.1995.1080p.mkv", { title: "Heat", year: 1995, plot: "robbers and police" });

  it("matches the title, the file name, the series and the year", () => {
    expect(page.matches(heat, "heat")).toBe(true);
    expect(page.matches(heat, "1080p")).toBe(true);
    expect(page.matches(heat, "1995")).toBe(true);
    expect(page.matches(ep("Fargo", 1, 1, { title: "Pilot" }), "fargo")).toBe(true);
  });

  it("needs every word, in any order, and an empty search matches everything", () => {
    expect(page.matches(heat, "1995 heat")).toBe(true);
    expect(page.matches(heat, "heat 1996")).toBe(false);
    expect(page.matches(heat, "")).toBe(true);
  });

  it("is a substring match over the plot-free fields, not a fuzzy one", () => {
    expect(page.matches(heat, "robbers")).toBe(false);
    expect(page.matches(heat, "hete")).toBe(false);
  });

  it("ignores Unicode form", () => {
    expect(page.matches(film("a", { title: "Caf\u00e9" }), "cafe\u0301")).toBe(true);
  });
});

describe("buildCatalog", () => {
  it("sorts the library into films, one entry per series, music and photos", () => {
    const c = page.buildCatalog([
      film("b.mkv", { title: "Zulu" }), film("a.mkv", { title: "Alpha" }),
      ep("Fargo", 2, 1), ep("Fargo", 1, 10), ep("Fargo", 1, 2), ep("Lost", 1, 1),
      film("s.mp3", { kind: "audio" }), film("p.jpg", { kind: "image" }),
    ]);
    expect(c.movies.map((m) => m.title)).toEqual(["Alpha", "Zulu"]);
    expect(c.shows.map((s) => s.name)).toEqual(["Fargo", "Lost"]);
    expect(c.shows[0]?.episodes.map((e) => `${e.season}x${e.episode}`)).toEqual(["1x2", "1x10", "2x1"]);
    expect(c.shows[0]).toMatchObject({ seasons: 2 });
    expect(c.music).toHaveLength(1);
    expect(c.photos).toHaveLength(1);
  });

  it("is one series across spellings that differ in case, and uses the first episode as its picture", () => {
    const c = page.buildCatalog([ep("Fargo", 1, 2), ep("FARGO ", 1, 1)]);
    expect(c.shows).toHaveLength(1);
    expect(c.shows[0]?.cover.episode).toBe(1);
  });

  it("opens an episode on its series' page and a film on its own", () => {
    expect(page.detailOf(ep("Fargo", 1, 1))).toEqual({ type: "show", key: "fargo" });
    expect(page.detailOf(film("heat"))).toEqual({ type: "item", id: "heat" });
  });
});

describe("pictures", () => {
  it("asks for a picture when the server can make them, or when a poster was found", () => {
    page.setThumbnails(false);
    expect(page.hasPicture(film("a"))).toBe(false);
    expect(page.hasPicture(film("a", { poster: true }))).toBe(true);
    page.setThumbnails(true);
    expect(page.hasPicture(film("a"))).toBe(true);
  });
});

describe("subtitle choice", () => {
  const withSubs = { ...film("a", { title: "A" }), subtitles: [{ label: "English", lang: "en" }, { label: "French", lang: "fr" }] };

  it("starts the picked track showing, on the item it was picked for", () => {
    page.setSubtitleChoice("a", 1);
    const player = page.el("video");
    page.addSubtitleTracks(player, withSubs);
    expect(player.children.map((t) => t.default)).toEqual([undefined, true]);
  });

  it("does not carry over to another item", () => {
    page.setSubtitleChoice("other", 1);
    const player = page.el("video");
    page.addSubtitleTracks(player, withSubs);
    expect(player.children.map((t) => t.default)).toEqual([undefined, undefined]);
  });
});
