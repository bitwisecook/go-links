// Go Links — background.
//
// Safari has no `omnibox` API, and its declarativeNetRequest `regexSubstitution`
// redirects are unreliable. So instead we watch top-level navigations with
// webNavigation.onBeforeNavigate and redirect with tabs.update — both of which
// Safari supports well. Two cases are handled, before the page loads:
//
//   1. "go <thing>"  — the default search engine is asked to search "go thing";
//                      we catch that and jump to the go-links server.
//   2. "go/<thing>"  — Safari resolves this as http://go/<thing>; we rewrite it.
//
// Everything is driven by user settings (server URL, keyword, engines).

const DEFAULTS = {
  serverBase: "https://go.bragi0.com",
  keyword: "go",
  engines: {
    google: true,
    duckduckgo: true,
    bing: true,
    yahoo: true,
    ecosia: true,
    brave: true,
    startpage: true,
  },
  slashRedirect: true,
};

// Each engine: does this URL look like that engine's results page, and which
// query parameter holds the typed text.
const ENGINES = {
  google: { label: "Google", param: "q",
    match: (u) => /(^|\.)google\.[a-z.]+$/.test(u.hostname) && u.pathname.startsWith("/search") },
  duckduckgo: { label: "DuckDuckGo", param: "q",
    match: (u) => /(^|\.)(duckduckgo\.com|duck\.com)$/.test(u.hostname) },
  bing: { label: "Bing", param: "q",
    match: (u) => /(^|\.)bing\.com$/.test(u.hostname) && u.pathname.startsWith("/search") },
  yahoo: { label: "Yahoo", param: "p",
    match: (u) => /(^|\.)search\.yahoo\.com$/.test(u.hostname) && u.pathname.startsWith("/search") },
  ecosia: { label: "Ecosia", param: "q",
    match: (u) => /(^|\.)ecosia\.org$/.test(u.hostname) && u.pathname.startsWith("/search") },
  brave: { label: "Brave", param: "q",
    match: (u) => u.hostname === "search.brave.com" && u.pathname.startsWith("/search") },
  startpage: { label: "Startpage", param: "query",
    match: (u) => /(^|\.)startpage\.com$/.test(u.hostname) },
};

let cfg = null;

function normalizeBase(b) { return (b || "").trim().replace(/\/+$/, ""); }

async function loadConfig() {
  const stored = await browser.storage.local.get("config");
  const c = stored.config || {};
  cfg = { ...DEFAULTS, ...c, engines: { ...DEFAULTS.engines, ...(c.engines || {}) } };
  return cfg;
}

// "keyword arg1 arg2" remainder -> server path "arg1+arg2" (server splits on +).
function toServerPath(rest) {
  return rest.trim().split(/\s+/).map(encodeURIComponent).join("+");
}

// Resolve a navigation URL to a go-links target, or null if it isn't one of ours.
function resolveTarget(rawUrl) {
  if (!cfg) return null;
  const base = normalizeBase(cfg.serverBase) || normalizeBase(DEFAULTS.serverBase);
  const kw = (cfg.keyword || DEFAULTS.keyword).trim().toLowerCase();

  let u;
  try { u = new URL(rawUrl); } catch { return null; }

  // Never touch navigations that already point at the server.
  let serverHost = "";
  try { serverHost = new URL(base).host; } catch {}
  if (serverHost && u.host === serverHost) return null;

  // (2) "go/thing" — single-label host equal to the keyword.
  if (cfg.slashRedirect && u.hostname.toLowerCase() === kw) {
    return base + u.pathname + u.search;
  }

  // (1) "go thing" — the default search engine's query starts with the keyword.
  for (const [key, eng] of Object.entries(ENGINES)) {
    if (cfg.engines && cfg.engines[key] === false) continue;
    if (!eng.match(u)) continue;
    const q = u.searchParams.get(eng.param);
    if (!q) return null;
    const parts = q.trim().split(/\s+/);
    if (parts.length >= 2 && parts[0].toLowerCase() === kw) {
      return base + "/" + toServerPath(parts.slice(1).join(" "));
    }
    return null; // engine matched but not a "go …" query
  }
  return null;
}

async function onBeforeNavigate(details) {
  if (details.frameId !== 0) return;      // top-level frame only
  if (!cfg) await loadConfig();
  const target = resolveTarget(details.url);
  if (!target) return;
  try {
    await browser.tabs.update(details.tabId, { url: target });
  } catch (e) {
    // tab may have closed; ignore
  }
}

browser.webNavigation.onBeforeNavigate.addListener(onBeforeNavigate);

browser.storage.onChanged.addListener((changes, area) => {
  if (area === "local" && changes.config) loadConfig();
});

browser.runtime.onInstalled.addListener(async () => {
  const stored = await browser.storage.local.get("config");
  if (!stored.config) await browser.storage.local.set({ config: DEFAULTS });
  await loadConfig();
});

browser.runtime.onStartup?.addListener(loadConfig);

// Config + engine labels for the popup / options page.
browser.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  (async () => {
    if (msg && msg.type === "getConfig") {
      sendResponse({
        config: await loadConfig(),
        defaults: DEFAULTS,
        engines: Object.fromEntries(Object.entries(ENGINES).map(([k, v]) => [k, v.label])),
      });
    } else if (msg && msg.type === "rebuild") {
      await loadConfig();
      sendResponse({ ok: true });
    }
  })();
  return true;
});

loadConfig();
