// Zero-dependency static-site generator for the ensemble marketing site.
//
//   node build.mjs            → renders ./dist (a fully self-contained static site)
//
// Output is plain HTML/CSS/woff2/png with RELATIVE paths, so ./dist can be served
// from any "dumb" web server (or opened from file://) with nothing else required.
// Edit content.mjs for the words; edit src/assets/styles.css for the look.

import { content as C } from "./content.mjs";
import { promises as fs } from "node:fs";
import { createHash } from "node:crypto";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.dirname(fileURLToPath(import.meta.url));
const SRC = path.join(root, "src");
const OUT = path.join(root, "dist");
const VERSION = process.env.ENSEMBLE_VERSION || "";

const esc = (s = "") =>
  String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

const eq = (n = 7) =>
  `<span class="eq" aria-hidden="true">${Array.from({ length: n }, (_, i) => `<i style="--i:${i}"></i>`).join("")}</span>`;

// Nav links, with in-page anchors prefixed so they also work from a sub-page
// (download.html → index.html#why). prefix is "" on the home page.
// GitHub mark, sized to sit inline with the nav text (fills currentColor).
const GITHUB_ICON =
  '<svg class="gh-mark" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 012-.27c.68 0 1.36.09 2 .27 1.53-1.03 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>';

const renderNav = (prefix = "") =>
  C.nav
    .map((l) => {
      const href = l.href.startsWith("#") ? prefix + l.href : l.href;
      const rel = l.href.startsWith("#") ? "" : ' rel="noopener"';
      const icon = l.icon === "github" ? GITHUB_ICON : "";
      const cls = icon ? ' class="nav-gh"' : "";
      return `<a${cls} href="${esc(href)}"${rel}>${icon}${esc(l.label)}</a>`;
    })
    .join("");

const features = C.why.features
  .map(
    (f) => `
      <article class="feat">
        <div class="feat-top"><span class="feat-n">${esc(f.n)}</span><span class="tag">${esc(f.tag)}</span></div>
        <h3>${esc(f.title)}</h3>
        <p>${esc(f.body)}</p>
      </article>`
  )
  .join("");

const screens = C.screens.items
  .map(
    (s, i) => `
      <article class="screen${i % 2 ? " flip" : ""}">
        <figure class="screen-shot">
          <img class="lb-thumb" data-lb="${i}" role="button" tabindex="0" aria-label="Open “${esc(s.title)}” full size" src="${esc(s.src)}" alt="${esc(s.alt)}" loading="lazy" decoding="async" />
        </figure>
        <div class="screen-copy">
          <span class="kicker">${eq(5)}${esc(s.kicker)}</span>
          <h3>${esc(s.title)}</h3>
          <p>${esc(s.body)}</p>
        </div>
      </article>`
  )
  .join("");

// Theme carousel — one framed screenshot per built-in theme, in a scroll-snap row.
const themeSlides = C.themes.items
  .map(
    (t, i) => `
      <figure class="tc-slide" data-i="${i}">
        <div class="tc-frame"><img src="${esc(t.img)}" alt="ensemble in the ${esc(t.name)} theme" loading="lazy" decoding="async" /></div>
        <figcaption class="tc-cap"><span class="tc-name">${esc(t.name)}</span><span class="tc-blurb">${esc(t.blurb)}</span></figcaption>
      </figure>`
  )
  .join("");
const themeDots = C.themes.items
  .map(
    (t, i) =>
      `<button class="tc-dot" data-i="${i}" aria-label="Show the ${esc(t.name)} theme"${i === 0 ? ' aria-current="true"' : ""}></button>`
  )
  .join("");

// Lightbox carousel — the screenshot gallery followed by the measured-coherence
// graphs, in order. data-lb indices on the thumbs are global across both sets, so
// the proof graphs open at C.screens.items.length + their own index. The graphs are
// wide (not portrait phone shots), so they carry a `wide` flag for a larger cap.
const lbItems = [
  ...C.screens.items.map((s) => ({ src: s.src, alt: s.alt, cap: `${s.kicker} — ${s.title}` })),
  ...C.proof.items.map((s) => ({ src: s.src, alt: s.alt, cap: `${s.kicker} — ${s.title}`, wide: true })),
];

const lbSlides = lbItems
  .map(
    (s) => `
      <figure class="lb-slide${s.wide ? " wide" : ""}" data-cap="${esc(s.cap)}">
        <img src="${esc(s.src)}" alt="${esc(s.alt)}" loading="lazy" decoding="async" />
      </figure>`
  )
  .join("");

const lbDots = lbItems
  .map(
    (s, i) =>
      `<button class="lb-dot" type="button" data-i="${i}" aria-label="View “${esc(s.cap)}”"></button>`
  )
  .join("");

// The hero reuses one of the gallery shots; open the lightbox at its slide.
const heroLbIdx = Math.max(0, C.screens.items.findIndex((s) => s.src === C.hero.shot.src));

const steps = C.quickstart.steps
  .map(
    (s) => `
      <li class="step">
        <span class="step-n">${esc(s.n)}</span>
        <h3>${esc(s.title)}</h3>
        <p>${esc(s.body)}</p>
      </li>`
  )
  .join("");

const techItems = C.tech.items
  .map(
    (t) => `
      <article class="tech">
        <span class="tag">${esc(t.tag)}</span>
        <p class="tech-problem">${esc(t.problem)}</p>
        <p class="tech-solution"><span class="tech-arrow" aria-hidden="true">→</span>${esc(t.solution)}</p>
      </article>`
  )
  .join("");

// Measured-coherence proof: a branded graph (bare PNG from tools/calib/) with the
// headline + honest judgement set in the brand font. Alternates side like screens.
const proofLbBase = C.screens.items.length;
const proof = C.proof.items
  .map(
    (s, i) => `
      <article class="proof-item${i % 2 ? " flip" : ""}">
        <figure class="proof-shot">
          <img class="lb-thumb" data-lb="${proofLbBase + i}" role="button" tabindex="0" aria-label="Open “${esc(s.title)}” full size" src="${esc(s.src)}" alt="${esc(s.alt)}" loading="lazy" decoding="async" />
        </figure>
        <div class="proof-copy">
          <span class="kicker">${eq(5)}${esc(s.kicker)}</span>
          ${s.metric ? `<span class="proof-metric">${esc(s.metric)}</span>` : ""}
          <h3>${esc(s.title)}</h3>
          <p>${esc(s.body)}</p>
        </div>
      </article>`
  )
  .join("");

const testimonials = C.testimonials.items
  .map(
    (t) => `
      <figure class="quote">
        <img class="quote-photo" src="${esc(t.img)}" alt="${esc(t.name)}" loading="lazy" decoding="async" width="72" height="72" />
        <blockquote>“${esc(t.quote)}”</blockquote>
        <figcaption><span class="quote-name">${esc(t.name)}</span><span class="quote-role">${esc(t.role)}</span></figcaption>
        ${
          t.credit
            ? `<span class="quote-credit">image: <a href="${esc(t.credit.href)}" target="_blank" rel="noopener license" title="source on Wikimedia Commons">${esc([t.credit.author, t.credit.license].filter(Boolean).join(" · "))}</a></span>`
            : ""
        }
      </figure>`
  )
  .join("");

const footLinks = C.footer.links
  .map((l) => `<a href="${esc(l.href)}" rel="noopener">${esc(l.label)}</a>`)
  .join("");

const page = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>${esc(C.meta.title)}</title>
<meta name="description" content="${esc(C.meta.description)}" />
<meta name="theme-color" content="${esc(C.meta.themeColor)}" />
<meta property="og:title" content="${esc(C.meta.title)}" />
<meta property="og:description" content="${esc(C.meta.description)}" />
<meta property="og:type" content="website" />
<meta property="og:image" content="assets/img/overview.png" />
<link rel="preload" href="assets/fonts/fraunces-wght.woff2" as="font" type="font/woff2" crossorigin />
<link rel="preload" href="assets/fonts/plex-sans-400.woff2" as="font" type="font/woff2" crossorigin />
<link rel="stylesheet" href="assets/styles.css" />
</head>
<body>
<div class="grain" aria-hidden="true"></div>

<header class="nav">
  <a class="brand" href="#top">${esc(C.brand.name)}<span class="brand-dot"></span></a>
  <nav class="nav-links">${renderNav("")}</nav>
  <a class="btn btn-solid nav-cta" href="${esc(C.hero.primary.href)}" rel="noopener">${esc(C.hero.primary.label)}</a>
</header>

