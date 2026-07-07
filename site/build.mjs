// Zero-dependency static-site generator for the ondaire marketing site.
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
const VERSION = process.env.ONDAIRE_VERSION || "";

const esc = (s = "") =>
  String(s).replace(/[&<>"]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;" }[c]));

const eq = (n = 7) =>
  `<span class="eq" aria-hidden="true">${Array.from({ length: n }, (_, i) => `<i style="--i:${i}"></i>`).join("")}</span>`;

// GitHub mark, sized to sit inline with the nav text (fills currentColor).
const GITHUB_ICON =
  '<svg class="gh-mark" viewBox="0 0 16 16" width="16" height="16" aria-hidden="true" fill="currentColor"><path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82a7.6 7.6 0 012-.27c.68 0 1.36.09 2 .27 1.53-1.03 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.01 8.01 0 0016 8c0-4.42-3.58-8-8-8z"/></svg>';

// The nav is now a shortlist of page links (install/ui/tech) + the source — no
// in-page anchors, so no prefix juggling: every link is a real URL or external.
// `active` is the current page's href (e.g. "ui.html") so its link highlights.
const renderNav = (active = "") =>
  C.nav
    .map((l) => {
      const ext = /^https?:/.test(l.href);
      const rel = ext ? ' rel="noopener"' : "";
      const icon = l.icon === "github" ? GITHUB_ICON : "";
      const current = l.href === active;
      const classes = [icon ? "nav-gh" : "", current ? "is-current" : ""].filter(Boolean).join(" ");
      const cls = classes ? ` class="${classes}"` : "";
      const aria = current ? ' aria-current="page"' : "";
      return `<a${cls} href="${esc(l.href)}"${rel}${aria}>${icon}${esc(l.label)}</a>`;
    })
    .join("");

// ── shared page chrome ──────────────────────────────────────────────────
// Favicon + PWA icon set (the whole assets/ dir is copied into ./dist).
const FAVICONS = `
<link rel="icon" href="assets/favicon.ico" sizes="any" />
<link rel="icon" type="image/svg+xml" href="assets/favicon.svg" />
<link rel="apple-touch-icon" href="assets/apple-touch-icon.png" />
<link rel="manifest" href="assets/site.webmanifest" />`;

// <head> contents shared by every page (og:image defaults to the overview shot).
const head = (title, description, og = "assets/img/overview.png") => `
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>${esc(title)}</title>
<meta name="description" content="${esc(description)}" />
<meta name="theme-color" content="${esc(C.meta.themeColor)}" />${FAVICONS}
<meta property="og:title" content="${esc(title)}" />
<meta property="og:description" content="${esc(description)}" />
<meta property="og:type" content="website" />
<meta property="og:image" content="${esc(og)}" />
<link rel="preload" href="assets/fonts/fraunces-wght.woff2" as="font" type="font/woff2" crossorigin />
<link rel="preload" href="assets/fonts/plex-sans-400.woff2" as="font" type="font/woff2" crossorigin />
<link rel="stylesheet" href="assets/styles.css" />`;

// Right-hand nav CTA: "Get it" on the marketing pages, "← Home" on download/flash.
const GET_IT_CTA = `<a class="btn btn-solid nav-cta" href="${esc(C.hero.primary.href)}" rel="noopener">${esc(C.hero.primary.label)}</a>`;
const HOME_CTA = `<a class="btn btn-ghost nav-cta" href="index.html">← Home</a>`;

