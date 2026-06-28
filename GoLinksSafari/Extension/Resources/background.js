// Go Links — background service worker.
//
// Safari has no `omnibox` API (unlike Chrome), so we cannot register a keyword
// that lights up the address bar directly. Instead we use declarativeNetRequest
// to redirect, before the request ever leaves the device, in two situations:
//
//   1. "go <thing>"  — the user's DEFAULT search engine is asked to search for
//                      "go something". We match that search URL and rewrite it
//                      to the go-links server.
//   2. "go/<thing>"  — Safari treats this as the URL http://go/<thing> (single
//                      label host). We rewrite http(s)://go/* to the server.
//
// All rules are built dynamically from user settings (server URL, keyword, which
// engines are enabled), so everything is configurable from the options page.

const DEFAULTS = {
  // Base URL of the go-links server. No trailing slash.
  serverBase: "https://go.bragi0.com",
  // The trigger word typed before the link name.
  keyword: "go",
  // Which default-search-engine queries to intercept for the "go <thing>" form.
  engines: {
    google: true,
    duckduckgo: true,
    bing: true,
    yahoo: true,
    ecosia: true,
    brave: true,
    startpage: true,
  },
  // Also redirect the bullet-proof "go/<thing>" slash form (http://go/*).
  slashRedirect: true,
};

// Search engines we know how to intercept. Each builds a regexFilter that:
//   - matches the engine's results URL,
//   - requires the query parameter to START WITH "<keyword>" followed by a
//     separator (+ or %20),
//   - captures everything after that separator up to the next & as group 1.
// The captured group is handed verbatim to the go-links server, which already
// knows how to split "name+arg1+arg2" / "name arg1 arg2" into a link + args.
const ENGINES = {
  google: {
    label: "Google",
    filter: (kw) =>
      `^https?://(?:[a-z0-9-]+\\.)*google\\.[a-z.]+/search\\?(?:.*&)?q=${kw}(?:\\+|%20)(.+?)(?:&.*)?$`,
  },
  duckduckgo: {
    label: "DuckDuckGo",
    filter: (kw) =>
      `^https?://(?:[a-z0-9-]+\\.)*(?:duckduckgo\\.com|duck\\.com)/(?:\\?|[^?]*\\?)(?:.*&)?q=${kw}(?:\\+|%20)(.+?)(?:&.*)?$`,
  },
  bing: {
    label: "Bing",
    filter: (kw) =>
      `^https?://(?:www\\.)?bing\\.com/search\\?(?:.*&)?q=${kw}(?:\\+|%20)(.+?)(?:&.*)?$`,
  },
  yahoo: {
    label: "Yahoo",
    filter: (kw) =>
      `^https?://(?:[a-z0-9-]+\\.)*search\\.yahoo\\.com/search\\?(?:.*&)?p=${kw}(?:\\+|%20)(.+?)(?:&.*)?$`,
  },
  ecosia: {
    label: "Ecosia",
    filter: (kw) =>
      `^https?://(?:www\\.)?ecosia\\.org/search\\?(?:.*&)?q=${kw}(?:\\+|%20)(.+?)(?:&.*)?$`,
  },
  brave: {
    label: "Brave",
    filter: (kw) =>
      `^https?://search\\.brave\\.com/search\\?(?:.*&)?q=${kw}(?:\\+|%20)(.+?)(?:&.*)?$`,
  },
  startpage: {
    label: "Startpage",
    filter: (kw) =>
      `^https?://(?:www\\.)?startpage\\.com/[^?]*\\?(?:.*&)?query=${kw}(?:\\+|%20)(.+?)(?:&.*)?$`,
  },
};

// Escape a user-supplied keyword for safe embedding inside a regex.
function escapeRegex(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, "\\$&");
}

// Strip a trailing slash from the server base.
function normalizeBase(base) {
  return (base || "").trim().replace(/\/+$/, "");
}

async function getConfig() {
  const stored = await browser.storage.local.get("config");
  return { ...DEFAULTS, ...(stored.config || {}),
           engines: { ...DEFAULTS.engines, ...((stored.config || {}).engines || {}) } };
}

// Build the full dynamic rule set from the current config.
function buildRules(cfg) {
  const base = normalizeBase(cfg.serverBase) || normalizeBase(DEFAULTS.serverBase);
  const kw = escapeRegex((cfg.keyword || DEFAULTS.keyword).trim());
  const rules = [];
  let id = 1;

  // (1) "go <thing>" via each enabled search engine.
  for (const [key, engine] of Object.entries(ENGINES)) {
    if (cfg.engines && cfg.engines[key] === false) continue;
    rules.push({
      id: id++,
      priority: 1,
      action: {
        type: "redirect",
        redirect: { regexSubstitution: `${base}/\\1` },
      },
      condition: {
        regexFilter: engine.filter(kw),
        resourceTypes: ["main_frame"],
      },
    });
  }

  // (2) "go/<thing>" slash form: http(s)://go/<rest> -> server/<rest>.
  if (cfg.slashRedirect) {
    rules.push({
      id: id++,
      priority: 1,
      action: { type: "redirect", redirect: { regexSubstitution: `${base}/\\1` } },
      condition: {
        regexFilter: `^https?://${kw}/(.*)$`,
        resourceTypes: ["main_frame"],
      },
    });
    // Bare "go/" or "go" -> server home.
    rules.push({
      id: id++,
      priority: 1,
      action: { type: "redirect", redirect: { regexSubstitution: `${base}/` } },
      condition: {
        regexFilter: `^https?://${kw}/?$`,
        resourceTypes: ["main_frame"],
      },
    });
  }

  return rules;
}

// Replace every dynamic rule with a freshly built set.
async function applyRules() {
  const cfg = await getConfig();
  const rules = buildRules(cfg);
  const existing = await browser.declarativeNetRequest.getDynamicRules();
  await browser.declarativeNetRequest.updateDynamicRules({
    removeRuleIds: existing.map((r) => r.id),
    addRules: rules,
  });
  return rules.length;
}

// First run: persist defaults so the options page has something to show.
async function ensureConfig() {
  const stored = await browser.storage.local.get("config");
  if (!stored.config) {
    await browser.storage.local.set({ config: DEFAULTS });
  }
}

browser.runtime.onInstalled.addListener(async () => {
  await ensureConfig();
  await applyRules();
});

browser.runtime.onStartup?.addListener(async () => {
  await applyRules();
});

// Rebuild whenever settings change.
browser.storage.onChanged.addListener((changes, area) => {
  if (area === "local" && changes.config) {
    applyRules();
  }
});

// Let the popup / options page query and force-refresh.
browser.runtime.onMessage.addListener((msg, _sender, sendResponse) => {
  (async () => {
    if (msg && msg.type === "getConfig") {
      sendResponse({ config: await getConfig(), defaults: DEFAULTS, engines:
        Object.fromEntries(Object.entries(ENGINES).map(([k, v]) => [k, v.label])) });
    } else if (msg && msg.type === "rebuild") {
      const n = await applyRules();
      sendResponse({ ok: true, ruleCount: n });
    }
  })();
  return true; // async response
});

// Make sure rules exist even on a plain worker wake-up.
ensureConfig().then(applyRules);