<main id="top">
  <section class="hero">
    <div class="hero-copy">
      <span class="eyebrow">${eq(6)}${esc(C.hero.eyebrow)}</span>
      <h1>${C.hero.title.map((t) => `<span>${esc(t)}</span>`).join("")}</h1>
      <p class="lede">${esc(C.hero.lede)}</p>
      <div class="actions">
        <a class="btn btn-solid" href="${esc(C.hero.primary.href)}" rel="noopener">${esc(C.hero.primary.label)}<span class="arrow">→</span></a>
        <a class="btn btn-ghost" href="${esc(C.hero.secondary.href)}" rel="noopener">${esc(C.hero.secondary.label)}</a>
      </div>
      <div class="snippet">
        <code><span class="prompt">$</span> ${esc(C.hero.snippet.cmd)}</code>
        <span class="snippet-cap">${esc(C.hero.snippet.caption)}</span>
      </div>
    </div>
    <figure class="hero-shot">
      <div class="frame"><img class="lb-thumb" data-lb="${heroLbIdx}" role="button" tabindex="0" aria-label="Open screenshots full size" src="${esc(C.hero.shot.src)}" alt="${esc(C.hero.shot.alt)}" /></div>
    </figure>
  </section>

  <section id="why" class="why">
    <header class="sec-head">
      <span class="eyebrow">${esc(C.why.eyebrow)}</span>
      <h2>${esc(C.why.title)}</h2>
    </header>
    <div class="feat-grid">${features}</div>
  </section>

  <section id="screens" class="screens">
    <header class="sec-head">
      <span class="eyebrow">${esc(C.screens.eyebrow)}</span>
      <h2>${esc(C.screens.title)}</h2>
    </header>
    <div class="screen-list">${screens}</div>
  </section>

  <section id="themes" class="themes">
    <header class="sec-head">
      <span class="eyebrow">${esc(C.themes.eyebrow)}</span>
      <h2>${esc(C.themes.title)}</h2>
      <p class="sec-intro">${esc(C.themes.intro)}</p>
    </header>
    <div class="tc">
      <button class="tc-arrow tc-prev" type="button" aria-label="Previous theme">‹</button>
      <div class="tc-track">${themeSlides}</div>
      <button class="tc-arrow tc-next" type="button" aria-label="Next theme">›</button>
    </div>
    <div class="tc-dots">${themeDots}</div>
  </section>

  <section id="quickstart" class="how">
    <header class="sec-head">
      <span class="eyebrow">${esc(C.quickstart.eyebrow)}</span>
      <h2>${esc(C.quickstart.title)}</h2>
    </header>
    <ol class="steps">${steps}</ol>
    <div class="qs-cta">
      <p class="qs-cta-text">${esc(C.quickstart.cta.text)}</p>
      <a class="btn btn-solid qs-cta-btn" href="${esc(C.quickstart.cta.href)}" rel="noopener">${esc(C.quickstart.cta.label)}<span class="arrow">→</span></a>
    </div>
  </section>

  <section id="tech" class="tech-sec">
    <header class="sec-head">
      <span class="eyebrow">${esc(C.tech.eyebrow)}</span>
      <h2>${esc(C.tech.title)}</h2>
      <p class="sec-intro">${esc(C.tech.intro)}</p>
    </header>
    <div class="tech-grid">${techItems}</div>
    <header class="sec-head tech-proof-head">
      <span class="eyebrow">${esc(C.proof.eyebrow)}</span>
      <h2>${esc(C.proof.title)}</h2>
      <p class="sec-intro">${esc(C.proof.intro)}</p>
    </header>
    <div class="proof-list">${proof}</div>
  </section>

  <section id="praise" class="praise">
    <header class="sec-head">
      <span class="eyebrow">${esc(C.testimonials.eyebrow)}</span>
      <h2>${esc(C.testimonials.title)}</h2>
      <p class="sec-intro">${esc(C.testimonials.note)}</p>
    </header>
    <div class="quote-grid">${testimonials}</div>
  </section>

  <section id="colophon" class="colophon">
    <header class="sec-head">
      <span class="eyebrow">${esc(C.authorship.eyebrow)}</span>
      <h2>${esc(C.authorship.title)}</h2>
    </header>
    <div class="colophon-body">${C.authorship.body.map((p) => `<p>${esc(p)}</p>`).join("")}</div>
  </section>

  <section class="cta">
    <h2>${esc(C.cta.title)}</h2>
    <p>${esc(C.cta.body)}</p>
    <div class="actions">
      <a class="btn btn-solid" href="${esc(C.cta.primary.href)}" rel="noopener">${esc(C.cta.primary.label)}<span class="arrow">→</span></a>
      <a class="btn btn-ghost" href="${esc(C.cta.secondary.href)}" rel="noopener">${esc(C.cta.secondary.label)}</a>
    </div>
  </section>
</main>

<footer class="foot">
  <div class="foot-brand">${esc(C.brand.name)}${eq(4)}</div>
  <p class="foot-note">${esc(C.footer.note)}</p>
  <nav class="foot-links">${footLinks}</nav>
</footer>

<div class="lightbox" id="lightbox" role="dialog" aria-modal="true" aria-label="Screenshots" hidden>
  <button class="lb-btn lb-close" type="button" aria-label="Close (Esc)">✕</button>
  <button class="lb-btn lb-nav lb-prev" type="button" aria-label="Previous">‹</button>
  <button class="lb-btn lb-nav lb-next" type="button" aria-label="Next">›</button>
  <div class="lb-track">${lbSlides}</div>
  <p class="lb-cap" aria-live="polite"></p>
  <div class="lb-dots">${lbDots}</div>
</div>

<script>
(function () {
  var lb = document.getElementById("lightbox");
  if (!lb) return;
  var track = lb.querySelector(".lb-track");
  var slides = [].slice.call(lb.querySelectorAll(".lb-slide"));
  var dots = [].slice.call(lb.querySelectorAll(".lb-dot"));
  var capEl = lb.querySelector(".lb-cap");
  var caps = slides.map(function (s) { return s.getAttribute("data-cap") || ""; });
  var n = slides.length, idx = 0, lastFocus = null, raf = 0;

  function render() {
    dots.forEach(function (d, k) { d.setAttribute("aria-current", k === idx ? "true" : "false"); });
    capEl.textContent = caps[idx];
  }
  function goTo(i, smooth) {
    idx = Math.max(0, Math.min(n - 1, i));
    track.scrollTo({ left: slides[idx].offsetLeft, behavior: smooth ? "smooth" : "auto" });
    render();
  }
  function syncFromScroll() {
    var i = Math.round(track.scrollLeft / track.clientWidth);
    if (i !== idx && i >= 0 && i < n) { idx = i; render(); }
  }
  function open(i) {
    lastFocus = document.activeElement;
    lb.hidden = false;
    document.documentElement.style.overflow = "hidden";
    requestAnimationFrame(function () { goTo(i, false); });
    lb.querySelector(".lb-close").focus();
  }
  function close() {
    lb.hidden = true;
    document.documentElement.style.overflow = "";
    if (lastFocus && lastFocus.focus) lastFocus.focus();
  }

  [].slice.call(document.querySelectorAll("[data-lb]")).forEach(function (el) {
    el.addEventListener("click", function (e) { e.preventDefault(); open(+el.getAttribute("data-lb") || 0); });
    el.addEventListener("keydown", function (e) {
      if (e.key === "Enter" || e.key === " ") { e.preventDefault(); open(+el.getAttribute("data-lb") || 0); }
    });
  });

  track.addEventListener("scroll", function () {
    cancelAnimationFrame(raf);
    raf = requestAnimationFrame(syncFromScroll);
  }, { passive: true });

  lb.querySelector(".lb-prev").addEventListener("click", function () { goTo(idx - 1, true); });
  lb.querySelector(".lb-next").addEventListener("click", function () { goTo(idx + 1, true); });
  lb.querySelector(".lb-close").addEventListener("click", close);
  dots.forEach(function (d) {
    d.addEventListener("click", function () { goTo(+d.getAttribute("data-i") || 0, true); });
  });
  // Click outside the image (backdrop / slide padding) closes.
  lb.addEventListener("click", function (e) {
    if (e.target === lb || e.target.classList.contains("lb-slide") || e.target.classList.contains("lb-track")) close();
  });
  document.addEventListener("keydown", function (e) {
    if (lb.hidden) return;
    if (e.key === "Escape") close();
    else if (e.key === "ArrowLeft") goTo(idx - 1, true);
    else if (e.key === "ArrowRight") goTo(idx + 1, true);
  });
})();