// The combined mark + "ndaire" wordmark lockup, inlined so it renders crisp at
// a fixed 44px header height without a separate <img> request. Colors are
// fixed (mint badge / ink glyph / light wordmark) — the marketing site has no
// theme switcher, unlike the web UI header this geometry is shared with.
const BRAND_LOCKUP = `<svg class="brand-lockup" style="height:44px;width:auto;display:block" viewBox="-16.7 -74.0 281.7 108.0" xmlns="http://www.w3.org/2000/svg" xmlns:svg="http://www.w3.org/2000/svg"> <rect x="-6.7" y="-40.0" width="51.6" height="49.8" rx="11.0" fill="#35e3b3" /> <path d="M19.1999453125 -31.03995703125Q24.153515625 -31.03995703125 27.894296875000002 -29.014359375Q31.635078125 -26.988761718750002 33.727869140625 -23.395167968750002Q35.82066015625 -19.80157421875 35.82066015625 -15.05278515625Q35.82066015625 -10.53438671875 33.667072265625 -6.953591796875Q31.513484375 -3.372796875 27.71190625 -1.2896015625000001Q23.910328125 0.79359375 18.87995703125 0.79359375Q13.9455859375 0.79359375 10.211205078125001 -1.2512031250000002Q6.47682421875 -3.2960000000000003 4.368033203125 -6.895994140625Q2.2592421875 -10.49598828125 2.2592421875 -15.193578125Q2.2592421875 -19.782375000000002 4.4224296875 -23.347169921875Q6.5856171875000005 -26.91196484375 10.387195312500001 -28.9759609375Q14.1887734375 -31.03995703125 19.1999453125 -31.03995703125ZM21.47194140625 -2.5471328125Q23.66080078125 -2.89913671875 24.99204296875 -4.56315234375Q26.32328515625 -6.22716796875 26.675310546875004 -9.167982421875Q27.027335937500002 -12.108796875 26.25934375 -16.268796875Q25.52975 -20.35200390625 24.11852734375 -23.024013671875Q22.7073046875 -25.6960234375 20.796861328124997 -26.88962890625Q18.88641796875 -28.083234375 16.62716015625 -27.69923046875Q14.43830078125 -27.3472265625 13.10705859375 -25.6832109375Q11.77581640625 -24.0191953125 11.433390625000001 -21.08798046875Q11.090964843750001 -18.156765625000002 11.8397578125 -13.97756640625Q12.58215234375 -9.91355859375 13.986974609375 -7.23194921875Q15.391796875 -4.55033984375 17.308640625000002 -3.366333984375Q19.225484375 -2.182328125 21.47194140625 -2.5471328125Z" fill="#03130d" /> <g transform="translate(11.1)"> <path d="m 52.588605,-28.851195 v 23.7312848 q 0,0.9791836 0.281606,1.4399805 0.281605,0.4607969 0.851211,0.6655938 l 1.164832,0.3391992 q 1.017598,0.396789 1.017598,1.2607734 Q 55.903852,0 54.13109,0 H 41.747203 q -0.889578,0 -1.308769,-0.35519141 -0.419192,-0.3551914 -0.419192,-0.98237499 0,-0.4543906 0.287998,-0.8063828 0.287998,-0.3519922 0.876791,-0.537586 l 1.273633,-0.3328007 q 0.582406,-0.1983985 0.864012,-0.6559942 0.281605,-0.4575957 0.281605,-1.4367793 V -23.481699 q 0,-0.799988 -0.252799,-1.151985 -0.252798,-0.351996 -0.809599,-0.447992 l -1.67686,-0.134394 q -0.595195,-0.134395 -0.854392,-0.419188 -0.259197,-0.284793 -0.259197,-0.745582 0,-0.505586 0.319994,-0.844779 0.319994,-0.339194 1.171177,-0.627186 l 5.951922,-2.195164 q 1.203176,-0.428789 1.958362,-0.633584 0.755185,-0.204795 1.388767,-0.204795 1.011176,0 1.529563,0.563186 0.518386,0.563185 0.518386,1.471967 z m -0.99839,7.142359 -1.452774,-1.516769 1.273618,-1.100813 q 4.006371,-3.622367 6.883152,-5.174353 2.876781,-1.551987 5.449574,-1.551987 3.948789,0 6.143981,2.636789 2.195191,2.636789 2.649593,7.020782 l 1.804821,16.1856714 q 0.115199,1.0303867 0.364806,1.5199843 0.249608,0.4895977 0.812815,0.6879961 l 1.158426,0.32 q 0.588793,0.1855938 0.876791,0.537586 0.287998,0.3519922 0.287998,0.8063828 0,0.62718359 -0.406391,0.98237499 Q 77.030234,0 76.115055,0 H 63.60957 q -1.772761,0 -1.772761,-1.4143633 0,-0.8639844 1.017597,-1.2607734 l 1.203235,-0.3391992 q 0.620808,-0.2047969 0.953613,-0.6911954 0.332805,-0.4863984 0.217605,-1.5039843 L 63.577633,-20.051223 q -0.320008,-2.816047 -1.436828,-4.20807 -1.116821,-1.392023 -3.254453,-1.392023 -1.350422,0 -2.85125,0.707216 -1.500829,0.707217 -3.145676,2.083256 z" fill="#e9eef4" /> <path d="m 103.33446,-5.3567852 -0.30079,-0.147207 V -40.140906 q 0,-0.831989 -0.2368,-1.177584 -0.2368,-0.345596 -0.7936,-0.441592 l -1.69606,-0.147195 q -0.56319,-0.134395 -0.822388,-0.412787 -0.259197,-0.278393 -0.259197,-0.751983 0,-0.505586 0.303994,-0.838379 0.303994,-0.332793 1.174381,-0.633586 l 5.96472,-2.182363 q 1.20317,-0.44159 1.93596,-0.646385 0.73278,-0.204795 1.41117,-0.204795 1.01117,0 1.52316,0.563186 0.51199,0.563185 0.51199,1.484767 v 40.4096918 q 0,0.9791836 0.2816,1.4495801 0.28161,0.4703965 0.86401,0.6559942 l 1.19043,0.32 q 0.6336,0.1855937 0.912,0.5375859 0.27839,0.3519922 0.27839,0.8191836 0,0.62718359 -0.41919,0.98237499 Q 114.73905,0 113.82387,0 h -6.6111 q -1.67677,0 -2.77754,-1.0687715 -1.10077,-1.0687715 -1.10077,-2.7775488 z M 80.000043,-14.316797 q 0,-5.113594 2.038395,-8.863984 2.038394,-3.750391 5.516781,-5.811184 3.478386,-2.060793 7.804765,-2.060793 3.833598,0 6.764806,1.79201 2.9312,1.79201 4.84479,5.196814 l -2.20792,1.830371 q -1.55521,-2.604832 -3.52325,-3.900857 -1.968043,-1.296025 -4.246488,-1.296025 -2.182442,0 -3.913678,1.292812 -1.731236,1.292813 -2.720058,3.97443 -0.988823,2.681617 -0.988823,6.86723 0,3.910418 0.969619,6.4640394 0.96962,2.5536211 2.640053,3.8112363 1.670434,1.2576153 3.814477,1.2576153 2.342433,0 4.358468,-1.3888125 2.01604,-1.3888125 3.72486,-4.2624258 l 1.41435,1.8367812 q -2.52159,3.9231719 -5.83358,6.1471524 -3.311997,2.22398045 -7.292774,2.22398045 -3.795199,0 -6.777598,-1.93919925 -2.982398,-1.9391992 -4.684797,-5.3375976 -1.702398,-3.3983985 -1.702398,-7.8335939 z" fill="#e9eef4" /> <path d="m 138.82884,-4.0831836 v -0.6591953 l -0.61437,-0.1280078 V -23.955086 q 0,-2.252863 -1.14243,-3.488094 -1.14243,-1.23523 -3.22249,-1.23523 -1.89446,0 -2.91529,0.803224 -1.02082,0.803225 -1.02082,1.980854 v 2.83516 q 0,1.900754 -1.27357,2.943928 -1.27357,1.043174 -3.6095,1.043174 -1.99674,0 -3.01432,-0.934383 -1.01758,-0.934383 -1.01758,-2.579149 0,-2.022394 1.66079,-3.951984 1.66078,-1.92959 4.89275,-3.203182 3.23196,-1.273591 7.96789,-1.273591 5.86875,0 8.74232,2.383994 2.87358,2.383994 2.87358,6.44158 v 16.6272811 q 0,0.9216172 0.368,1.3568203 0.368,0.4352031 1.04641,0.4352031 0.71041,0 1.05281,-0.3328027 0.3424,-0.3328027 0.6048,-0.6592051 0.18559,-0.2111992 0.36479,-0.3647988 0.1792,-0.1535996 0.4608,-0.1535996 0.41599,0 0.58879,0.2815937 0.17279,0.2815938 0.17279,0.7743868 0,1.0879804 -0.75198,2.239957 -0.75199,1.15197654 -2.24635,1.96475975 -1.49437,0.8127832 -3.69592,0.8127832 -2.87356,0 -4.57273,-1.30558789 -1.69917,-1.30558786 -1.69917,-3.57118946 z m -19.19358,-2.6304101 q 0,-3.9679763 3.57438,-6.4351723 3.57438,-2.467195 9.97433,-2.467195 2.09281,0 3.76643,0.352006 1.67362,0.352006 2.90244,0.934412 l -0.71678,2.054359 q -1.10723,-0.499207 -2.23046,-0.793615 -1.12323,-0.294408 -2.45447,-0.294408 -2.64327,0 -4.1377,1.369619 -1.49444,1.369619 -1.49444,3.776045 0,2.4256211 1.27363,3.6960332 1.27362,1.2704121 3.28325,1.2704121 1.72165,0 3.3121,-0.7904218 1.59044,-0.7904219 2.68489,-2.1856719 l 0.70399,1.8303594 q -1.77921,2.4768125 -4.73602,3.83361911 -2.9568,1.35680664 -6.2976,1.35680664 -4.16638,0 -6.78718,-2.07359175 -2.62079,-2.0735918 -2.62079,-5.4335957 z" fill="#e9eef4" /> <path d="m 167.24461,-28.851195 v 23.7312848 q 0,0.9791836 0.2816,1.4495801 0.28161,0.4703965 0.86401,0.6559942 l 1.19043,0.32 q 0.6336,0.1855937 0.92799,0.5375859 0.2944,0.3519922 0.2944,0.8191836 0,0.62718359 -0.43519,0.98237499 Q 169.93266,0 169.01748,0 H 156.4032 q -0.88957,0 -1.30877,-0.35519141 -0.41919,-0.3551914 -0.41919,-0.98237499 0,-0.4543906 0.288,-0.8063828 0.288,-0.3519922 0.88959,-0.537586 l 1.27363,-0.3328007 q 0.56961,-0.1983985 0.85122,-0.6559942 0.2816,-0.4575957 0.2816,-1.4367793 V -23.481699 q 0,-0.799988 -0.2528,-1.151985 -0.2528,-0.351996 -0.8096,-0.447992 l -1.66406,-0.134394 q -0.59519,-0.134395 -0.86079,-0.419188 -0.2656,-0.284793 -0.2656,-0.745582 0,-0.505586 0.3264,-0.844779 0.32639,-0.339194 1.16478,-0.627186 l 5.96472,-2.195164 q 1.24797,-0.447988 1.99356,-0.643183 0.74558,-0.195196 1.27677,-0.195196 1.05597,0 1.58396,0.563186 0.52799,0.563185 0.52799,1.471967 z m -5.20307,-6.918481 q -2.56634,0 -4.1151,-1.334379 -1.54877,-1.334379 -1.54877,-3.465535 0,-2.156758 1.55517,-3.459137 1.55516,-1.302378 4.1087,-1.302378 2.59834,0 4.14391,1.308779 1.54557,1.308779 1.54557,3.452736 0,2.131156 -1.54557,3.465535 -1.54557,1.334379 -4.14391,1.334379 z" fill="#e9eef4" /> <path d="m 185.75341,-16.947168 q 0,-4.646387 1.3248,-7.788785 1.32481,-3.142399 3.44961,-4.729602 2.1248,-1.587203 4.58879,-1.587203 3.07198,0 4.73598,1.743983 1.66399,1.743982 1.66399,4.96313 0,2.854321 -1.18717,4.275086 -1.18717,1.420766 -3.08152,1.420766 -1.91355,0 -2.91193,-1.027178 -0.99838,-1.027177 -0.99838,-2.851138 v -1.17759 q -0.0192,-0.96641 -0.47041,-1.478414 -0.45121,-0.512004 -1.46242,-0.512004 -1.12643,0 -2.16647,0.934422 -1.04003,0.934422 -1.69285,2.819259 -0.65282,1.884838 -0.65282,4.835288 z m 0.62082,-11.904027 0.51838,7.28323 v 16.4480548 q 0,0.8831875 0.3584,1.321586 0.3584,0.4383984 1.31199,0.5983945 l 2.75842,0.4287969 q 0.73599,0.1279961 1.10718,0.4639883 0.3712,0.3359922 0.3712,0.9503789 0,0.64638279 -0.44799,1.00157419 Q 191.90382,0 190.99504,0 H 176.0832 q -0.92157,0 -1.34077,-0.35519141 -0.41919,-0.3551914 -0.41919,-0.98237499 0,-0.4543906 0.288,-0.8063828 0.288,-0.3519922 0.87679,-0.537586 l 1.27363,-0.3328007 q 0.58241,-0.1855977 0.86402,-0.6495938 0.2816,-0.4639961 0.2816,-1.4559805 V -23.404902 q 0,-0.799989 -0.2528,-1.161584 -0.2528,-0.361596 -0.8096,-0.457592 l -1.67686,-0.115195 q -0.59519,-0.134395 -0.85439,-0.428788 -0.2592,-0.294392 -0.2592,-0.735982 0,-0.537586 0.336,-0.863978 0.33599,-0.326393 1.15518,-0.639987 l 5.81752,-2.124765 q 1.58716,-0.582383 2.31035,-0.767979 0.72318,-0.185596 1.15198,-0.185596 0.71679,0 1.06558,0.460789 0.34879,0.460789 0.48319,1.574364 z" fill="#e9eef4" /> <path d="m 233.67026,-18.911973 q 0,1.689571 -0.96638,2.617553 -0.96639,0.927982 -2.83517,0.927982 h -19.93606 v -2.45755 h 13.08828 q 1.62559,0 1.62559,-1.46559 0,-4.224008 -1.55203,-6.412834 -1.55204,-2.188826 -4.1057,-2.188826 -2.00326,0 -3.5393,1.203222 -1.53604,1.203223 -2.40326,3.475256 -0.86722,2.272033 -0.86722,5.452865 0,6.201622 2.90562,9.289645 2.90562,3.0880234 7.71843,3.0880234 2.82244,0 4.95687,-1.1200371 2.13442,-1.1200371 3.3312,-3.1872715 0.55037,-0.6527888 0.88637,-0.8671858 0.33599,-0.214396 0.70718,-0.214396 0.512,0 0.73599,0.45759 0.224,0.4575895 0.2112,1.0847731 -0.14081,2.7135898 -1.9232,5.0111777 -1.78239,2.2975879 -4.82237,3.65437893 -3.03998,1.35679102 -7.00154,1.35679102 -4.60798,0 -8.18876,-1.90399805 -3.58079,-1.903998 -5.62237,-5.3375957 -2.04159,-3.4335977 -2.04159,-8.047996 0,-4.838379 1.93919,-8.566371 1.9392,-3.727992 5.55517,-5.859192 3.61598,-2.131199 8.63995,-2.131199 4.28156,0 7.29274,1.548797 3.01118,1.548797 4.61118,4.291193 1.59999,2.742397 1.59999,6.300795 z" fill="#e9eef4" /> </g> </svg>`;

