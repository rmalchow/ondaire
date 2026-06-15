import { mount } from "svelte";
import App from "./App.svelte";

// Self-hosted theme fonts (bundled by Vite — no CDN). Family names match the CSS
// stacks in app.css. Fraunces (the fixed wordmark) is @font-face'd in app.css.
import "@fontsource/bricolage-grotesque/400.css";
import "@fontsource/bricolage-grotesque/600.css";
import "@fontsource/bricolage-grotesque/700.css";
import "@fontsource/hanken-grotesk/400.css";
import "@fontsource/hanken-grotesk/500.css";
import "@fontsource/hanken-grotesk/600.css";
import "@fontsource/hanken-grotesk/700.css";
import "@fontsource/jetbrains-mono/400.css";
import "@fontsource/jetbrains-mono/500.css";
import "@fontsource/jetbrains-mono/600.css";
import "@fontsource/syne/600.css";
import "@fontsource/syne/700.css";
import "@fontsource/syne/800.css";
import "@fontsource/press-start-2p/400.css"; // 8bit theme
import "@fontsource/vt323/400.css"; // 8bit body/mono
import "./app.css";

const app = mount(App, { target: document.getElementById("app") });

export default app;