// Theme carousel: scroll-snap track with arrows, dots, and gentle autoplay.
(function () {
  var track = document.querySelector(".tc-track");
  if (!track) return;
  var slides = [].slice.call(track.querySelectorAll(".tc-slide"));
  var dots = [].slice.call(document.querySelectorAll(".tc-dot"));
  var idx = 0, auto = null, t = null;
  function render() {
    slides.forEach(function (s, k) { s.classList.toggle("is-active", k === idx); });
    dots.forEach(function (d, k) { d.setAttribute("aria-current", k === idx ? "true" : "false"); });
  }
  function center(i) {
    var s = slides[i], max = track.scrollWidth - track.clientWidth;
    var c = s.offsetLeft - (track.clientWidth - s.offsetWidth) / 2;
    return Math.max(0, Math.min(c, max));
  }
  function go(i, smooth) {
    idx = (i + slides.length) % slides.length;
    track.scrollTo({ left: center(idx), behavior: smooth ? "smooth" : "auto" });
    render();
  }
  function sync() {
    var mid = track.scrollLeft + track.clientWidth / 2, min = Infinity, best = 0;
    slides.forEach(function (s, k) {
      var d = Math.abs(s.offsetLeft + s.offsetWidth / 2 - mid);
      if (d < min) { min = d; best = k; }
    });
    idx = best; render();
  }
  track.addEventListener("scroll", function () { clearTimeout(t); t = setTimeout(sync, 90); }, { passive: true });
  var prev = document.querySelector(".tc-prev"), next = document.querySelector(".tc-next");
  if (prev) prev.addEventListener("click", function () { stop(); go(idx - 1, true); });
  if (next) next.addEventListener("click", function () { stop(); go(idx + 1, true); });
  dots.forEach(function (d) { d.addEventListener("click", function () { stop(); go(+d.getAttribute("data-i") || 0, true); }); });
  function stop() { if (auto) { clearInterval(auto); auto = null; } }
  function start() {
    if (auto || window.matchMedia("(prefers-reduced-motion: reduce)").matches) return;
    auto = setInterval(function () { go(idx + 1, true); }, 4500);
  }
  ["pointerdown", "mouseenter", "focusin"].forEach(function (ev) { track.addEventListener(ev, stop); });
  render(); start();
})();
</script>
</body>
</html>
`;

const footLinksHtml = C.footer.links
  .map((l) => `<a href="${esc(l.href)}" rel="noopener">${esc(l.label)}</a>`)
  .join("");

const fmtBytes = (n) => {
  if (!n) return "";
  const mb = n / (1024 * 1024);
  return mb >= 1 ? mb.toFixed(1) + " MB" : Math.round(n / 1024) + " KB";
};

// resolveDownloads enriches each download option that has a `file` with the
// staged binary's SHA-256 + size (computed here, so local and CI builds get the
// real hash). A missing binary renders as "not built" rather than failing — so a
// plain `node build.mjs` (no binaries staged) still produces the page.
async function resolveDownloads() {
  const out = [];
  for (const o of C.download.options) {
    if (!o.file) {
      out.push({ ...o });
      continue;
    }
    try {
      const buf = await fs.readFile(path.join(SRC, o.file));
      out.push({ ...o, present: true, size: buf.length, hash: createHash("sha256").update(buf).digest("hex") });
    } catch {
      out.push({ ...o, present: false });
    }
  }
  return out;
}

// Simplified, brand-coloured inline SVG marks (self-contained — no external
// assets). Recognisable rather than pixel-exact; ~22px, drawn on a 24px grid.
const LOGOS = {
  raspberrypi: `<svg class="dl-logo" viewBox="0 0 24 24" width="22" height="22" role="img" aria-label="Raspberry Pi">
    <g fill="#75A928"><ellipse cx="9.6" cy="6.6" rx="1.5" ry="2.9" transform="rotate(-32 9.6 6.6)"/><ellipse cx="14.4" cy="6.6" rx="1.5" ry="2.9" transform="rotate(32 14.4 6.6)"/></g>
    <g fill="#C7203E"><circle cx="11" cy="12" r="2"/><circle cx="14.4" cy="12.3" r="2"/><circle cx="9.2" cy="14.6" r="2"/><circle cx="12.6" cy="14.8" r="2"/><circle cx="15.6" cy="15" r="1.8"/><circle cx="11" cy="17.4" r="1.9"/><circle cx="14" cy="17.6" r="1.7"/></g>
  </svg>`,
  docker: `<svg class="dl-logo" viewBox="0 0 24 24" width="22" height="22" role="img" aria-label="Docker" fill="#2496ED">
    <g><rect x="6" y="9.4" width="2.4" height="2.4"/><rect x="8.8" y="9.4" width="2.4" height="2.4"/><rect x="11.6" y="9.4" width="2.4" height="2.4"/><rect x="8.8" y="6.6" width="2.4" height="2.4"/><rect x="11.6" y="6.6" width="2.4" height="2.4"/></g>
    <path d="M4 12.4h15.2c.2 1.4-.4 2.6-1.7 2.6.1-1.1-2-1.5-2.4-.4-1 1.9-3.4 3.1-6.3 3.1C5.6 17.7 4 15.4 4 12.4z"/>
  </svg>`,
  fedora: `<svg class="dl-logo" viewBox="0 0 24 24" width="22" height="22" role="img" aria-label="Fedora">
    <circle cx="12" cy="12" r="10" fill="#51A2DA"/>
    <path d="M13.7 6.6a3.2 3.2 0 0 0-3.2 3.2v1.4H8.8v2.3h1.7v4h2.4v-4h2v-2.3h-2v-1.4c0-.5.4-.9.9-.9h1.7V6.6z" fill="#fff"/>
  </svg>`,
  ubuntu: `<svg class="dl-logo" viewBox="0 0 24 24" width="22" height="22" role="img" aria-label="Ubuntu">
    <circle cx="12" cy="12" r="5.4" fill="none" stroke="#E95420" stroke-width="1.7"/>
    <g fill="#E95420"><circle cx="12" cy="5.3" r="1.8"/><circle cx="6.2" cy="15.3" r="1.8"/><circle cx="17.8" cy="15.3" r="1.8"/></g>
  </svg>`,
  debian: `<svg class="dl-logo" viewBox="0 0 24 24" width="22" height="22" role="img" aria-label="Debian" fill="none" stroke="#A80030" stroke-width="1.7" stroke-linecap="round">
    <path d="M15.6 8a5.2 5.2 0 1 0 1.5 4.7"/>
  </svg>`,
  arch: `<svg class="dl-logo" viewBox="0 0 24 24" width="22" height="22" role="img" aria-label="Arch Linux" fill="#1793D1">
    <path d="M12 3.5 4.5 20l7.5-3.6L19.5 20z"/>
  </svg>`,
  manjaro: `<svg class="dl-logo" viewBox="0 0 24 24" width="22" height="22" role="img" aria-label="Manjaro" fill="#35BF5C">
    <rect x="4" y="4" width="5" height="16" rx="1"/><rect x="10.5" y="9.5" width="5" height="10.5" rx="1"/><rect x="17" y="4" width="3" height="16" rx="1"/>
  </svg>`,
};

const logosHtml = (keys = []) => {
  const svgs = keys.map((k) => LOGOS[k]).filter(Boolean).join("");
  return svgs ? `<div class="dl-logos">${svgs}</div>` : "";
};

function downloadCard(o) {
  const head = `
        <div class="dl-card-head">
          <div class="dl-card-head-l">
            <h3>${esc(o.name)}</h3>
            <p class="dl-rec">${esc(o.rec)}</p>
          </div>
          <div class="dl-card-meta">
            <span class="tag">${esc(o.arch)}</span>
            ${logosHtml(o.logos)}
          </div>
        </div>`;
  // o.note is trusted author HTML from content.mjs (may carry <code>/<strong>),
  // so it is intentionally not run through esc(). Rendered full-width under head.
  const note = o.note ? `\n        <p class="dl-note">${o.note}</p>` : "";
  if (o.docker) {
    // o.body is trusted author HTML too — a plain explanatory paragraph that
    // sits above the --network host callout (o.note) and the run command.
    const body = o.body ? `\n        <p class="dl-body">${o.body}</p>` : "";
    return `
      <article class="dl-card">${head}${body}${note}
        <div class="dl-cmd dl-cmd-multi">
          <code>${esc(o.docker)}</code>
          <button class="dl-copy" type="button" data-copy="${esc(o.docker)}" aria-label="Copy command">copy</button>
        </div>
      </article>`;
  }
  const fname = o.file.split("/").pop();
  if (!o.present) {
    return `
      <article class="dl-card">${head}${note}
        <p class="dl-missing">Binary not staged — run <code>site/build.sh</code> (or build from a tagged CI pipeline).</p>
      </article>`;
  }
  return `
      <article class="dl-card">${head}${note}
        <div class="dl-card-foot">
          <div class="dl-file">
            <span class="dl-fname"><code>${esc(fname)}</code> <span class="dl-size">${esc(fmtBytes(o.size))}</span></span>
            <span class="dl-sha"><span class="dl-sha-label">SHA-256</span><code>${esc(o.hash)}</code></span>
          </div>
          <a class="btn btn-solid dl-dl" href="${esc(o.file)}" download>Download<span class="arrow">↓</span></a>
        </div>
      </article>`;
}

function downloadPage(options) {
  const cards = options.map(downloadCard).join("");
  const t = C.download.esp32;
  const esp32 = t
    ? `
    <section class="dl-teaser" aria-label="${esc(t.title)}">
      <div class="dl-teaser-head">
        <h2>${esc(t.title)}</h2>
        <span class="dl-soon">${esc(t.badge)}</span>
      </div>
      <p class="dl-teaser-body">${esc(t.body)}</p>${
        t.href
          ? `\n      <a class="btn btn-ghost dl-link-btn" href="${esc(t.href)}">${esc(t.hrefLabel)}<span class="arrow">→</span></a>`
          : ""
      }
    </section>`
    : "";
  const inst = C.download.installer;
  const iw = inst && inst.walkthrough;
  const installer = inst
    ? `
    <article class="dl-card dl-installer">
      <div class="dl-card-head"><div class="dl-card-head-l"><h3>${esc(inst.title)}</h3></div></div>
      <p class="dl-rec">${esc(inst.body)}</p>
      <div class="dl-cmd">
        <code>${esc(inst.code)}</code>
        <button class="dl-copy" type="button" data-copy="${esc(inst.code)}" aria-label="Copy command">copy</button>
      </div>${iw ? `
      <details class="dl-script">
        <summary>${esc(iw.summary)}</summary>
        <pre class="dl-code"><code>${esc(iw.script)}</code></pre>
        <a class="dl-doc" href="${esc(iw.href)}" rel="noopener">${esc(iw.hrefLabel)}<span class="arrow">→</span></a>
      </details>` : ""}
    </article>`
    : "";
  const fl = C.download.flags;
  const flags = fl
    ? `
    <section class="dl-flags">
      <header class="sec-head">
        <h2>${esc(fl.title)}</h2>
        <p class="sec-intro">${esc(fl.intro)}</p>
      </header>
      <table class="qs-table">
        <thead><tr>${fl.cols.map((c) => `<th>${esc(c)}</th>`).join("")}</tr></thead>
        <tbody>${fl.params
          .map(
            (p) =>
              `<tr><td><code class="qs-flag">${esc(p.param)}</code></td><td><code class="qs-env">${esc(p.env)}</code></td><td><code class="qs-def">${esc(p.def)}</code></td><td>${esc(p.what)}</td></tr>`
          )
          .join("")}</tbody>
      </table>${fl.doc ? `\n      <a class="qs-doc" href="${esc(fl.doc.href)}" rel="noopener">for further details — ${esc(fl.doc.label)}<span class="arrow">→</span></a>` : ""}
    </section>`
    : "";
  const links = C.download.links
    .map(
      (l) => `
      <div class="dl-link-row">
        <p class="dl-link-desc">${esc(l.desc)}</p>
        <a class="btn btn-ghost dl-link-btn" href="${esc(l.href)}" rel="noopener">${esc(l.label)}<span class="arrow">→</span></a>
      </div>`
    )
    .join("");
  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>Download — ${esc(C.meta.title)}</title>
<meta name="description" content="Download ensemble — pure-Go binaries for 64-bit Raspberry Pi (arm64) and x86-64 Linux, plus the Docker image. SHA-256 for every build." />
<meta name="theme-color" content="${esc(C.meta.themeColor)}" />
<link rel="preload" href="assets/fonts/fraunces-wght.woff2" as="font" type="font/woff2" crossorigin />
<link rel="preload" href="assets/fonts/plex-sans-400.woff2" as="font" type="font/woff2" crossorigin />
<link rel="stylesheet" href="assets/styles.css" />
</head>
<body>
<div class="grain" aria-hidden="true"></div>

<header class="nav">
  <a class="brand" href="index.html">${esc(C.brand.name)}<span class="brand-dot"></span></a>
  <nav class="nav-links">${renderNav("index.html")}</nav>
  <a class="btn btn-ghost nav-cta" href="index.html">← Home</a>
</header>

<main id="top">
  <section class="dl">
    <header class="sec-head">
      <span class="eyebrow">${eq(6)}${esc(C.download.eyebrow)}${VERSION ? " · " + esc(VERSION) : ""}</span>
      <h1>${esc(C.download.title)}</h1>
      <p class="sec-intro">${esc(C.download.intro)}</p>
    </header>
    <div class="dl-list">${esp32}${installer}${cards}</div>
    ${flags}
    <div class="dl-links">${links}</div>
  </section>
</main>

<footer class="foot">
  <div class="foot-brand">${esc(C.brand.name)}${eq(4)}</div>
  <p class="foot-note">${esc(C.footer.note)}</p>
  <nav class="foot-links">${footLinksHtml}</nav>
</footer>

<script>
(function () {
  document.querySelectorAll(".dl-copy").forEach(function (b) {
    b.addEventListener("click", function () {
      var t = b.getAttribute("data-copy") || "";
      (navigator.clipboard ? navigator.clipboard.writeText(t) : Promise.reject()).then(
        function () { var o = b.textContent; b.textContent = "copied"; setTimeout(function () { b.textContent = o; }, 1200); },
        function () {}
      );
    });
  });
})();
</script>
</body>
</html>
`;
}