const navHeader = (brandHref, cta, active = "") => `
<header class="nav">
  <a class="brand brand--lockup" href="${esc(brandHref)}">${BRAND_LOCKUP}</a>
  <nav class="nav-links">${renderNav(active)}</nav>
  ${cta}
</header>`;

const footer = () => `
<footer class="foot">
  <div class="foot-brand">${esc(C.brand.name)}${eq(4)}</div>
  <p class="foot-note">${esc(C.footer.note)}</p>
  <nav class="foot-links">${C.footer.links
    .map((l) => `<a href="${esc(l.href)}" rel="noopener">${esc(l.label)}</a>`)
    .join("")}</nav>
</footer>`;

// Lightbox markup for a given image set. Thumbs elsewhere on the page open it via
// a data-lb index into this same array. Pages that have no zoomable image just
// omit it (and the LIGHTBOX_SCRIPT no-ops when #lightbox is absent).
const lightbox = (items) => {
  const slides = items
    .map(
      (s) => `
      <figure class="lb-slide${s.wide ? " wide" : ""}" data-cap="${esc(s.cap)}">
        <img src="${esc(s.src)}" alt="${esc(s.alt)}" loading="lazy" decoding="async" />
      </figure>`
    )
    .join("");
  const dots = items
    .map(
      (s, i) =>
        `<button class="lb-dot" type="button" data-i="${i}" aria-label="View “${esc(s.cap)}”"></button>`
    )
    .join("");
  return `
<div class="lightbox" id="lightbox" role="dialog" aria-modal="true" aria-label="Screenshots" hidden>
  <button class="lb-btn lb-close" type="button" aria-label="Close (Esc)">✕</button>
  <button class="lb-btn lb-nav lb-prev" type="button" aria-label="Previous">‹</button>
  <button class="lb-btn lb-nav lb-next" type="button" aria-label="Next">›</button>
  <div class="lb-track">${slides}</div>
  <p class="lb-cap" aria-live="polite"></p>
  <div class="lb-dots">${dots}</div>
</div>`;
};

// Lightbox behaviour — scroll-snap track, arrows, dots, keyboard. Shared verbatim
// by every page that calls lightbox(); guards on #lightbox so it's safe to include
// (or omit) anywhere.
const LIGHTBOX_SCRIPT = `
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
})();`;

// Theme carousel — scroll-snap track with arrows, dots, and gentle autoplay.
// Shared by ui.html; no-ops when there's no .tc-track on the page.
const THEME_CAROUSEL_SCRIPT = `
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
})();`;

