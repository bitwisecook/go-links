// Popup: quick launcher with live suggestions from the go-links server.

const qEl = document.getElementById("q");
const listEl = document.getElementById("suggestions");
const serverEl = document.getElementById("server");
const formEl = document.getElementById("form");

let base = "https://go.bragi0.com";
let active = -1;           // highlighted suggestion index
let items = [];            // [{name, desc, url}]
let seq = 0;               // guards against out-of-order fetches

function normalizeBase(b) { return (b || "").trim().replace(/\/+$/, ""); }

async function init() {
  const res = await browser.runtime.sendMessage({ type: "getConfig" });
  base = normalizeBase(res.config.serverBase) || base;
  try { serverEl.textContent = new URL(base).host; } catch { serverEl.textContent = base; }
}

// Navigate the active tab to a go-links URL and close the popup.
async function go(path) {
  const url = `${base}/${path}`;
  const [tab] = await browser.tabs.query({ active: true, currentWindow: true });
  if (tab) {
    await browser.tabs.update(tab.id, { url });
  } else {
    await browser.tabs.create({ url });
  }
  window.close();
}

// Turn the raw query into a server path: "name arg1 arg2" -> "name+arg1+arg2".
function queryToPath(q) {
  return q.trim().split(/\s+/).map(encodeURIComponent).join("+");
}

function highlight(text, query) {
  const i = text.toLowerCase().indexOf(query.toLowerCase());
  if (i < 0 || !query) return document.createTextNode(text);
  const frag = document.createDocumentFragment();
  frag.append(text.slice(0, i));
  const b = document.createElement("b");
  b.textContent = text.slice(i, i + query.length);
  frag.append(b, text.slice(i + query.length));
  return frag;
}

function render(query) {
  listEl.innerHTML = "";
  active = -1;
  items.forEach((it, idx) => {
    const li = document.createElement("li");
    li.setAttribute("role", "option");
    const name = document.createElement("div");
    name.className = "name";
    name.append(highlight(it.name, query.split(/\s+/)[0] || ""));
    const desc = document.createElement("div");
    desc.className = "desc";
    desc.textContent = it.desc || it.url || "";
    li.append(name, desc);
    li.addEventListener("click", () => go(queryToPath(it.name)));
    li.addEventListener("mouseenter", () => setActive(idx));
    listEl.append(li);
  });
}

function setActive(idx) {
  active = idx;
  [...listEl.children].forEach((li, i) => li.classList.toggle("active", i === idx));
}

async function fetchSuggestions(q) {
  const mine = ++seq;
  if (!q.trim()) { items = []; render(q); return; }
  try {
    const r = await fetch(`${base}/api/suggestions?q=${encodeURIComponent(q)}`, {
      headers: { Accept: "application/json" },
    });
    if (mine !== seq) return; // a newer keystroke superseded us
    const data = await r.json();
    // OpenSearch suggestions: [query, [names], [descriptions], [urls]]
    const names = data[1] || [], descs = data[2] || [], urls = data[3] || [];
    items = names.map((n, i) => ({ name: n, desc: descs[i] || "", url: urls[i] || "" }));
    render(q);
  } catch (e) {
    if (mine !== seq) return;
    items = [];
    render(q);
  }
}

let debounce;
qEl.addEventListener("input", () => {
  clearTimeout(debounce);
  const q = qEl.value;
  debounce = setTimeout(() => fetchSuggestions(q), 120);
});

qEl.addEventListener("keydown", (e) => {
  if (e.key === "ArrowDown") { e.preventDefault(); setActive(Math.min(active + 1, items.length - 1)); }
  else if (e.key === "ArrowUp") { e.preventDefault(); setActive(Math.max(active - 1, -1)); }
});

formEl.addEventListener("submit", (e) => {
  e.preventDefault();
  if (active >= 0 && items[active]) go(queryToPath(items[active].name));
  else if (qEl.value.trim()) go(queryToPath(qEl.value));
});

document.getElementById("opts").addEventListener("click", () => {
  browser.runtime.openOptionsPage();
});

init();