// resolveFirmware enriches each board build with its merged-image's SHA-256 +
// size (like resolveDownloads), and records the resolved `src` path so main()
// can copy the image into ./dist. The image is looked up first where the build
// staged it (src/assets/firmware/, as CI's docker-site job does), then falling
// back to a local esp32 build (esp32/build-<id>/ensemble-fw-<id>.bin) so a bare
// `node build.mjs` after `esp32/build.sh <id>` produces a working flasher too.
// A missing image renders as "not built" — the page + manifests still build.
async function resolveFirmware() {
  const out = [];
  for (const b of C.firmware.builds) {
    const fname = b.file.split("/").pop();
    const candidates = [
      path.join(SRC, b.file),
      path.join(root, "..", "esp32", `build-${b.id}`, fname),
    ];
    let hit = null;
    for (const p of candidates) {
      try {
        hit = { src: p, buf: await fs.readFile(p) };
        break;
      } catch {
        // try the next location
      }
    }
    if (hit) {
      out.push({ ...b, present: true, src: hit.src, size: hit.buf.length, hash: createHash("sha256").update(hit.buf).digest("hex") });
    } else {
      out.push({ ...b, present: false });
    }
  }
  return out;
}

// Per-board ESP Web Tools manifest: a single merged image at offset 0 for the
// board's chipFamily. `path` is relative to the manifest's own location. Two
// variants per board so the wizard's "Fresh install" toggle picks the right one:
//   manifest-<id>.json       fresh: new_install_prompt_erase:false ⇒ ESP Web
//                            Tools full-erases the chip up front, then writes —
//                            a clean install, no extra in-dialog question.
//   manifest-<id>-keep.json  keep: new_install_prompt_erase:true ⇒ the installer
//                            asks erase-or-keep, so a re-flash can preserve the
//                            node's stored Wi-Fi/name (pick "Keep").
// (ESP Web Tools has no silent "keep" — preserving NVS requires that one prompt.)
// Several boards can share a chipFamily, hence one manifest pair per board.
function firmwareManifest(b, fresh) {
  return {
    name: `${C.firmware.manifestName} — ${b.label}`,
    version: VERSION || "dev",
    new_install_prompt_erase: !fresh,
    builds: [
      {
        chipFamily: b.chipFamily,
        parts: [{ path: b.file.split("/").pop(), offset: 0 }],
      },
    ],
  };
}