// Theme carousel — one framed screenshot per built-in theme, in a scroll-snap row.
const themeSlides = C.themes.items
  .map(
    (t, i) => `
      <figure class="tc-slide" data-i="${i}">
        <div class="tc-frame"><img src="${esc(t.img)}" alt="ondaire in the ${esc(t.name)} theme" loading="lazy" decoding="async" /></div>
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

// The three prominent home blocks. Each is an image (or a faux-terminal mock for
// the install block) beside copy + a "learn more" link to its own page; they
// alternate sides via .flip.
const homeBlocks = C.home.blocks
  .map((b, i) => {
    const media = b.term
      ? `<div class="block-term" aria-hidden="true">
          <div class="term-bar"><span></span><span></span><span></span></div>
          <pre class="term-body"><code>${b.term
            .map((l) =>
              l.p
                ? `<span class="term-line"><span class="term-prompt">${esc(l.p)}</span> <span class="term-cmd">${esc(l.t)}</span></span>`
                : `<span class="term-line term-out">${esc(l.c)}</span>`
            )
            .join("")}</code></pre>
        </div>`
      : `<figure class="block-shot${b.phone ? " phone" : ""}">
          <img src="${esc(b.img)}" alt="${esc(b.alt)}" loading="lazy" decoding="async" />
        </figure>`;
    return `
      <article class="block${i % 2 ? " flip" : ""}">
        ${media}
        <div class="block-copy">
          <span class="kicker">${eq(5)}${esc(b.kicker)}</span>
          <h2>${esc(b.title)}</h2>
          <p>${esc(b.body)}</p>
          <a class="btn btn-ghost block-cta" href="${esc(b.cta.href)}">${esc(b.cta.label)}<span class="arrow">→</span></a>
        </div>
      </article>`;
  })
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

// ── home page (index.html) ──────────────────────────────────────────────
// Deliberately slim: hero → three prominent topic blocks (linking to install /
// ui / tech) → testimonials → the AI colophon → a closing CTA. The depth lives
// on the three sub-pages; this page is the overview.
const page = `<!doctype html>
<html lang="en">
<head>${head(C.meta.title, C.meta.description)}
</head>
<body>
<div class="grain" aria-hidden="true"></div>
${navHeader("#top", GET_IT_CTA)}
<main id="top">
  <section class="hero">
    <div class="hero-copy">
      <span class="eyebrow">${eq(6)}${esc(C.hero.eyebrow)}</span>
      <h1>${C.hero.title.map((t) => `<span>${esc(t)}</span>`).join("")}</h1>
      <p class="lede">${esc(C.hero.lede)}</p>
      <div class="actions">
        <a class="btn btn-solid" href="${esc(C.hero.primary.href)}" rel="noopener">${esc(C.hero.primary.label)}<span class="arrow">→</span></a>
        <a class="btn btn-ghost btn-gh" href="${esc(C.hero.secondary.href)}" rel="noopener">${GITHUB_ICON}${esc(C.hero.secondary.label)}</a>
      </div>
    </div>
    <figure class="hero-shot">
      <div class="frame"><img src="${esc(C.hero.shot.src)}" alt="${esc(C.hero.shot.alt)}" fetchpriority="high" /></div>
    </figure>
  </section>

  <section class="blocks">${homeBlocks}</section>

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
${footer()}
</body>
</html>
`;

// ── install methods page (install.html) ─────────────────────────────────
// Each method is a card: heading + body, an optional code block (copy:true makes
// it copyable), an optional note callout, and an optional CTA or doc link. The
// binaries + SHA-256s themselves live on download.html (the first method links there).
function installPage() {
  const I = C.install;
  const methods = I.methods
    .map((m) => {
      let code = "";
      if (m.code && m.copy) {
        const multi = m.code.includes("\n") ? " dl-cmd-multi" : "";
        code = `
        <div class="dl-cmd${multi}">
          <code>${esc(m.code)}</code>
          <button class="dl-copy" type="button" data-copy="${esc(m.code)}" aria-label="Copy command">copy</button>
        </div>`;
      } else if (m.code) {
        code = `\n        <pre class="qs-code"><code>${esc(m.code)}</code></pre>`;
      }
      const note = m.note ? `\n        <p class="dl-note">${m.note}</p>` : "";
      let foot = "";
      if (m.cta) {
        const rel = /^https?:/.test(m.cta.href) ? ' rel="noopener"' : "";
        foot = `\n        <div class="method-foot"><a class="btn btn-ghost" href="${esc(m.cta.href)}"${rel}>${esc(m.cta.label)}<span class="arrow">→</span></a></div>`;
      } else if (m.doc) {
        foot = `\n        <div class="method-foot"><a class="qs-doc" href="${esc(m.doc.href)}" rel="noopener">${esc(m.doc.label)}<span class="arrow">→</span></a></div>`;
      }
      return `
      <article class="method">
        <div class="method-top"><h3>${esc(m.title)}</h3><span class="tag">${esc(m.tag)}</span></div>
        <p class="method-body">${esc(m.body)}</p>${code}${note}${foot}
      </article>`;
    })
    .join("");
  const links = I.links
    .map((l) => {
      const rel = /^https?:/.test(l.href) ? ' rel="noopener"' : "";
      return `
      <div class="dl-link-row">
        <p class="dl-link-desc">${esc(l.desc)}</p>
        <a class="btn btn-ghost dl-link-btn" href="${esc(l.href)}"${rel}>${esc(l.label)}<span class="arrow">→</span></a>
      </div>`;
    })
    .join("");
  return `<!doctype html>
<html lang="en">
<head>${head(`Install — ${C.meta.title}`, "Install ondaire: just run the binary, a guided one-liner, Docker, a systemd boot service, or a DIY ESP32 player flashed from your browser.")}
</head>
<body>
<div class="grain" aria-hidden="true"></div>
${navHeader("index.html", GET_IT_CTA, "install.html")}
<main id="top">
  <section class="sub-page">
    <header class="sec-head">
      <span class="eyebrow">${eq(6)}${esc(I.eyebrow)}</span>
      <h1>${esc(I.title)}</h1>
      <p class="sec-intro">${esc(I.intro)}</p>
    </header>
    <div class="methods">${methods}</div>
    <div class="dl-links">${links}</div>
  </section>
</main>
${footer()}
</body>
</html>
`;
}

// ── Home Assistant integration page (home-assistant.html) ───────────────
// Screenshots (desktop + mobile), what it does, and the install steps with a
// restart callout. Linked from the download card and the install page.
function homeAssistantPage() {
  const H = C.homeAssistant;
  const lbImgs = H.shots.map((s) => ({ src: s.src, alt: s.alt, cap: s.cap, wide: s.wide }));
  const shots = H.shots
    .map(
      (s, i) => `
        <figure class="ha-shot">
          <img class="lb-thumb" data-lb="${i}" role="button" tabindex="0" aria-label="Enlarge “${esc(s.cap)}”" src="${esc(s.src)}" alt="${esc(s.alt)}" loading="lazy" decoding="async" />
          <figcaption>${esc(s.cap)}</figcaption>
        </figure>`,
    )
    .join("");
  const features = H.features.map((f) => `<li>${esc(f)}</li>`).join("");
  // steps + important are trusted author HTML (they carry <code>/<em>/<strong>).
  const steps = H.steps.map((s) => `<li>${s}</li>`).join("");
  return `<!doctype html>
<html lang="en">
<head>${head(`Home Assistant — ${C.meta.title}`, "Control every ondaire room from Home Assistant: each room is a media_player, plus a custom Lovelace card for players, media, streams and the queue.", "assets/img/home_assistant.png")}
</head>
<body>
<div class="grain" aria-hidden="true"></div>
${navHeader("index.html", GET_IT_CTA, "home-assistant.html")}
<main id="top">
  <section class="sub-page ha-page">
    <header class="sec-head">
      <span class="eyebrow">${eq(6)}${esc(H.eyebrow)}</span>
      <h1>${esc(H.title)}</h1>
      <p class="sec-intro">${esc(H.intro)}</p>
    </header>

    <div class="ha-shots">${shots}</div>

    <ul class="ha-features">${features}</ul>

    <div class="ha-install">
      <h2>Install</h2>
      <ol class="ha-steps">${steps}</ol>
      <p class="dl-note ha-important"><strong>Important —</strong> ${H.important}</p>
      <div class="ha-cta">
        <a class="btn btn-solid" href="${esc(H.download.href)}">${esc(H.download.label)}<span class="arrow">↓</span></a>
        <a class="btn btn-ghost" href="${esc(H.readme.href)}" rel="noopener">${esc(H.readme.label)}<span class="arrow">→</span></a>
      </div>
    </div>
  </section>
</main>
${footer()}
${lightbox(lbImgs)}
<script>${LIGHTBOX_SCRIPT}</script>
</body>
</html>
`;
}

// ── UI tour page (ui.html) ──────────────────────────────────────────────
// Reuses screens.items grouped by page (rooms / nodes) in the alternating
// screenshot layout, then the themes carousel. The lightbox holds the screens.
function uiPage() {
  const U = C.ui;
  const lbImgs = C.screens.items.map((s) => ({ src: s.src, alt: s.alt, cap: `${s.kicker} — ${s.title}` }));
  const screenCard = (s) => {
    const i = C.screens.items.indexOf(s);
    return `
      <article class="screen${i % 2 ? " flip" : ""}">
        <figure class="screen-shot">
          <img class="lb-thumb" data-lb="${i}" role="button" tabindex="0" aria-label="Open “${esc(s.title)}” full size" src="${esc(s.src)}" alt="${esc(s.alt)}" loading="lazy" decoding="async" />
        </figure>
        <div class="screen-copy">
          <span class="kicker">${eq(5)}${esc(s.kicker)}</span>
          <h3>${esc(s.title)}</h3>
          <p>${esc(s.body)}</p>
        </div>
      </article>`;
  };
  const group = (key) => C.screens.items.filter((s) => s.page === key).map(screenCard).join("");
  return `<!doctype html>
<html lang="en">
<head>${head(`The UI — ${C.meta.title}`, "A tour of the ondaire web app: grouping rooms, the media library and shared queue, multi-room Spotify, live per-node stats, and one-click themes.")}
</head>
<body>
<div class="grain" aria-hidden="true"></div>
${navHeader("index.html", GET_IT_CTA, "ui.html")}
<main id="top">
  <section class="screens">
    <header class="sec-head">
      <span class="eyebrow">${eq(6)}${esc(U.eyebrow)}</span>
      <h1>${esc(U.title)}</h1>
      <p class="sec-intro">${esc(U.intro)}</p>
    </header>
    <header class="sec-head sub-head">
      <span class="eyebrow">${esc(U.rooms.kicker)}</span>
      <h2>${esc(U.rooms.title)}</h2>
    </header>
    <div class="screen-list">${group("rooms")}</div>
    <header class="sec-head sub-head">
      <span class="eyebrow">${esc(U.nodes.kicker)}</span>
      <h2>${esc(U.nodes.title)}</h2>
    </header>
    <div class="screen-list">${group("nodes")}</div>
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
</main>
${footer()}
${lightbox(lbImgs)}
<script>${LIGHTBOX_SCRIPT}
${THEME_CAROUSEL_SCRIPT}</script>
</body>
</html>
`;
}

// ── under-the-hood page (tech.html) ─────────────────────────────────────
// The four sync problems + the measured-coherence graphs. The lightbox holds
// only the (wide) graphs, opened by the proof thumbs' 0-based data-lb.
function techPage() {
  // Both proof blocks (acoustic mic + clock telemetry) share one lightbox; the
  // thumbs' data-lb index into this combined, in-order list. All are wide graphs.
  const lbItems = [...C.proof.items, ...C.clocks.items].map((s) => ({
    src: s.src, alt: s.alt, cap: `${s.kicker} — ${s.title}`, wide: true,
  }));
  // One proof/measurement row; `base` offsets data-lb so the second block's
  // thumbs continue the lightbox index where the first left off.
  const proofRows = (items, base) =>
    items
      .map(
        (s, i) => `
      <article class="proof-item${i % 2 ? " flip" : ""}">
        <figure class="proof-shot">
          <img class="lb-thumb" data-lb="${base + i}" role="button" tabindex="0" aria-label="Open “${esc(s.title)}” full size" src="${esc(s.src)}" alt="${esc(s.alt)}" loading="lazy" decoding="async" />
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
  return `<!doctype html>
<html lang="en">
<head>${head(`How it works — ${C.meta.title}`, "How ondaire keeps rooms in sync: beating network jitter, packet loss, system-clock and DAC drift — with microphone and live-telemetry measurements that prove it.")}
</head>
<body>
<div class="grain" aria-hidden="true"></div>
${navHeader("index.html", GET_IT_CTA, "tech.html")}
<main id="top">
  <section class="tech-sec">
    <header class="sec-head">
      <span class="eyebrow">${eq(6)}${esc(C.tech.eyebrow)}</span>
      <h1>${esc(C.tech.title)}</h1>
      <p class="sec-intro">${esc(C.tech.intro)}</p>
    </header>
    <div class="tech-grid">${techItems}</div>
    <header class="sec-head tech-proof-head">
      <span class="eyebrow">${esc(C.proof.eyebrow)}</span>
      <h2>${esc(C.proof.title)}</h2>
      <p class="sec-intro">${esc(C.proof.intro)}</p>
    </header>
    <div class="proof-list">${proofRows(C.proof.items, 0)}</div>
    <header class="sec-head tech-proof-head">
      <span class="eyebrow">${esc(C.clocks.eyebrow)}</span>
      <h2>${esc(C.clocks.title)}</h2>
      <p class="sec-intro">${esc(C.clocks.intro)}</p>
    </header>
    <div class="proof-list">${proofRows(C.clocks.items, C.proof.items.length)}</div>
  </section>
</main>
${footer()}
${lightbox(lbItems)}
<script>${LIGHTBOX_SCRIPT}</script>
</body>
</html>
`;
}

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
  homeassistant: `<svg class="dl-logo" viewBox="0 0 24 24" width="22" height="22" role="img" aria-label="Home Assistant" fill="#18BCF2">
    <path d="M22.939 10.627 13.061.749a1.505 1.505 0 0 0-2.121 0l-9.879 9.878C.478 11.21 0 12.363 0 13.187v9c0 .826.675 1.5 1.5 1.5h9.227l-4.063-4.062a2.034 2.034 0 0 1-.664.113c-1.13 0-2.05-.92-2.05-2.05s.92-2.05 2.05-2.05 2.05.92 2.05 2.05c0 .233-.041.456-.113.665l3.163 3.163V9.928a2.05 2.05 0 0 1-1.15-1.84c0-1.13.92-2.05 2.05-2.05s2.05.92 2.05 2.05a2.05 2.05 0 0 1-1.15 1.84v8.127l3.146-3.146A2.051 2.051 0 0 1 18 12.239c1.13 0 2.05.92 2.05 2.05s-.92 2.05-2.05 2.05c-.25 0-.488-.047-.709-.13L12.9 20.602v3.088h9.6c.825 0 1.5-.675 1.5-1.5v-9c0-.825-.477-1.977-1.061-2.561z"/>
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
<meta name="description" content="Download ondaire — pure-Go binaries for 64-bit Raspberry Pi (arm64) and x86-64 Linux, plus the Docker image. SHA-256 for every build." />
<meta name="theme-color" content="${esc(C.meta.themeColor)}" />${FAVICONS}
<link rel="preload" href="assets/fonts/fraunces-wght.woff2" as="font" type="font/woff2" crossorigin />
<link rel="preload" href="assets/fonts/plex-sans-400.woff2" as="font" type="font/woff2" crossorigin />
<link rel="stylesheet" href="assets/styles.css" />
</head>
<body>
<div class="grain" aria-hidden="true"></div>

${navHeader("index.html", HOME_CTA)}

<main id="top">
  <section class="dl">
    <header class="sec-head">
      <span class="eyebrow">${eq(6)}${esc(C.download.eyebrow)}${VERSION ? " · " + esc(VERSION) : ""}</span>
      <h1>${esc(C.download.title)}</h1>
      <p class="sec-intro">${esc(C.download.intro)}</p>${
        C.download.note
          ? `\n      <p class="sec-intro"><strong>Tip:</strong> ${esc(C.download.note)}</p>`
          : ""
      }
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
// back to a local esp32 build (esp32/build-<id>/ondaire-fw-<id>.bin) so a bare
// `node build.mjs` after `esp32/build.sh <id>` produces a working flasher too.
// A missing image renders as "not built" — the page + manifests still build.
async function resolveFirmware() {
  const out = [];
  const find = async (candidates) => {
    for (const p of candidates) {
      try {
        return { src: p, buf: await fs.readFile(p) };
      } catch {
        // try the next location
      }
    }
    return null;
  };
  for (const b of C.firmware.builds) {
    const fname = b.file.split("/").pop();
    // Merged image (flash-all) …
    const merged = await find([
      path.join(SRC, b.file),
      path.join(root, "..", "esp32", `build-${b.id}`, fname),
    ]);
    // … and the app-only image (keep-config): staged by CI as ondaire-app-<id>.bin,
    // or straight out of a local build as ondaire-node.bin.
    const app = await find([
      path.join(SRC, "assets", "firmware", `ondaire-app-${b.id}.bin`),
      path.join(root, "..", "esp32", `build-${b.id}`, "ondaire-node.bin"),
    ]);
    if (merged) {
      out.push({
        ...b,
        present: true,
        src: merged.src,
        size: merged.buf.length,
        hash: createHash("sha256").update(merged.buf).digest("hex"),
        appSrc: app ? app.src : null,
      });
    } else {
      out.push({ ...b, present: false });
    }
  }
  return out;
}

// The app (ota_0) partition offset from esp32/partitions.csv — where the app
// image lands, and the lowest address a "keep config" flash may touch (NVS sits
// below it at 0x9000). Same for every board (they share partitions.csv).
const APP_OFFSET = 0x20000;

// Per-board ESP Web Tools manifests — one pair per board, matching the install
// step's two radio modes. `path` is relative to the manifest's own location, and
// new_install_prompt_erase is false in both (no in-dialog erase question; ESP Web
// Tools just writes the listed parts):
//   manifest-<id>.json       flash-all: the merged image at offset 0. It spans
//                            (and 0xFF-pads) the whole flash including NVS, so a
//                            write wipes stored config — the node reboots into its
//                            Wi-Fi captive portal. A clean, first-time install.
//   manifest-<id>-keep.json  keep-config: ONLY the app image at APP_OFFSET. NVS
//                            (0x9000) is below that and never written, so stored
//                            Wi-Fi/name/pins survive — a firmware-only update.
// Several boards can share a chipFamily, hence one pair per board.
function firmwareManifest(b, mode) {
  const part =
    mode === "keep"
      ? { path: `ondaire-app-${b.id}.bin`, offset: APP_OFFSET }
      : { path: b.file.split("/").pop(), offset: 0 };
  return {
    name: `${C.firmware.manifestName} — ${b.label} (${mode === "keep" ? "update" : "clean"})`,
    version: VERSION || "dev",
    new_install_prompt_erase: false,
    builds: [{ chipFamily: b.chipFamily, parts: [part] }],
  };
}

function flashPage(builds) {
  const F = C.flash;
  // A BOM item is either a plain string (escaped) or {html} for pre-marked-up
  // content like an external link.
  const bom = F.bom.items.map((i) => `<li>${typeof i === "string" ? esc(i) : i.html}</li>`).join("");

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
  // shows until one is picked. Cards are role=radio <div>s (not <button>s) so each
  // can nest its own zoom button — the picker JS wires click + Enter/Space to select.
  // Boards with a photo also get a lightbox slide; data-lb is that slide's index.
  // data-label feeds the search filter. Untested boards carry a corner badge.
  const lbBoards = builds.filter((b) => b.img);
  const boardLbImgs = lbBoards.map((b) => ({ src: b.img, alt: b.label, cap: b.label }));
  const boardCards = builds
    .map((b) => {
      const li = lbBoards.indexOf(b);
      const media = b.img
        ? `<img src="${esc(b.img)}" alt="${esc(b.label)}" loading="lazy" decoding="async" />
             <button type="button" class="fl-board-zoom" data-lb="${li}" aria-label="Zoom the ${esc(b.label)} photo">⤢</button>`
        : `<span class="fl-board-ph" aria-hidden="true">${esc(b.chipFamily || "ESP32")}</span>`;
      return `<div class="fl-board-card" role="radio" aria-checked="false" tabindex="0" data-id="${esc(b.id)}" data-label="${esc(b.label.toLowerCase())}" data-tested="${b.tested ? "1" : "0"}">
            ${b.tested ? "" : `<span class="fl-board-badge" title="Built from the vendor pin-map but not yet verified on real hardware">Untested</span>`}
            <span class="fl-board-imgwrap">${media}</span>
            <span class="fl-board-name">${esc(b.label)}</span>
          </div>`;
    })
    .join("");

  // Board metadata for the picker JS: image, both manifests (fresh / keep), the
  // wiring diagram, staged state, and the merged-image filename/size/sha rendered
  // once a board is selected.
  const boardData = builds.map((b) => ({
    id: b.id,
    label: b.label,
    note: b.note,
    tested: !!b.tested,
    img: b.img,
    doc: b.doc || "",
    manifest: `assets/firmware/manifest-${b.id}.json`,
    manifestKeep: `assets/firmware/manifest-${b.id}-keep.json`,
    present: !!b.present,
    keep: !!(b.present && b.appSrc),   // keep-config needs the app-only image staged
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
    notBuilt: "Firmware isn’t staged for this board — built by the CI firmware job, or run esp32/build.sh ",
    noSerial: "Web Serial needs Chrome or Edge on desktop.",
    flashDone: "Flashing finished — startup logs will appear here soon…",
    noLogs: "No logs yet — the board re-enumerates after flashing. Click “Connect over USB” to attach to it.",
  }).replace(/</g, "\\u003c");

  return `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8" />
