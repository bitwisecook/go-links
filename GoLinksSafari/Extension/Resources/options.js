// Options page: edit config, persist to storage.local; background rebuilds rules.

const els = {
  serverBase: document.getElementById("serverBase"),
  keyword: document.getElementById("keyword"),
  slashRedirect: document.getElementById("slashRedirect"),
  engines: document.getElementById("engines"),
  status: document.getElementById("status"),
};

let defaults = null;
let engineLabels = {};

function buildEngineToggles(enabled) {
  els.engines.innerHTML = "";
  for (const [key, label] of Object.entries(engineLabels)) {
    const wrap = document.createElement("label");
    const cb = document.createElement("input");
    cb.type = "checkbox";
    cb.dataset.engine = key;
    cb.checked = enabled[key] !== false;
    const span = document.createElement("span");
    span.textContent = label;
    wrap.append(cb, span);
    els.engines.append(wrap);
  }
}

function fill(cfg) {
  els.serverBase.value = cfg.serverBase || "";
  els.keyword.value = cfg.keyword || "";
  els.slashRedirect.checked = cfg.slashRedirect !== false;
  buildEngineToggles(cfg.engines || {});
}

async function load() {
  const res = await browser.runtime.sendMessage({ type: "getConfig" });
  defaults = res.defaults;
  engineLabels = res.engines;
  fill(res.config);
}

function collect() {
  const engines = {};
  els.engines.querySelectorAll("input[data-engine]").forEach((cb) => {
    engines[cb.dataset.engine] = cb.checked;
  });
  let serverBase = els.serverBase.value.trim();
  if (serverBase && !/^https?:\/\//i.test(serverBase)) serverBase = "https://" + serverBase;
  return {
    serverBase: serverBase || defaults.serverBase,
    keyword: (els.keyword.value.trim() || defaults.keyword).toLowerCase(),
    slashRedirect: els.slashRedirect.checked,
    engines,
  };
}

function flash(text) {
  els.status.textContent = text;
  setTimeout(() => (els.status.textContent = ""), 1800);
}

document.getElementById("save").addEventListener("click", async () => {
  const config = collect();
  await browser.storage.local.set({ config });
  const res = await browser.runtime.sendMessage({ type: "rebuild" });
  fill(config);
  flash(`Saved · ${res?.ruleCount ?? 0} rules active`);
});

document.getElementById("reset").addEventListener("click", async () => {
  await browser.storage.local.set({ config: defaults });
  fill(defaults);
  await browser.runtime.sendMessage({ type: "rebuild" });
  flash("Reset to defaults");
});

load();