function flashPage(builds) {
  const F = C.flash;
  const bom = F.bom.items.map((i) => `<li>${esc(i)}</li>`).join("");

  // The progress header — one numbered chip per wizard step. The first is current
  // on load; pick()/show() in the page script move the is-current/is-done classes.
  const stepChips = F.wizard
    .map(
      (s, i) =>
        `<li class="fl-step-chip${i === 0 ? " is-current" : ""}" data-step="${esc(s.id)}">
            <span class="fl-step-n">${i + 1}</span><span class="fl-step-label">${esc(s.label)}</span>
          </li>`
    )
    .join("");

  // One photo card per board in a horizontal scroll row; nothing board-specific
  // shows until one is picked.
  const boardCards = builds
    .map(
      (b) =>
        `<button type="button" class="fl-board-card" role="radio" aria-checked="false" data-id="${esc(b.id)}">
            <img src="${esc(b.img)}" alt="${esc(b.label)}" loading="lazy" decoding="async" />
            <span class="fl-board-name">${esc(b.label)}</span>
          </button>`
    )
    .join("");

  // Board metadata for the picker JS: image, both manifests (fresh / keep), the
  // wiring diagram, staged state, and the merged-image filename/size/sha rendered
  // once a board is selected.
  const boardData = builds.map((b) => ({
    id: b.id,
    label: b.label,
    note: b.note,
    img: b.img,
    doc: b.doc || "",
    manifest: `assets/firmware/manifest-${b.id}.json`,
    manifestKeep: `assets/firmware/manifest-${b.id}-keep.json`,
    present: !!b.present,
    file: b.file.split("/").pop(),
    size: b.present ? fmtBytes(b.size) : "",
    hash: b.present ? b.hash : "",
  }));
  // </ inside the JSON would close the <script>; escape the opening angle.
  const boardJson = JSON.stringify(boardData).replace(/</g, "\\u003c");
  // Status copy injected into the script the same way (avoids hand-escaping).
  const msgJson = JSON.stringify({
    flashOk: F.install.okMsg,
    flashErr: F.install.errMsg,
    flashUnknown: F.install.unknownMsg,
    cfgOk: F.provision.okMsg,
    cfgErr: F.provision.errMsg,
    notBuilt: "Firmware isn’t staged for this board — built by the CI firmware job, or run esp32/build.sh ",
    noSerial: "Web Serial needs Chrome or Edge on desktop.",
    connecting: "Connecting over USB…",
    saving: "Saving configuration…",
    noPort: "No serial port selected.",
    consoleHint: "Connect over USB to watch the device boot.",
    flashDone: "Flashing finished — startup logs will appear here soon…",
    cfgReboot: "Configuration saved — the device is rebooting; startup logs will appear here soon…",
    noReply: "No reply from the device. Make sure it’s plugged in over USB-C (not just power), then reconnect.",
  }).replace(/</g, "\\u003c");

  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>Flash a player — ${esc(C.meta.title)}</title>
<meta name="description" content="Flash an ESP32 + I2S DAC ensemble player from your browser — ESP Web Tools + Web Serial provisioning. No toolchain." />
<meta name="theme-color" content="${esc(C.meta.themeColor)}" />
<link rel="preload" href="assets/fonts/fraunces-wght.woff2" as="font" type="font/woff2" crossorigin />
<link rel="preload" href="assets/fonts/plex-sans-400.woff2" as="font" type="font/woff2" crossorigin />
<link rel="stylesheet" href="assets/styles.css" />
<script type="module" src="https://unpkg.com/esp-web-tools@10/dist/web/install-button.js?module"></script>
<style>
  /* The flasher is themed off the site's mint accent (var(--accent)); only the
     genuine cautions — the Unstable banner and the band-steering note — use a
     warning amber so they read as warnings, not brand. */
  .fl-warn-amber{--fl-amber:#e0a64a}

  /* ── progress stepper ─────────────────────────────────────────────── */
  .fl-steps{display:flex;list-style:none;padding:0;margin:26px 0 20px;gap:0;flex-wrap:wrap}
  .fl-step-chip{display:flex;align-items:center;gap:10px;font-size:13.5px;font-weight:600;color:var(--faint);flex:1 1 0;min-width:140px}
  .fl-step-chip::after{content:"";flex:1;height:1px;background:var(--line-2);margin:0 12px}
  .fl-step-chip:last-child::after{display:none}
  .fl-step-n{flex:none;display:grid;place-items:center;width:28px;height:28px;border-radius:50%;border:1px solid var(--line-2);font-family:var(--mono);font-size:13px;color:var(--faint);background:var(--bg-2)}
  .fl-step-chip.is-current{color:var(--ink)}
  .fl-step-chip.is-current .fl-step-n{border-color:var(--accent);color:var(--accent);box-shadow:0 0 0 3px color-mix(in srgb,var(--accent) 18%,transparent)}
  .fl-step-chip.is-done{color:var(--ink)}
  .fl-step-chip.is-done .fl-step-n{background:var(--accent);border-color:var(--accent);color:var(--accent-ink)}
  @media (max-width:560px){ .fl-step-label{display:none} .fl-step-chip{min-width:0;flex:0 0 auto} .fl-step-chip::after{min-width:18px} }

  /* ── wizard card + per-step panels ────────────────────────────────── */
  .fl-wizard{background:var(--panel);border:1px solid var(--line);border-radius:16px;padding:26px 28px;margin:0 0 22px}
  /* these panels are <section>s, so zero out the global section{padding:clamp(72px…)} */
  .fl-step{display:flex;flex-direction:column;padding:0;animation:flfade .25s ease}
  .fl-step[hidden]{display:none}
  @keyframes flfade{from{opacity:0;transform:translateY(6px)}to{opacity:1;transform:none}}
  .fl-step h2{margin:0 0 6px;font-family:var(--serif);font-size:25px}
  .fl-lead{margin:0 0 18px;color:var(--muted);max-width:60ch}

  /* footer: action (primary) + next (outline), generously spaced */
  .fl-foot{margin-top:24px;padding-top:20px;display:flex;align-items:center;justify-content:space-between;gap:18px;flex-wrap:wrap;border-top:1px solid var(--line)}
  .fl-foot-r{display:flex;align-items:center;gap:14px;flex-wrap:wrap;margin-left:auto}
  .fl-foot .btn[disabled],.fl-foot .btn:disabled{opacity:.4;pointer-events:none}
  .btn-back{padding-left:8px}

  /* ── board picker ─────────────────────────────────────────────────── */
  .fl-board-row{display:flex;gap:14px;overflow-x:auto;padding:6px 2px 14px;margin:0 -2px;scroll-snap-type:x proximity;-webkit-overflow-scrolling:touch}
  .fl-board-card{flex:0 0 220px;scroll-snap-align:start;display:flex;flex-direction:column;gap:10px;align-items:center;text-align:center;background:var(--bg-2);border:1px solid var(--line-2);border-radius:12px;padding:14px;color:inherit;font:inherit;cursor:pointer;transition:border-color .15s,box-shadow .15s}
  .fl-board-card:hover{border-color:color-mix(in srgb,var(--accent) 55%,var(--line-2))}
  .fl-board-card.is-active{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent),0 0 34px -16px var(--accent)}
  .fl-board-card img{width:100%;aspect-ratio:4/3;object-fit:contain;border-radius:8px;background:var(--bg)}
  .fl-board-name{font-size:14px;font-weight:600}

  /* selected-board meta line (staged sha / not built) */
  .fl-build{margin:4px 0 2px;padding:14px 0;border-top:1px solid var(--line)}
  .fl-build strong{font-weight:600}
  .fl-build-meta{font-size:13px;color:var(--muted);display:flex;gap:8px;align-items:center;flex-wrap:wrap;margin-top:6px}
  .fl-build-meta code{font-family:var(--mono);font-size:12px}
  .fl-sha{font-size:11px;word-break:break-all;max-width:100%}
  .fl-ok{color:var(--accent)}.fl-no{color:#e0764a}
  .fl-muted{color:var(--muted)}

  /* "what you need" aside on step 1 */
  .fl-bom{margin:14px 0 0;background:var(--bg-2);border:1px solid var(--line);border-radius:12px;padding:6px 16px}
  .fl-bom>summary{cursor:pointer;font-weight:600;font-size:14px;padding:10px 0;list-style:none}
  .fl-bom>summary::-webkit-details-marker{display:none}
  .fl-bom>summary::before{content:"+ ";color:var(--accent);font-family:var(--mono)}
  .fl-bom[open]>summary::before{content:"– "}
  .fl-bom ul{margin:0 0 12px;padding-left:20px;color:var(--muted);font-size:14.5px}

  /* ── install step ─────────────────────────────────────────────────── */
  .fl-check{display:flex;gap:12px;align-items:flex-start;background:var(--bg-2);border:1px solid var(--line);border-radius:12px;padding:14px 16px;margin:16px 0;font-size:14px;line-height:1.55;cursor:pointer}
  .fl-check input{margin-top:3px;width:17px;height:17px;accent-color:var(--accent);flex:none}
  .fl-check strong{color:var(--ink)}
  .fl-check span{color:var(--muted)}

  /* ── provision step ───────────────────────────────────────────────── */
  .fl-grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(190px,1fr));gap:16px;margin:18px 0 6px}
  .fl-field{display:flex;flex-direction:column;gap:6px;font-size:14px}
  .fl-field label{color:var(--muted)}
  .fl-field input{background:var(--bg-2);border:1px solid var(--line-2);border-radius:9px;color:inherit;padding:10px 12px;font:inherit}
  .fl-field input:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 3px color-mix(in srgb,var(--accent) 16%,transparent)}

  /* warning/heads-up box (amber, semantic) */
  .fl-warn-box{display:flex;gap:12px;align-items:flex-start;background:rgba(224,166,74,.08);border:1px solid rgba(224,166,74,.4);border-radius:12px;padding:14px 16px;margin:6px 0 4px;color:#e7d3a8;font-size:13.5px;line-height:1.55}
  .fl-warn-box .fl-tag{flex:none;font-size:11px;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:var(--bg);background:#e0a64a;border-radius:999px;padding:3px 9px;margin-top:1px}
  .fl-warn{color:#e0a64a}

  /* serial console / boot-log terminal, revealed on flash + configure success */
  .fl-console{margin:14px 0 2px;border:1px solid var(--line);border-radius:10px;overflow:hidden;background:var(--bg)}
  .fl-console-bar{display:flex;align-items:center;justify-content:space-between;gap:10px;padding:8px 12px;background:var(--bg-2);border-bottom:1px solid var(--line);font-family:var(--mono);font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--muted)}
  .fl-console-connect{font:inherit;text-transform:none;letter-spacing:0;color:var(--accent);background:none;border:1px solid color-mix(in srgb,var(--accent) 45%,transparent);border-radius:7px;padding:4px 10px;cursor:pointer}
  .fl-console-connect:hover{background:color-mix(in srgb,var(--accent) 12%,transparent)}
  .fl-log{margin:0;padding:12px;font-family:var(--mono);font-size:12px;line-height:1.5;white-space:pre-wrap;word-break:break-word;max-height:240px;overflow:auto;color:#9aa3b2}

  /* status lines under install / configure */
  .fl-status{margin:14px 0 2px;padding:11px 14px;border-radius:10px;font-size:14px;line-height:1.5}
  .fl-status.ok{background:color-mix(in srgb,var(--accent) 12%,transparent);border:1px solid color-mix(in srgb,var(--accent) 45%,transparent);color:color-mix(in srgb,var(--accent) 85%,white)}
  .fl-status.err{background:rgba(224,118,74,.1);border:1px solid rgba(224,118,74,.45);color:#f0a988}
  .fl-status.info{background:var(--bg-2);border:1px solid var(--line);color:var(--muted)}

  /* ── finished step ────────────────────────────────────────────────── */
  .fl-done{text-align:center;padding:8px 0 4px}
  .fl-done-badge{width:64px;height:64px;border-radius:50%;display:grid;place-items:center;margin:0 auto 16px;font-size:30px;color:var(--accent-ink);background:var(--accent);box-shadow:0 0 0 8px color-mix(in srgb,var(--accent) 16%,transparent),0 10px 40px -12px var(--accent)}
  .fl-done h2{margin:0 0 8px}
  .fl-done .fl-lead{margin:0 auto;text-align:center}
  .fl-done-doc{margin-top:20px}

  /* ── unstable banner (amber, semantic) ────────────────────────────── */
  .fl-unstable{display:flex;gap:12px;align-items:flex-start;background:rgba(224,166,74,.09);border:1px solid rgba(224,166,74,.4);border-radius:12px;padding:14px 16px;margin:18px 0 2px;color:#e7d3a8;font-size:14px;line-height:1.55}
  .fl-unstable .fl-tag{flex:none;font-size:11px;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:var(--bg);background:#e0a64a;border-radius:999px;padding:3px 9px;margin-top:1px}
</style>
</head>
<body>
<div class="grain" aria-hidden="true"></div>

<header class="nav">
  <a class="brand" href="index.html">${esc(C.brand.name)}<span class="brand-dot"></span></a>
  <nav class="nav-links">${renderNav("index.html")}</nav>
  <a class="btn btn-ghost nav-cta" href="index.html">← Home</a>
</header>

<main id="top">
  <section class="dl">
    <header class="sec-head">
      <span class="eyebrow">${eq(6)}${esc(F.eyebrow)}${VERSION ? " · " + esc(VERSION) : ""}</span>
      <h1>${esc(F.title)}</h1>
      <p class="sec-intro">${esc(F.intro)}</p>
    </header>

    <div class="fl-unstable" role="note"><span class="fl-tag">Unstable</span><span>${esc(F.unstable)}</span></div>

    <ol class="fl-steps" id="fl-steps">${stepChips}</ol>

    <div class="fl-wizard">
      <!-- Step 1 — select board -->
      <section class="fl-step" data-step="board">
        <h2>${esc(F.board.title)}</h2>
        <p class="fl-lead">${esc(F.board.body)}</p>
        <div class="fl-board-row" role="radiogroup" aria-label="${esc(F.board.title)}">
          ${boardCards}
        </div>
        <div class="fl-build" id="fl-build" hidden></div>
        <details class="fl-bom">
          <summary>${esc(F.bom.title)}</summary>
          <ul>${bom}</ul>
        </details>
        <div class="fl-foot">
          <span></span>
          <div class="fl-foot-r">
            <button class="btn btn-solid fl-next" data-go="install" id="fl-next-board" disabled>${esc(F.board.next)} <span class="arrow">→</span></button>
          </div>
        </div>
      </section>

      <!-- Step 2 — install -->
      <section class="fl-step" data-step="install" hidden>
        <h2>${esc(F.install.title)}</h2>
        <p class="fl-lead">${esc(F.install.requirements)}</p>
        <div class="fl-build" id="fl-build-2"></div>
        <label class="fl-check">
          <input type="checkbox" id="fl-fresh" checked />
          <span><strong>${esc(F.install.fresh.label)}</strong> — ${esc(F.install.fresh.note)}</span>
        </label>
        <div class="fl-status" id="fl-install-status" hidden></div>
        <div class="fl-console" id="fl-console-install" hidden>
          <div class="fl-console-bar"><span>Console</span><button type="button" class="fl-console-connect" id="fl-connect-install" hidden>Connect over USB</button></div>
          <pre class="fl-log" id="fl-log-install" aria-live="polite"></pre>
        </div>
        <div class="fl-foot">
          <button class="btn btn-ghost btn-back fl-back" data-go="board" type="button"><span class="arrow">←</span> Back</button>
          <div class="fl-foot-r">
            <esp-web-install-button id="fl-install">
              <button class="btn btn-solid" slot="activate">${esc(F.install.action)} <span class="arrow">↧</span></button>
              <span slot="unsupported" class="fl-warn">This browser can’t flash — use Chrome or Edge on desktop.</span>
              <span slot="not-allowed" class="fl-warn">Flashing needs a secure (https) page.</span>
            </esp-web-install-button>
            <button class="btn btn-ghost fl-next" data-go="provision" id="fl-next-install" disabled>${esc(F.install.next)} <span class="arrow">→</span></button>
          </div>
        </div>
      </section>

      <!-- Step 3 — provision -->
      <section class="fl-step" data-step="provision" hidden>
        <h2>${esc(F.provision.title)}</h2>
        <p class="fl-lead">${esc(F.provision.body)}</p>
        <div class="fl-warn-box" role="note"><span class="fl-tag">Heads up</span><span>${esc(F.provision.warning)}</span></div>
        <div class="fl-grid">
          <div class="fl-field"><label for="cf-name">${esc(F.provision.fields.name.label)}</label><input id="cf-name" placeholder="${esc(F.provision.fields.name.placeholder)}" autocomplete="off" /></div>
          <div class="fl-field"><label for="cf-wifi_ssid">${esc(F.provision.fields.ssid.label)}</label><input id="cf-wifi_ssid" placeholder="${esc(F.provision.fields.ssid.placeholder)}" autocomplete="off" /></div>
          <div class="fl-field"><label for="cf-wifi_pass">${esc(F.provision.fields.pass.label)}</label><input id="cf-wifi_pass" type="password" placeholder="${esc(F.provision.fields.pass.placeholder)}" autocomplete="off" /></div>
        </div>
        <div class="fl-status" id="fl-cfg-status" hidden></div>
        <div class="fl-console" id="fl-console-provision" hidden>
          <div class="fl-console-bar"><span>Console</span><button type="button" class="fl-console-connect" id="fl-connect-provision" hidden>Connect over USB</button></div>
          <pre class="fl-log" id="fl-log-provision" aria-live="polite"></pre>
        </div>
        <div class="fl-foot">
          <button class="btn btn-ghost btn-back fl-back" data-go="install" type="button"><span class="arrow">←</span> Back</button>
          <div class="fl-foot-r">
            <button class="btn btn-solid" id="fl-configure" type="button" disabled>${esc(F.provision.action)}</button>
            <button class="btn btn-ghost fl-next" data-go="finished" id="fl-next-provision" disabled>${esc(F.provision.next)} <span class="arrow">→</span></button>
          </div>
        </div>
      </section>

      <!-- Step 4 — finished -->
      <section class="fl-step" data-step="finished" hidden>
        <div class="fl-done">
          <div class="fl-done-badge" aria-hidden="true">✓</div>
          <h2>${esc(F.finished.title)}</h2>
          <p class="fl-lead">${esc(F.finished.body)}</p>
          <a id="fl-doc-link" class="btn btn-ghost fl-done-doc" href="${esc(F.docHref)}" rel="noopener">${esc(F.finished.docLink)} <span class="arrow">→</span></a>
        </div>
        <div class="fl-foot">
          <button class="btn btn-ghost btn-back fl-back" data-go="provision" type="button"><span class="arrow">←</span> Back</button>
          <div class="fl-foot-r">
            <a class="btn btn-solid" href="index.html">Done</a>
          </div>
        </div>
      </section>
    </div>

    <div class="dl-links">
      <div class="dl-link-row">
        <p class="dl-link-desc">Wiring diagrams, pinouts, the config protocol, and the build are all in the repo.</p>
        <a class="btn btn-ghost dl-link-btn" href="${esc(F.docHref)}" rel="noopener">${esc(F.docLabel)}<span class="arrow">→</span></a>
      </div>
      <div class="dl-link-row">
        <p class="dl-link-desc">Prefer prebuilt software nodes for a Pi or PC?</p>
        <a class="btn btn-ghost dl-link-btn" href="download.html">Download builds<span class="arrow">→</span></a>
      </div>
    </div>
  </section>
</main>

<footer class="foot">
  <div class="foot-brand">${esc(C.brand.name)}${eq(4)}</div>
  <p class="foot-note">${esc(C.footer.note)}</p>
  <nav class="foot-links">${footLinksHtml}</nav>
</footer>

<script>
(function () {
  function $(id){ return document.getElementById(id); }
  var BOARDS = {};
  (${boardJson}).forEach(function(b){ BOARDS[b.id] = b; });
  var MSG = ${msgJson};
  var ORDER = ["board","install","provision","finished"];
  var selected = null;

  // ── wizard navigation ─────────────────────────────────────────────────
  // One panel visible at a time; the stepper chips track current/done. Forward
  // moves are gated by per-step "next" buttons that JS only enables once that
  // step's condition is met, so you can't skip ahead.
  var panels = {}, chips = {};
  ORDER.forEach(function(id){
    panels[id] = document.querySelector('.fl-step[data-step="' + id + '"]');
    chips[id]  = document.querySelector('.fl-step-chip[data-step="' + id + '"]');
  });
  function show(id){
    var ci = ORDER.indexOf(id);
    ORDER.forEach(function(s, i){
      panels[s].hidden = s !== id;
      chips[s].classList.toggle("is-current", i === ci);
      chips[s].classList.toggle("is-done", i < ci);
    });
    var top = $("fl-steps");
    if (top && top.scrollIntoView) top.scrollIntoView({ behavior: "smooth", block: "start" });
  }
  [].forEach.call(document.querySelectorAll(".fl-next,.fl-back"), function(btn){
    btn.addEventListener("click", function(){ if (!btn.disabled) show(btn.getAttribute("data-go")); });
  });

  // ── step 1: board picker ──────────────────────────────────────────────
  var install = $("fl-install");
  var cards = [].slice.call(document.querySelectorAll(".fl-board-card"));
  function buildMeta(b){
    var head = "<strong>" + b.label + "</strong> <span class='fl-muted'>" + b.note + "</span>";
    if (b.present)
      return head + "<div class='fl-build-meta'><code>" + b.file + "</code> <span class='fl-ok'>staged</span> <span class='fl-muted'>" + b.size + "</span> <code class='fl-sha'>" + b.hash + "</code></div>";
    return head + "<div class='fl-build-meta'><code>" + b.file + "</code> <span class='fl-no'>not built</span> <span class='fl-muted'>— " + MSG.notBuilt + b.id + ".</span></div>";
  }
  // Point ESP Web Tools at the fresh-erase or keep-settings manifest per toggle.
  function applyManifest(){
    if (!selected || !selected.present) return;
    install.setAttribute("manifest", $("fl-fresh").checked ? selected.manifest : selected.manifestKeep);
  }
  function pick(id){
    var b = BOARDS[id];
    if (!b) return;
    selected = b;
    cards.forEach(function(c){ var on = c.getAttribute("data-id") === id; c.classList.toggle("is-active", on); c.setAttribute("aria-checked", on ? "true" : "false"); });
    var m1 = $("fl-build"); m1.hidden = false; m1.innerHTML = buildMeta(b);
    $("fl-build-2").innerHTML = buildMeta(b);
    var doc = $("fl-doc-link");
    if (b.doc){ doc.href = b.doc; doc.hidden = false; } else { doc.hidden = true; }
    install.style.display = b.present ? "" : "none";
    applyManifest();
    // Re-arm the install gate whenever the board changes.
    flashStatus(null);
    $("fl-next-install").disabled = true;
    $("fl-next-board").disabled = !b.present;   // can't flash a board with no image
  }
  cards.forEach(function(c){ c.addEventListener("click", function(){ pick(c.getAttribute("data-id")); }); });
  $("fl-fresh").addEventListener("change", applyManifest);

  // ── step 2: detect flash success ──────────────────────────────────────
  // ESP Web Tools runs in its own dialog appended to <body>; it exposes no
  // completion event, so we watch the dialog's reflected "state" attribute.
  // ERROR ⇒ failure. Otherwise, once it has entered INSTALL and then closes we
  // treat it as done (DASHBOARD looks identical before and after a flash, so the
  // INSTALL→close round-trip is the most reliable success signal we get). A
  // dialog dismissed without ever installing falls back to a neutral prompt that
  // still lets you continue — we never trap the user behind a missed signal.
  var sawInstall = false, sawError = false;
  function flashStatus(kind){
    var el = $("fl-install-status");
    if (!kind){ el.hidden = true; el.className = "fl-status"; el.textContent = ""; return; }
    el.hidden = false;
    el.className = "fl-status " + (kind === "error" ? "err" : kind === "ok" ? "ok" : "info");
    el.textContent = kind === "error" ? MSG.flashErr : kind === "ok" ? MSG.flashOk : MSG.flashUnknown;
    if (kind !== "error") {
      $("fl-next-install").disabled = false;   // unlock "Configure →"
      // esptool hard-resets into the app after flashing, so the board is already
      // booting — reuse the port ESP Web Tools was granted and show the boot log.
      openConsole("fl-log-install", false, "fl-connect-install", MSG.flashDone);
    }
  }
  function watchDialog(dlg){
    sawInstall = false; sawError = false; flashStatus(null);
    function read(){ var s = dlg.getAttribute("state"); if (s === "INSTALL") sawInstall = true; else if (s === "ERROR") sawError = true; }
    read();
    new MutationObserver(read).observe(dlg, { attributes: true, attributeFilter: ["state"] });
  }
  function isDialog(n){ return n.nodeName && n.nodeName.toLowerCase() === "ewt-install-dialog"; }
  new MutationObserver(function(muts){
    muts.forEach(function(m){
      [].forEach.call(m.addedNodes, function(n){ if (isDialog(n)) watchDialog(n); });
      [].forEach.call(m.removedNodes, function(n){ if (isDialog(n)) flashStatus(sawError ? "error" : sawInstall ? "ok" : "unknown"); });
    });
  }).observe(document.body, { childList: true });

  // ── shared serial console ─────────────────────────────────────────────
  // Line-JSON protocol (docs/developer/esp32.md §6.2): {"cmd":"get|set|reboot"}
  // replies {"ok":...}. Everything the firmware prints (boot + ESP_LOG lines)
  // streams raw into a terminal box so flash + configure both end with a reset
  // and a live look at the device booting.
  var port = null, writer = null, buf = "", waiters = [], logEl = null, wantConsole = false;
  function appendLog(s){ if (!logEl) return; var t = logEl.textContent + s; if (t.length > 12000) t = t.slice(-12000); logEl.textContent = t; logEl.scrollTop = logEl.scrollHeight; }
  function showConsole(id){ logEl = $(id); var box = logEl && logEl.closest(".fl-console"); if (box) box.hidden = false; }
  // Only an actual command reply ({"ok":...}) resolves a pending send() — plain
  // log lines (and stray JSON) are shown but never mistaken for a response, which
  // is what made the old configure return "ok" instantly without really working.
  function onLine(line){ line = line.trim(); if (!line) return; var o; try { o = JSON.parse(line); } catch(e){ return; } if (!o || !("ok" in o)) return; var w = waiters.shift(); if (w) w(o); }
  async function readLoop(){
    var dec = new TextDecoderStream();
    port.readable.pipeTo(dec.writable).catch(function(){});
    var r = dec.readable.getReader();
    for(;;){
      var out; try { out = await r.read(); } catch(e){ break; }
      if (out.done) break;
      appendLog(out.value);
      buf += out.value;
      var nl;
      while ((nl = buf.indexOf("\\n")) >= 0) { onLine(buf.slice(0, nl)); buf = buf.slice(nl + 1); }
    }
    try { if (writer) writer.releaseLock(); } catch(e){}
    writer = null; port = null; buf = "";   // closed — e.g. a reboot re-enumerated the USB-JTAG
  }
  async function openPort(p){ port = p; await port.open({ baudRate: 115200 }); writer = port.writable.getWriter(); buf = ""; readLoop(); }
  async function dropPort(){
    try { if (writer) writer.releaseLock(); } catch(e){}
    try { if (port) await port.close(); } catch(e){}
    writer = null; port = null; buf = "";
  }
  function send(obj){
    if (!writer) return Promise.resolve(null);
    return writer.write(new TextEncoder().encode(JSON.stringify(obj) + "\\n")).then(function(){
      return new Promise(function(res){ waiters.push(res); setTimeout(function(){ var i = waiters.indexOf(res); if (i >= 0){ waiters.splice(i,1); res(null); } }, 2500); });
    }, function(){ return null; });
  }
  // The first line after opening the USB-JTAG is sometimes dropped while the
  // device's console task spins up, so retry a few times before giving up.
  async function sendRetry(obj, tries){
    for (var i = 0; i < (tries || 3); i++){ var r = await send(obj); if (r) return r; }
    return null;
  }
  // esp_restart() drops the native USB-JTAG and it re-enumerates as a fresh port.
  if (navigator.serial){
    navigator.serial.addEventListener("connect", function(ev){
      if (wantConsole && !port){ appendLog("\\n[device reconnected]\\n"); openPort(ev.target).catch(function(){}); }
    });
    // Clear the stale handle when the device drops, so the next connect() opens a
    // fresh port instead of writing into a dead one (the old "no response" trap).
    navigator.serial.addEventListener("disconnect", function(){
      if (port){ appendLog("\\n[device disconnected]\\n"); writer = null; port = null; buf = ""; }
    });
  }
  async function connect(allowPrompt){
    if (port) return true;
    if (!navigator.serial) throw new Error(MSG.noSerial);
    var ports = await navigator.serial.getPorts();
    var p = ports[ports.length - 1] || (allowPrompt ? await navigator.serial.requestPort() : null);
    if (!p) return false;
    await openPort(p);
    return true;
  }
  // Reveal a step's console and attach to the device. allowPrompt=false reuses a
  // port the page was already granted (e.g. by ESP Web Tools) without a picker.
  // intro seeds an explanation so the user sees what's happening before logs flow.
  async function openConsole(logId, allowPrompt, connectBtnId, intro){
    showConsole(logId); wantConsole = true;
    if (intro) appendLog(intro + "\\n");
    try {
      var ok = await connect(allowPrompt);
      if (!ok && connectBtnId){ appendLog(MSG.consoleHint + "\\n"); $(connectBtnId).hidden = false; }
    } catch(e){ appendLog(e.message + "\\n"); if (connectBtnId) $(connectBtnId).hidden = false; }
  }
  ["fl-connect-install","fl-connect-provision"].forEach(function(id){
    var b = $(id); if (!b) return;
    b.addEventListener("click", function(){ b.hidden = true; wantConsole = true; connect(true).then(function(ok){ appendLog(ok ? "[connected]\\n" : "[no port selected]\\n"); }).catch(function(e){ appendLog(e.message + "\\n"); }); });
  });

  // ── step 3: provision ─────────────────────────────────────────────────
  // Push name + Wi-Fi (only filled fields, so a blank never clobbers a stored
  // value), then reset so the firmware re-reads config and joins Wi-Fi while the
  // console shows it boot.
  function cfgCheck(){ $("fl-configure").disabled = !($("cf-name").value.trim() && $("cf-wifi_ssid").value.trim()); }
  ["cf-name","cf-wifi_ssid"].forEach(function(id){ $(id).addEventListener("input", cfgCheck); });
  function cfgStatus(kind, msg){ var el = $("fl-cfg-status"); el.hidden = false; el.className = "fl-status " + kind; el.textContent = msg; }
  $("fl-configure").addEventListener("click", async function(){
    try {
      cfgStatus("info", MSG.connecting);
      showConsole("fl-log-provision"); wantConsole = true;
      var ok = await connect(true);
      if (!ok){ cfgStatus("err", MSG.noPort); return; }
      var cfg = {};
      var name = $("cf-name").value.trim(); if (name) cfg.name = name;
      var ssid = $("cf-wifi_ssid").value.trim(); if (ssid) cfg.wifi_ssid = ssid;
      var pass = $("cf-wifi_pass").value; if (pass) cfg.wifi_pass = pass;
      cfgStatus("info", MSG.saving);
      var r = await sendRetry({ cmd: "set", cfg: cfg }, 3);
      // No reply usually means the handle we reused went stale (a prior reset
      // re-enumerated the USB) — drop it, reconnect fresh, and try once more.
      if (!r){
        appendLog("[no reply — reconnecting…]\\n");
        await dropPort();
        if (await connect(true)) r = await sendRetry({ cmd: "set", cfg: cfg }, 3);
      }
      if (!(r && r.ok)){ cfgStatus("err", r && r.err ? MSG.cfgErr + " (" + r.err + ")" : MSG.noReply); return; }
      cfgStatus("ok", MSG.cfgOk);
      appendLog("\\n[" + MSG.cfgReboot + "]\\n");
      send({ cmd: "reboot" });                     // apply Wi-Fi + join the LAN
      $("fl-next-provision").disabled = false;     // unlock "Finish →"
    } catch(e){ cfgStatus("err", "Connect/save failed: " + e.message); }
  });
})();
</script>
</body>
</html>
`;
}

async function copyDir(from, to) {
  await fs.mkdir(to, { recursive: true });
  for (const e of await fs.readdir(from, { withFileTypes: true })) {
    const s = path.join(from, e.name);
    const d = path.join(to, e.name);
    if (e.isDirectory()) await copyDir(s, d);
    else await fs.copyFile(s, d);
  }
}

async function main() {
  await fs.rm(OUT, { recursive: true, force: true });
  await fs.mkdir(OUT, { recursive: true });
  await copyDir(path.join(SRC, "assets"), path.join(OUT, "assets"));
  await fs.writeFile(path.join(OUT, "index.html"), page);

  const downloads = await resolveDownloads();
  const dl = downloadPage(downloads);
  await fs.writeFile(path.join(OUT, "download.html"), dl);

  // Flasher page + one ESP Web Tools manifest per board (manifest-<id>.json). Each
  // present board's merged image is copied into ./dist here — resolveFirmware may
  // have found it under src/assets/firmware/ OR a local esp32/build-<id>/, so we
  // copy from wherever it resolved rather than relying on copyDir(assets) alone.
  const firmware = await resolveFirmware();
  await fs.writeFile(path.join(OUT, "flash.html"), flashPage(firmware));
  const fwOut = path.join(OUT, "assets", "firmware");
  await fs.mkdir(fwOut, { recursive: true });
  for (const b of firmware) {
    // Fresh-install (full erase) + keep-settings (prompt) manifest per board.
    await fs.writeFile(
      path.join(fwOut, `manifest-${b.id}.json`),
      JSON.stringify(firmwareManifest(b, true), null, 2)
    );
    await fs.writeFile(
      path.join(fwOut, `manifest-${b.id}-keep.json`),
      JSON.stringify(firmwareManifest(b, false), null, 2)
    );
    if (b.present) await fs.copyFile(b.src, path.join(fwOut, b.file.split("/").pop()));
  }

  // Serve the installer at /get.sh (the "curl … | sudo bash" one-liner). Source of
  // truth is ../scripts/get.sh; the CI docker-site job stages a copy to site/get.sh
  // for the Docker build context.
  let getSh = false;
  for (const p of [path.join(root, "..", "scripts", "get.sh"), path.join(root, "get.sh")]) {
    try {
      await fs.writeFile(path.join(OUT, "get.sh"), await fs.readFile(p), { mode: 0o755 });
      getSh = true;
      break;
    } catch {
      // not here; try the next location
    }
  }

  const staged = downloads.filter((o) => o.present).length;
  const total = downloads.filter((o) => o.file).length;
  const fwStaged = firmware.filter((b) => b.present).length;
  console.log(
    `built ./dist (index ${(page.length / 1024).toFixed(1)} kB, download ${(dl.length / 1024).toFixed(1)} kB; ${staged}/${total} binaries, ${fwStaged}/${firmware.length} firmware staged; get.sh ${getSh ? "✓" : "—"})`
  );
}

main().catch((e) => {
  console.error(e);
  process.exit(1);
});