<meta name="viewport" content="width=device-width, initial-scale=1" />
<title>Flash a player — ${esc(C.meta.title)}</title>
<meta name="description" content="Flash an ESP32 + I2S DAC ondaire player from your browser with ESP Web Tools — clean install or firmware-only update, then set Wi-Fi via the captive portal. No toolchain." />
<meta name="theme-color" content="${esc(C.meta.themeColor)}" />${FAVICONS}
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
  /* Search box + "show untested" toggle above the board row. */
  .fl-search-wrap{margin:0 0 14px;display:flex;align-items:center;gap:16px;flex-wrap:wrap}
  .fl-search{flex:1 1 240px;max-width:340px;padding:9px 12px;border:1px solid var(--line-2);border-radius:9px;background:var(--bg-2);color:inherit;font:inherit;font-size:14px}
  .fl-search:focus{outline:none;border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .fl-show-untested{display:inline-flex;align-items:center;gap:7px;color:var(--muted);font-size:14px;cursor:pointer;user-select:none}
  .fl-show-untested input{cursor:pointer;accent-color:var(--accent)}
  .fl-board-none{color:var(--muted);font-size:14px;margin:4px 2px 0}
  .fl-board-row{display:flex;gap:14px;overflow-x:auto;padding:6px 2px 14px;margin:0 -2px;scroll-snap-type:x proximity;-webkit-overflow-scrolling:touch}
  .fl-board-card{position:relative;flex:0 0 220px;scroll-snap-align:start;display:flex;flex-direction:column;gap:10px;align-items:center;text-align:center;background:var(--bg-2);border:1px solid var(--line-2);border-radius:12px;padding:14px;color:inherit;font:inherit;cursor:pointer;transition:border-color .15s,box-shadow .15s}
  .fl-board-card[hidden]{display:none}
  .fl-board-card:hover{border-color:color-mix(in srgb,var(--accent) 55%,var(--line-2))}
  .fl-board-card:focus-visible{outline:none;border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .fl-board-card.is-active{border-color:var(--accent);box-shadow:0 0 0 1px var(--accent),0 0 34px -16px var(--accent)}
  .fl-board-imgwrap{position:relative;width:100%;display:block}
  .fl-board-card img{width:100%;aspect-ratio:4/3;object-fit:contain;border-radius:8px;background:var(--bg)}
  /* Zoom affordance over the photo; visible on card hover/focus (always on touch). */
  .fl-board-zoom{position:absolute;right:6px;bottom:6px;width:30px;height:30px;display:grid;place-items:center;border:1px solid var(--line-2);border-radius:8px;background:color-mix(in srgb,var(--bg) 82%,transparent);color:inherit;font-size:15px;line-height:1;cursor:zoom-in;opacity:0;transition:opacity .15s,border-color .15s}
  .fl-board-card:hover .fl-board-zoom,.fl-board-card:focus-within .fl-board-zoom{opacity:1}
  .fl-board-zoom:hover{border-color:var(--accent)}
  @media (hover:none){.fl-board-zoom{opacity:1}}
  /* "Untested" corner badge on boards not yet verified on hardware. */
  .fl-board-badge{position:absolute;top:8px;left:8px;z-index:1;padding:2px 8px;border-radius:999px;font-size:11px;font-weight:600;letter-spacing:.02em;color:#7a3d10;background:#f6c67a;border:1px solid #e0a047}
  /* Placeholder tile for boards without a marketing photo. */
  .fl-board-ph{width:100%;aspect-ratio:4/3;border-radius:8px;display:grid;place-items:center;font-family:var(--mono);font-size:13px;letter-spacing:.04em;color:var(--faint);background:linear-gradient(135deg,color-mix(in srgb,var(--accent) 12%,var(--bg)),var(--bg))}
  .fl-board-name{font-size:14px;font-weight:600}

  /* Untested-board warning banner (shown under the meta when such a board is picked). */
  .fl-untested{margin:14px 0 2px;padding:12px 14px;border-radius:10px;font-size:13px;line-height:1.5;color:var(--fg);background:color-mix(in srgb,#f6c67a 16%,var(--bg));border:1px solid color-mix(in srgb,#e0a047 60%,var(--line-2))}
  .fl-untested[hidden]{display:none}
  .fl-untested strong{font-weight:600}

  /* selected-board meta line (staged sha / not built) */
  .fl-build{margin:4px 0 2px;padding:14px 0;border-top:1px solid var(--line)}
  .fl-build strong{font-weight:600}
  /* Headline (board name) bold, description stacked below it. */
  .fl-build-name{display:block;font-weight:600;font-size:15px}
  .fl-build-note{display:block;color:var(--muted);margin-top:2px}
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
  /* "or" divider between the DIY parts list and the all-in-one alternative. */
  .fl-bom-or{display:flex;align-items:center;gap:12px;margin:2px 0 10px;color:var(--faint);font-size:12px;text-transform:uppercase;letter-spacing:.14em}
  .fl-bom-or::before,.fl-bom-or::after{content:"";flex:1;height:1px;background:var(--line)}
  /* The Sonocotta alternative is a normal list bullet; only the link reads bold. */
  .fl-bom-alt-link{color:var(--accent);white-space:nowrap}
  .fl-ext{font-size:.85em}

  /* ── install step: flash-mode radio ───────────────────────────────── */
  .fl-modes{border:0;margin:16px 0;padding:0;display:flex;flex-direction:column;gap:10px}
  .fl-modes legend{padding:0;margin:0 0 2px;font-size:13px;font-weight:600;color:var(--faint);text-transform:uppercase;letter-spacing:.05em}
  .fl-mode{display:flex;gap:12px;align-items:flex-start;background:var(--bg-2);border:1px solid var(--line);border-radius:12px;padding:14px 16px;font-size:14px;line-height:1.55;cursor:pointer;transition:border-color .15s}
  .fl-mode:hover{border-color:color-mix(in srgb,var(--accent) 45%,var(--line))}
  .fl-mode:has(input:checked){border-color:var(--accent);box-shadow:0 0 0 1px var(--accent)}
  .fl-mode input{margin-top:3px;width:17px;height:17px;accent-color:var(--accent);flex:none}
  /* label as a headline, explanation stacked below it */
  .fl-mode-text{display:flex;flex-direction:column;gap:4px}
  .fl-mode-text strong{color:var(--ink);font-size:15px}
  .fl-mode-note{color:var(--muted)}
  .fl-mode.is-disabled{opacity:.5;cursor:not-allowed}

  /* warning/heads-up box (amber, semantic) */
  .fl-warn-box{display:flex;gap:12px;align-items:flex-start;background:rgba(224,166,74,.08);border:1px solid rgba(224,166,74,.4);border-radius:12px;padding:14px 16px;margin:6px 0 4px;color:#e7d3a8;font-size:13.5px;line-height:1.55}
  .fl-warn-box .fl-tag{flex:none;font-size:11px;font-weight:600;letter-spacing:.08em;text-transform:uppercase;color:var(--bg);background:#e0a64a;border-radius:999px;padding:3px 9px;margin-top:1px}
  .fl-warn{color:#e0a64a}

  /* serial console / boot-log terminal, revealed after a flash */
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
</style>
</head>
<body>
<div class="grain" aria-hidden="true"></div>

${navHeader("index.html", HOME_CTA)}

<main id="top">
  <section class="dl">
    <header class="sec-head">
      <span class="eyebrow">${eq(6)}${esc(F.eyebrow)}${VERSION ? " · " + esc(VERSION) : ""}</span>
      <h1>${esc(F.title)}</h1>
      <p class="sec-intro">${esc(F.intro)}</p>
    </header>

    <ol class="fl-steps" id="fl-steps">${stepChips}</ol>

    <div class="fl-wizard">
      <!-- Step 1 — select board -->
      <section class="fl-step" data-step="board">
        <h2>${esc(F.board.title)}</h2>
        <p class="fl-lead">${esc(F.board.body)}</p>
        <div class="fl-search-wrap">
          <input type="search" id="fl-search" class="fl-search" placeholder="Search boards…" aria-label="Search boards by name" autocomplete="off" spellcheck="false" />
          <label class="fl-show-untested"><input type="checkbox" id="fl-show-untested" /> Show untested boards</label>
        </div>
        <div class="fl-board-row" role="radiogroup" aria-label="${esc(F.board.title)}">
          ${boardCards}
        </div>
        <p class="fl-board-none" id="fl-board-none" hidden>No boards match your search.</p>
        <div class="fl-build" id="fl-build" hidden></div>
        <div class="fl-untested" id="fl-untested" role="note" hidden>
          <strong>Untested board.</strong> This firmware is built from the vendor's published pin map but hasn't been verified on real hardware yet. If audio or Wi-Fi misbehaves, every pin and the DAC type are re-provisionable over USB after flashing.
        </div>
        <details class="fl-bom">
          <summary>${esc(F.bom.title)}</summary>
          <ul>${bom}</ul>
          ${
            F.bom.alt
              ? `<div class="fl-bom-or"><span>or</span></div>
          <ul><li>${F.bom.alt.html}</li></ul>`
              : ""
          }
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
        <fieldset class="fl-modes">
          <legend>${esc(F.install.modes.title)}</legend>
          <label class="fl-mode">
            <input type="radio" name="fl-mode" value="all" checked />
            <span class="fl-mode-text"><strong>${esc(F.install.modes.all.label)}</strong><span class="fl-mode-note">${esc(F.install.modes.all.note)}</span></span>
          </label>
          <label class="fl-mode" id="fl-mode-keep-label">
            <input type="radio" name="fl-mode" value="keep" id="fl-mode-keep" />
            <span class="fl-mode-text"><strong>${esc(F.install.modes.keep.label)}</strong><span class="fl-mode-note">${esc(F.install.modes.keep.note)}</span></span>
          </label>
        </fieldset>
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
            <button class="btn btn-ghost fl-next" data-go="finished" id="fl-next-install" disabled>${esc(F.install.next)} <span class="arrow">→</span></button>
          </div>
        </div>
      </section>

      <!-- Step 3 — finished -->
      <section class="fl-step" data-step="finished" hidden>
        <div class="fl-done">
          <div class="fl-done-badge" aria-hidden="true">✓</div>
          <h2>${esc(F.finished.title)}</h2>
          <p class="fl-lead">${esc(F.finished.body)}</p>
          <div class="fl-warn-box" role="note"><span class="fl-tag">Heads up</span><span>${esc(F.finished.warning)}</span></div>
          <a id="fl-doc-link" class="btn btn-ghost fl-done-doc" href="${esc(F.docHref)}" rel="noopener">${esc(F.finished.docLink)} <span class="arrow">→</span></a>
        </div>
        <div class="fl-foot">
          <button class="btn btn-ghost btn-back fl-back" data-go="install" type="button"><span class="arrow">←</span> Back</button>
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

${boardLbImgs.length ? lightbox(boardLbImgs) : ""}

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
  var ORDER = ["board","install","finished"];
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
    // Headline (board name) in bold, its description stacked below it.
    var head = "<strong class='fl-build-name'>" + b.label + "</strong><span class='fl-build-note'>" + b.note + "</span>";
    if (b.present)
      return head + "<div class='fl-build-meta'><code>" + b.file + "</code> <span class='fl-ok'>staged</span> <span class='fl-muted'>" + b.size + "</span> <code class='fl-sha'>" + b.hash + "</code></div>";
    return head + "<div class='fl-build-meta'><code>" + b.file + "</code> <span class='fl-no'>not built</span> <span class='fl-muted'>— " + MSG.notBuilt + b.id + ".</span></div>";
  }
  function flashMode(){ var r = document.querySelector('input[name="fl-mode"]:checked'); return r ? r.value : "all"; }
  // Point ESP Web Tools at the flash-all (merged) or keep-config (app-only) manifest.
  function applyManifest(){
    if (!selected || !selected.present) return;
    install.setAttribute("manifest", flashMode() === "keep" ? selected.manifestKeep : selected.manifest);
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
    var un = $("fl-untested"); if (un) un.hidden = !!b.tested;   // warn on unverified boards
    install.style.display = b.present ? "" : "none";
    // Keep-config needs the app-only image staged; if it isn't, disable that mode
    // and fall back to flash-all so the manifest is always valid.
    var keepInput = $("fl-mode-keep"), keepLabel = $("fl-mode-keep-label");
    if (keepInput){
      keepInput.disabled = !b.keep;
      if (keepLabel) keepLabel.classList.toggle("is-disabled", !b.keep);
      if (!b.keep && keepInput.checked){ var all = document.querySelector('input[name="fl-mode"][value="all"]'); if (all) all.checked = true; }
    }
    applyManifest();
    // Re-arm the install gate whenever the board changes.
    flashStatus(null);
    $("fl-next-install").disabled = true;
    $("fl-next-board").disabled = !b.present;   // can't flash a board with no image
  }
  cards.forEach(function(c){
    c.addEventListener("click", function(){ pick(c.getAttribute("data-id")); });
    c.addEventListener("keydown", function(e){
      if (e.key === "Enter" || e.key === " ") { e.preventDefault(); pick(c.getAttribute("data-id")); }
    });
  });
  // Zoom buttons open the photo lightbox (LIGHTBOX_SCRIPT wires the data-lb attr);
  // stop the click bubbling to the card so zooming doesn't also select the board.
  [].forEach.call(document.querySelectorAll(".fl-board-zoom"), function(z){
    z.addEventListener("click", function(e){ e.stopPropagation(); });
  });
  // Board filter: by search text AND the "show untested" toggle. Untested boards
  // are hidden by default (opt-in via the checkbox); a hint shows when nothing
  // matches. If the currently-selected board gets filtered out, reset the picker.
  var search = $("fl-search"), noneMsg = $("fl-board-none"), showUntested = $("fl-show-untested");
  function applyFilter(){
    var q = search ? search.value.trim().toLowerCase() : "";
    var showUn = showUntested ? showUntested.checked : true;
    var shown = 0;
    cards.forEach(function(c){
      var matchQ = !q || c.getAttribute("data-label").indexOf(q) !== -1;
      var vis = matchQ && (c.getAttribute("data-tested") === "1" || showUn);
      c.hidden = !vis;
      if (vis) shown++;
    });
    if (noneMsg) noneMsg.hidden = shown !== 0;
    if (selected){
      var sc = document.querySelector('.fl-board-card[data-id="' + selected.id + '"]');
      if (sc && sc.hidden){
        selected = null;
        sc.classList.remove("is-active"); sc.setAttribute("aria-checked", "false");
        var m1 = $("fl-build"); if (m1) m1.hidden = true;
        var un = $("fl-untested"); if (un) un.hidden = true;
        if (install) install.style.display = "none";
        var nb = $("fl-next-board"); if (nb) nb.disabled = true;
      }
    }
  }
  if (search) search.addEventListener("input", applyFilter);
  if (showUntested) showUntested.addEventListener("change", applyFilter);
  applyFilter();   // default: hide untested until the box is checked
  [].forEach.call(document.querySelectorAll('input[name="fl-mode"]'), function(r){ r.addEventListener("change", applyManifest); });

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
      $("fl-next-install").disabled = false;   // unlock "Finish →"
      // esptool hard-resets into the app after flashing, so the board is already
      // booting — try to reattach, but always offer an explicit Connect.
      attachConsole("fl-log-install", "fl-connect-install", MSG.flashDone);
    }
  }
  // ESP Web Tools renders its own dialog. For a non-Improv device its first screen
  // is an "Install / Logs & Console" menu — redundant here (we have our own
  // console), so auto-pick Install to land straight on its Confirm screen. Guarded
  // by sawInstall + autoTried so it never re-fires when DASHBOARD reappears after a
  // flash. Best-effort: if the shadow DOM ever changes, the menu just stays.
  function clickInstall(dlg, tries){
    try {
      var root = dlg.shadowRoot;
      var items = root ? root.querySelectorAll("ew-list-item") : [];
      for (var i = 0; i < items.length; i++){
        if (/install/i.test(items[i].textContent || "")){ items[i].click(); return; }
      }
    } catch(e){ return; }
    if ((tries || 0) < 10) setTimeout(function(){ clickInstall(dlg, (tries || 0) + 1); }, 60);
  }
  function watchDialog(dlg){
    sawInstall = false; sawError = false; flashStatus(null);
    var autoTried = false;
    function read(){
      var s = dlg.getAttribute("state");
      if (s === "INSTALL") sawInstall = true;
      else if (s === "ERROR") sawError = true;
      else if (s === "DASHBOARD" && !sawInstall && !autoTried){ autoTried = true; clickInstall(dlg, 0); }
    }
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

  // ── boot-log console (read-only) ──────────────────────────────────────
  // After flashing we attach to the board's USB-JTAG and stream everything it
  // prints (bootloader + ESP_LOG lines) into a terminal box, so you can watch it
  // boot — and, on a full flash, see the captive-portal AP come up. Display only;
  // provisioning is the captive portal / USB JSON console, not this page.
  var port = null, logEl = null, wantConsole = false, gotData = false;
  function appendLog(s){ if (!logEl) return; var t = logEl.textContent + s; if (t.length > 12000) t = t.slice(-12000); logEl.textContent = t; logEl.scrollTop = logEl.scrollHeight; }
  function showConsole(id){ logEl = $(id); var box = logEl && logEl.closest(".fl-console"); if (box) box.hidden = false; }
  async function readLoop(){
    var dec = new TextDecoderStream();
    port.readable.pipeTo(dec.writable).catch(function(){});
    var r = dec.readable.getReader();
    for(;;){
      var out; try { out = await r.read(); } catch(e){ break; }
      if (out.done) break;
      gotData = true;
      appendLog(out.value);
    }
    port = null;   // closed — e.g. a reboot re-enumerated the USB-JTAG
  }
  function sleep(ms){ return new Promise(function(r){ setTimeout(r, ms); }); }
  // Open p, retrying the transient "already open" — ESP Web Tools can still hold
  // the port for a moment after its dialog closes, so we wait for it to release
  // (implicit close-and-reopen). port is set only on success, so a failed open
  // never poisons a later connect().
  async function openPort(p){
    for (var i = 0; ; i++){
      try { await p.open({ baudRate: 115200 }); break; }
      catch(e){
        if (i >= 9 || !/already open/i.test(String(e && e.message))) throw e;
        await sleep(400);
      }
    }
    port = p; readLoop();
  }
  async function dropPort(){
    try { if (port) await port.close(); } catch(e){}
    port = null;
  }
  // esp_restart() drops the native USB-JTAG and it re-enumerates as a fresh port.
  if (navigator.serial){
    navigator.serial.addEventListener("connect", function(ev){
      if (wantConsole && !port){ appendLog("\\n[device reconnected]\\n"); openPort(ev.target).catch(function(){}); }
    });
    // Clear the stale handle when the device drops, so the next connect() opens a
    // fresh port instead of writing into a dead one (the old "no response" trap).
    navigator.serial.addEventListener("disconnect", function(){
      if (port){ appendLog("\\n[device disconnected]\\n"); port = null; }
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
  // Reveal a step's console after flashing and attach. The auto-attempt reuses a
  // port the page was already granted (no picker) — but esptool's hard-reset
  // re-enumerates the USB-JTAG, so that handle is often stale and yields nothing.
  // Hence the Connect button is ALWAYS offered, and a watchdog nudges toward it if
  // no bytes arrive (the old code hid the button whenever a stale port "opened").
  async function attachConsole(logId, connectBtnId, intro){
    showConsole(logId); wantConsole = true; gotData = false;
    if (intro) appendLog(intro + "\\n");
    if (connectBtnId) $(connectBtnId).hidden = false;
    try { await connect(false); } catch(e){ appendLog(e.message + "\\n"); }
    // Only nudge toward the button if we never actually attached.
    setTimeout(function(){ if (!port && !gotData) appendLog(MSG.noLogs + "\\n"); }, 6000);
  }
  // The Connect button always forces a fresh port: drop any stale handle, then
  // prompt, so the user can attach to the re-enumerated device after a reset.
  var cb = $("fl-connect-install");
  if (cb) cb.addEventListener("click", async function(){
    wantConsole = true; gotData = false;
    await dropPort();
    try { var ok = await connect(true); appendLog(ok ? "[connected]\\n" : "[no port selected]\\n"); }
    catch(e){ appendLog(e.message + "\\n"); }
  });
})();
${LIGHTBOX_SCRIPT}
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

  // The three home blocks each link to a dedicated topic page.
  await fs.writeFile(path.join(OUT, "install.html"), installPage());
  await fs.writeFile(path.join(OUT, "home-assistant.html"), homeAssistantPage());
  await fs.writeFile(path.join(OUT, "ui.html"), uiPage());
  await fs.writeFile(path.join(OUT, "tech.html"), techPage());

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
    // Flash-all (merged @ 0) + keep-config (app @ APP_OFFSET) manifest per board.
    await fs.writeFile(
      path.join(fwOut, `manifest-${b.id}.json`),
      JSON.stringify(firmwareManifest(b, "all"), null, 2)
    );
    await fs.writeFile(
      path.join(fwOut, `manifest-${b.id}-keep.json`),
      JSON.stringify(firmwareManifest(b, "keep"), null, 2)
    );
    if (b.present) {
      await fs.copyFile(b.src, path.join(fwOut, b.file.split("/").pop()));
      // App-only image for keep-config (absent → the wizard disables that mode).
      if (b.appSrc) await fs.copyFile(b.appSrc, path.join(fwOut, `ondaire-app-${b.id}.bin`));
    }
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
