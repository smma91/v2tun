const $ = (id) => document.getElementById(id);
const api = () => window.go.main.App;
// ---- i18n: English strings are the keys of i18n.json ----
const LANGS = { en: { dir: "ltr", locale: "en-US" }, fa: { dir: "rtl", locale: "fa-IR" }, ar: { dir: "rtl", locale: "ar-EG" }, zh: { dir: "ltr", locale: "zh-CN" } };
let lang = "en";
let dict = {};
// Loaded before anything is drawn, so no text is ever built from a missing dictionary.
const i18nReady = fetch("i18n.json").then((r) => r.json()).then((d) => (dict = d)).catch((e) => console.error(e));
const norm = (s) => s.replace(/\s+/g, " ").trim();

// t translates an English key; {0}, {1}… are replaced by the arguments.
function t(key, ...args) {
  let s = lang === "en" ? key : dict[key]?.[lang] ?? dict[norm(key)]?.[lang] ?? key;
  args.forEach((a, i) => (s = s.split(`{${i}}`).join(a)));
  return s;
}
const num = (n) => Number(n).toLocaleString(LANGS[lang].locale);

// Translates the static page: text nodes and placeholder/title attributes.
// The English original is kept on each node so the language can change again.
function translatePage() {
  const w = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  for (let n; (n = w.nextNode()); ) {
    if (n.__src === undefined) {
      if (!dict[norm(n.nodeValue)] || n.parentElement?.closest("[translate=no]")) continue;
      n.__src = n.nodeValue;
    }
    const lead = n.__src.match(/^\s*/)[0], tail = n.__src.match(/\s*$/)[0];
    n.nodeValue = lead + t(norm(n.__src)) + tail;
  }
  for (const el of document.querySelectorAll("[placeholder],[title],[aria-label]")) {
    for (const a of ["placeholder", "title", "aria-label"]) {
      if (!el.hasAttribute(a)) continue;
      const k = "__" + a;
      if (el[k] === undefined) {
        if (!dict[norm(el.getAttribute(a))]) continue;
        el[k] = el.getAttribute(a);
      }
      el.setAttribute(a, t(norm(el[k])));
    }
  }
  document.documentElement.lang = lang;
  document.documentElement.dir = LANGS[lang].dir;
  $("langSel").value = lang;
  document.querySelector(".app").classList.add("ready");
}

async function setLang(l) {
  lang = LANGS[l] ? l : "en";
  translatePage();
  if (settings) {
    showTab(currentTab);
    render();
    renderSplit();
    renderDNS();
    preflight();
    loadServers(); // server / subscription errors come back in the new language
    reloadLogs();
    fillAdapters(adapters); // lists built with t() must be rebuilt
    loadRunningApps();
  }
}

// The log is re-rendered by the backend in the current language.
async function reloadLogs() {
  try {
    const list = (await api().Logs()) || [];
    $("logs").innerHTML = "";
    list.forEach(addLog);
  } catch (e) { console.error(e); }
}

let settings = null;
let status = { running: false };
let connecting = false;

function bytes(n) {
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (n >= 1024 && i < u.length - 1) { n /= 1024; i++; }
  return `${num(n.toFixed(i ? 1 : 0))} ${u[i]}`;
}

function duration(sec) {
  const h = Math.floor(sec / 3600), m = Math.floor(sec / 60) % 60, s = sec % 60;
  return [h, m, s].map((x) => String(x).padStart(2, "0")).join(":");
}

function render() {
  const on = status.running, busy = connecting || status.busy;
  const power = $("power");
  power.classList.toggle("on", on && !busy);
  power.classList.toggle("busy", busy);
  power.disabled = busy;
  const pill = $("pill");
  pill.className = "pill" + (busy ? " busy" : on ? " on" : "");
  pill.textContent = busy ? t("Please wait") : on ? t("Connected") : t("Offline");
  const via = settings?.proxySource === "builtin" ? t("Built-in core") : "v2rayN";
  $("stateText").textContent = busy ? (on ? t("Disconnecting…") : t("Connecting…")) : on ? (status.proxyMode ? t("Local proxy active: {0}", (proxyAddrs[0] || "")) : t("All traffic goes through {0}", via)) : t("Tap to connect");
  $("uptime").textContent = on && status.since ? duration(Math.floor(Date.now() / 1000 - status.since)) : "";
  $("applyHint").hidden = !on;

  $("down").textContent = on ? bytes(status.downRate) + "/s" : num(0);
  $("up").textContent = on ? bytes(status.upRate) + "/s" : num(0);
  $("conns").textContent = num(on ? status.conns || 0 : 0);

  const ex = status.exit || {};
  $("exitIp").textContent = on && ex.ip ? ex.ip : "—";
  $("exitWhere").textContent = on && ex.ip ? [ex.country, ex.city, ex.org].filter(Boolean).join(" · ") : on ? t("Fetching…") : "—";
  $("servers").textContent = on && status.servers?.length ? "server: " + status.servers.join(", ") : "";
}

function renderAlerts(pre) {
  const box = $("alerts");
  box.innerHTML = "";
  const add = (cls, text) => {
    const d = document.createElement("div");
    d.className = "alert " + cls;
    d.textContent = text;
    box.appendChild(d);
  };
  if (status.running) return;
  if (pre.conflict) add("bad", t("Another TUN is active ({0}), e.g. v2rayN's own TUN mode. Turn it off first.", pre.conflict));
  if (settings.proxySource === "builtin") {
    if (!(serversView?.servers || []).some((s) => !s.error)) add("warn", t("No servers yet. Add a link or subscription in the \"Servers\" tab."));
    return;
  }
  if (!pre.socksOk) add("warn", t("The SOCKS proxy at {0} does not respond. Open v2rayN and connect to a server.", settings.socksAddr));
}

// ---- tabs ----
let currentTab = "home";

function showTab(name) {
  currentTab = name;
  document.querySelectorAll("#tabs button").forEach((b) => b.classList.toggle("active", b.dataset.tab === name));
  document.querySelectorAll(".page").forEach((p) => (p.hidden = p.id !== "page-" + name));
  window.scrollTo(0, 0);
}

// ---- servers (embedded core) ----
let serversView = null;

function delayBadge(s) {
  const b = document.createElement("span");
  b.className = "delay";
  if (s.error) return b;
  if (s.delay > 0) {
    b.textContent = s.delay + "ms";
    b.classList.add(s.delay < 300 ? "good" : s.delay < 800 ? "mid" : "slow");
  } else if (s.delay < 0) {
    b.textContent = t("Offline");
    b.classList.add("fail");
  } else {
    b.textContent = "—";
  }
  return b;
}

// Ping and download speed of one server, plus a button to speed-test just it.
function metrics(s) {
  const box = document.createElement("span");
  box.className = "metrics";
  const ping = document.createElement("span");
  ping.className = "m-ping" + (s.ping < 0 ? " fail" : "");
  ping.title = t("Ping");
  ping.textContent = `${t("Ping")} ${s.ping > 0 ? s.ping + "ms" : s.ping < 0 ? "✕" : "—"}`;
  const speed = document.createElement("span");
  const running = serversView?.speeding === s.id;
  speed.className = "m-speed" + (s.speed < 0 ? " fail" : "") + (running ? " running" : "");
  speed.title = t("Speed test");
  speed.dir = "ltr";
  speed.textContent = running ? "…" : s.speed > 0 ? `${s.speed >= 10 ? Math.round(s.speed) : s.speed.toFixed(1)} Mbps` : s.speed < 0 ? "✕" : "— Mbps";
  const go = document.createElement("button");
  go.className = "m-go";
  go.title = t("Speed test");
  go.textContent = "⚡";
  go.disabled = !!serversView?.speeding;
  go.onclick = (e) => { e.stopPropagation(); api().SpeedTest([s.id]); };
  box.append(ping, speed, go);
  return box;
}

function serverRow(s, selected) {
  const row = document.createElement("div");
  row.className = "srv" + (selected ? " sel" : "") + (s.error ? " bad" : "");
  const dot = document.createElement("span");
  dot.className = "dot";
  const info = document.createElement("div");
  info.className = "info";
  const name = document.createElement("span");
  name.className = "name";
  name.textContent = s.name || s.addr;
  const meta = document.createElement("span");
  meta.className = "meta";
  meta.textContent = s.error ? t(s.error) : `${s.type} · ${s.addr}`;
  info.append(name, meta);
  if (s.id !== "auto" && !s.error) info.append(metrics(s)); // own line: names stay readable
  row.append(dot, info, delayBadge(s));
  if (s.id !== "auto") {
    const del = document.createElement("button");
    del.className = "del";
    del.textContent = "×";
    del.title = t("Remove");
    del.onclick = (e) => { e.stopPropagation(); api().RemoveServer(s.id); };
    row.append(del);
  }
  if (!s.error) row.onclick = () => api().SelectServer(s.id);
  return row;
}

function renderServers() {
  const v = serversView;
  if (!v) return;
  const box = $("serverList");
  box.innerHTML = "";
  $("failover").checked = v.failover;
  $("autoHours").value = String(v.autoHours);
  $("testBtn").disabled = v.testing;
  $("testBtn").textContent = v.testing ? t("Testing…") : t("Real delay");
  $("pingBtn").disabled = v.pinging;
  $("pingBtn").textContent = v.pinging ? t("Testing…") : t("Ping");
  $("speedBtn").disabled = !!v.speeding;
  $("speedBtn").textContent = v.speeding ? t("Testing…") : t("Speed test");
  $("updSubsBtn").disabled = v.updating;
  $("updSubsBtn").textContent = v.updating ? t("Fetching…") : t("Update");

  const servers = v.servers || [];
  const usable = servers.filter((s) => !s.error);
  const sel = v.selected || "auto";
  const cur = servers.find((s) => s.id === sel);
  renderSource();

  if (!servers.length && !(v.subs || []).length) {
    box.innerHTML = '<div class="group empty">' + t("No servers yet.<br>Copy a config link and press \"Add from clipboard\",<br>or enter a subscription URL.") + "</div>";
    return;
  }
  // "auto" pseudo-server
  const auto = document.createElement("div");
  auto.className = "group";
  const best = usable.filter((s) => s.delay > 0).sort((a, b) => a.delay - b.delay)[0];
  auto.append(serverRow({ id: "auto", name: t("Auto (fastest server)"), type: "auto", addr: best ? t("Now: {0}", best.name) : t("Among all servers"), delay: best ? best.delay : 0 }, sel === "auto"));
  box.append(auto);

  const groups = [{ id: "", name: t("Manual servers") }, ...(v.subs || [])];
  for (const g of groups) {
    const list = servers.filter((s) => s.sub === g.id);
    if (!list.length && !g.id) continue;
    const el = document.createElement("div");
    el.className = "group";
    const head = document.createElement("div");
    head.className = "group-head";
    const title = document.createElement("b");
    title.textContent = g.name;
    const info = document.createElement("small");
    if (g.id) {
      info.textContent = g.error ? t("Error: {0}", g.error) : t("{0} servers", num(list.length)) + (g.updated ? " · " + new Date(g.updated * 1000).toLocaleString(LANGS[lang].locale, { dateStyle: "short", timeStyle: "short" }) : "");
      if (g.error) info.className = "err";
    } else {
      info.textContent = t("{0} servers", num(list.length));
    }
    head.append(title, info);
    if (g.id) {
      const del = document.createElement("button");
      del.className = "link";
      del.textContent = "×";
      del.title = t("Remove subscription");
      del.onclick = () => { if (confirm(t("Remove subscription \"{0}\" and its servers?", g.name))) api().RemoveSubscription(g.id); };
      head.append(del);
    }
    el.append(head);
    list.forEach((s) => el.append(serverRow(s, s.id === sel)));
    box.append(el);
  }
}

async function loadServers() {
  try { serversView = await api().GetServers(); renderServers(); } catch (e) { console.error(e); }
}

// ---- connection mode & local proxy ----
let proxyAddrs = [];

async function loadProxyAddrs() {
  try { proxyAddrs = (await api().ProxyAddresses()) || []; } catch { proxyAddrs = []; }
  renderMode();
}

function renderMode() {
  if (!settings) return;
  const builtin = settings.proxySource === "builtin";
  const mode = builtin ? settings.mode : "tun"; // v2rayN is already a proxy
  document.querySelectorAll("#modeSeg button").forEach((b) => {
    b.classList.toggle("active", mode === b.dataset.m);
    b.disabled = b.dataset.m === "proxy" && !builtin;
  });
  $("modeSeg").title = builtin ? "" : t("\"Proxy only\" works with the built-in core; v2rayN already has its own proxy");
  $("proxyAddrLabel").textContent = builtin ? proxyAddrs.map((a) => "socks5/http://" + a).join("  ") : "";
  $("leakGuardRow").hidden = settings.killSwitch || mode === "proxy";
  const box = $("proxyAddrs");
  box.innerHTML = "";
  for (const a of proxyAddrs) {
    for (const scheme of ["socks5://", "http://"]) {
      const d = document.createElement("code");
      d.textContent = scheme + a;
      box.append(d);
    }
  }
}

function renderSource() {
  renderMode();
  document.querySelectorAll("#sourceSeg button").forEach((b) => b.classList.toggle("active", settings.proxySource === b.dataset.s));
  $("socksLabel").textContent = settings.socksAddr;
  const v = serversView;
  const cur = (v?.servers || []).find((x) => x.id === (v?.selected || "auto"));
  $("activeServer").textContent = settings.proxySource === "builtin" ? (cur ? cur.name : t("Auto (fastest)")) : "";
}

async function preflight() {
  if (status.running || connecting) return;
  try { renderAlerts(await api().Preflight()); } catch (e) { /* ignore */ }
}

function fillSettings() {
  $("killSwitch").checked = settings.killSwitch;
  $("allowLan").checked = settings.allowLan;
  $("matchTimezone").checked = settings.matchTimezone;
  $("autoConnect").checked = settings.autoConnect;
  $("allowQuic").checked = settings.allowQuic;
  $("exitOnClose").checked = settings.exitOnClose;
  $("socks").value = settings.socksAddr;
  $("dns").value = (settings.dns || []).join(", ");
  $("proxyPort").value = settings.proxyPort;
  $("proxyLan").checked = settings.proxyLan;
  $("systemProxy").checked = settings.systemProxy;
  $("leakGuard").checked = !settings.disableLeakGuard;
  $("remoteDns").checked = !settings.localDns;
  renderMode();
  $("domains").value = (settings.domains || []).join("\n");
  renderSplit();
  renderSource();
}

// ---- split tunnel ----
function renderSplit() {
  $("appList").dataset.empty = t("The list is empty; all apps go through the tunnel"); // shown by CSS when empty
  document.querySelectorAll("#appMode button").forEach((b) => b.classList.toggle("active", settings.appMode === b.dataset.m));
  document.querySelectorAll("#domainMode button").forEach((b) => b.classList.toggle("active", settings.domainMode === b.dataset.m));
  const ul = $("appList");
  ul.innerHTML = "";
  (settings.apps || []).forEach((p, i) => {
    const li = document.createElement("li");
    const name = document.createElement("span");
    name.textContent = p.split(/[\\/]/).pop();
    const full = document.createElement("small");
    full.textContent = p.includes("\\") ? p : "";
    full.title = p;
    const del = document.createElement("button");
    del.textContent = "×";
    del.title = t("Remove");
    del.onclick = () => { settings.apps.splice(i, 1); renderSplit(); save(); };
    li.append(name, full, del);
    ul.appendChild(li);
  });
}

function addApp(path) {
  if (!path) return;
  settings.apps = settings.apps || [];
  if (!settings.apps.some((a) => a.toLowerCase() === path.toLowerCase())) settings.apps.push(path);
  renderSplit();
  save();
}

async function loadRunningApps() {
  const sel = $("appPick");
  const list = await api().RunningApps();
  sel.innerHTML = "";
  sel.append(new Option(t("Pick from running apps…"), ""));
  for (const a of list || []) {
    const o = new Option(a.name, a.path);
    o.title = a.path;
    sel.append(o);
  }
}

async function save() {
  settings = {
    ...settings,
    killSwitch: $("killSwitch").checked,
    allowLan: $("allowLan").checked,
    matchTimezone: $("matchTimezone").checked,
    autoConnect: $("autoConnect").checked,
    allowQuic: $("allowQuic").checked,
    exitOnClose: $("exitOnClose").checked,
    socksAddr: $("socks").value.trim() || "127.0.0.1:10808",
    dns: $("dns").value.split(/[\s,\u060C]+/).filter(Boolean),
    proxyPort: Math.min(65535, Math.max(1, parseInt($("proxyPort").value, 10) || 2080)),
    proxyLan: $("proxyLan").checked,
    systemProxy: $("systemProxy").checked,
    disableLeakGuard: !$("leakGuard").checked,
    localDns: !$("remoteDns").checked,
    domains: $("domains").value.split(/\r?\n/).map((s) => s.trim()).filter(Boolean),
  };
  await api().SaveSettings(settings);
  loadProxyAddrs();
  preflight();
}

function addLog(l) {
  const box = $("logs");
  const d = document.createElement("div");
  d.className = l.level;
  const t = document.createElement("time");
  t.textContent = l.time;
  d.append(t, l.msg);
  box.appendChild(d);
  while (box.childNodes.length > 400) box.firstChild.remove();
  box.scrollTop = box.scrollHeight;
  if (l.level === "error") $("logBox").open = true;
}

async function toggle() {
  connecting = true;
  render();
  try {
    if (status.running) await api().Disconnect();
    else await api().Connect();
  } catch (e) {
    preflight();
  } finally {
    connecting = false;
    status = await api().Status();
    render();
  }
}

async function leak() {
  const btn = $("leakBtn"), box = $("leakResult");
  btn.disabled = true;
  btn.textContent = t("Testing…");
  try {
    const r = await api().LeakTest();
    box.hidden = false;
    box.className = "leak " + (r.unknown ? "unknown" : r.pass ? "pass" : "fail");
    const stun = r.stunIp ? `<code>${r.stunIp}</code>` : t("Blocked (no leak)");
    const verdict = r.unknown ? t("? Unknown: the web IP could not be read") : r.pass ? t("✓ Pass") : t("✗ Fail");
    box.innerHTML = `${verdict}<br>${t("Web IP:")} <code>${r.httpIp || "—"}</code><br>WebRTC: ${stun}`;
  } finally {
    btn.disabled = false;
    btn.textContent = t("Run test");
  }
}

// ---- DNS of the real connection ----
let adapters = [];

function selectedAdapter() {
  const idx = Number($("adapter").value);
  return adapters.find((a) => a.index === idx);
}

function renderDNS() {
  const a = selectedAdapter();
  $("dnsNow").textContent = a ? [...(a.dns4 || []), ...(a.dns6 || [])].join("  ") || "—" : "";
  document.querySelectorAll("#dnsSeg button").forEach((b) => b.classList.toggle("active", !!a && a.preset === b.dataset.p));
  $("dnsRestore").hidden = !a?.hasBackup;
}

function fillAdapters(list) {
  const prev = Number($("adapter").value);
  adapters = list || [];
  const sel = $("adapter");
  sel.innerHTML = "";
  if (!adapters.length) {
    sel.append(new Option(t("No connection with internet access found"), ""));
  }
  for (const a of adapters) {
    sel.append(new Option(`${a.name}${a.uplink ? t(" (current internet)") : ""} — ${a.description}`, a.index));
  }
  const keep = adapters.find((a) => a.index === prev) || adapters.find((a) => a.uplink) || adapters[0];
  if (keep) sel.value = keep.index;
  renderDNS();
}

async function loadAdapters() {
  fillAdapters(await api().ListAdapters());
}

async function applyDNS(preset) {
  const a = selectedAdapter();
  if (!a) return;
  const seg = $("dnsSeg");
  seg.classList.add("busy");
  try {
    fillAdapters(await api().SetAdapterDNS(a.index, preset));
  } catch (e) {
    loadAdapters();
  } finally {
    seg.classList.remove("busy");
  }
}

window.addEventListener("DOMContentLoaded", async () => {
  await i18nReady;
  loadRunningApps();
  $("appPick").addEventListener("focus", loadRunningApps, { once: true });
  $("appAdd").addEventListener("click", () => { addApp($("appPick").value); $("appPick").value = ""; });
  $("appBrowse").addEventListener("click", async () => addApp(await api().BrowseApp()));
  document.querySelectorAll("#appMode button").forEach((b) => b.addEventListener("click", () => { settings.appMode = b.dataset.m; renderSplit(); save(); }));
  document.querySelectorAll("#domainMode button").forEach((b) => b.addEventListener("click", () => { settings.domainMode = b.dataset.m; renderSplit(); save(); }));
  $("domains").addEventListener("change", save);
  document.querySelectorAll(".chip[data-add]").forEach((c) => c.addEventListener("click", () => {
    const have = new Set($("domains").value.split(/\r?\n/).map((s) => s.trim()).filter(Boolean));
    c.dataset.add.split("\n").forEach((d) => have.add(d));
    $("domains").value = [...have].join("\n");
    save();
  }));
  loadAdapters();
  $("adapter").addEventListener("change", renderDNS);
  $("dnsReload").addEventListener("click", loadAdapters);
  document.querySelectorAll("#dnsSeg button").forEach((b) => b.addEventListener("click", () => applyDNS(b.dataset.p)));
  $("dnsRestore").addEventListener("click", () => applyDNS("restore"));
  document.querySelectorAll(".chip[data-dns]").forEach((c) => c.addEventListener("click", () => { $("dns").value = c.dataset.dns; save(); }));
});

window.addEventListener("DOMContentLoaded", async () => {
  // Live updates first, so one failing call below can never freeze the UI.
  window.runtime.EventsOn("status", (s) => { status = s || { running: false }; render(); });
  window.runtime.EventsOn("log", addLog);
  await i18nReady;
  settings = await api().GetSettings();
  lang = LANGS[settings.lang] ? settings.lang : "en";
  translatePage();
  $("langSel").addEventListener("change", async (e) => {
    settings.lang = e.target.value;
    await api().SetLang(settings.lang); // backend first: logs and errors follow
    await setLang(settings.lang);
  });
  fillSettings();
  try { ((await api().Logs()) || []).forEach(addLog); } catch (e) { console.error(e); }
  try { status = (await api().Status()) || status; } catch (e) { console.error(e); }
  render();
  preflight();

  $("power").addEventListener("click", toggle);
  $("leakBtn").addEventListener("click", leak);
  $("refreshExit").addEventListener("click", async () => { status.exit = await api().RefreshExit(); render(); });
  ["killSwitch", "leakGuard", "allowLan", "matchTimezone", "autoConnect", "exitOnClose", "allowQuic", "proxyLan", "systemProxy", "proxyPort", "remoteDns"].forEach((id) => $(id).addEventListener("change", save));

  // tabs, proxy source, window buttons
  document.querySelectorAll("#tabs button").forEach((b) => b.addEventListener("click", () => showTab(b.dataset.tab)));
  document.querySelectorAll("#sourceSeg button").forEach((b) => b.addEventListener("click", () => {
    settings.proxySource = b.dataset.s;
    renderSource();
    save();
    render();
    if (b.dataset.s === "builtin" && !(serversView?.servers || []).length) showTab("servers");
  }));
  api().Version().then((v) => ($("appVersion").textContent = "v" + v)).catch(() => {});
  document.querySelectorAll("#modeSeg button").forEach((b) => b.addEventListener("click", () => {
    if (b.disabled) return;
    settings.mode = b.dataset.m;
    renderMode();
    save();
  }));
  loadProxyAddrs();
  $("winMin").addEventListener("click", () => window.runtime.WindowMinimise());
  $("winClose").addEventListener("click", () => api().CloseWindow());
  window.runtime.EventsOn("servers", (v) => { serversView = v; renderServers(); });
  loadServers();
  const busyBtn = async (id, fn) => { $(id).disabled = true; try { await fn(); } catch (e) { alert(e); } finally { $(id).disabled = false; } };
  $("pasteBtn").addEventListener("click", () => busyBtn("pasteBtn", async () => {
    const n = await api().PasteLinks();
    if (!n) alert(t("No new server in the clipboard (duplicate or invalid)."));
  }));
  $("linksAdd").addEventListener("click", () => busyBtn("linksAdd", async () => {
    await api().AddLinks($("links").value);
    $("links").value = "";
  }));
  $("subAdd").addEventListener("click", () => busyBtn("subAdd", async () => {
    await api().AddSubscription($("subName").value.trim(), $("subUrl").value.trim());
    $("subUrl").value = $("subName").value = "";
  }));
  $("ovpnAdd").addEventListener("click", () => busyBtn("ovpnAdd", async () => {
    const name = await api().ImportOVPN($("ovpnUser").value.trim(), $("ovpnPass").value);
    if (name) $("ovpnPass").value = "";
  }));
  $("ocAdd").addEventListener("click", () => busyBtn("ocAdd", async () => {
    await api().AddOpenConnect($("ocServer").value, $("ocFlavor").value, $("ocUser").value.trim(), $("ocPass").value, $("ocGroup").value.trim(), "");
    $("ocServer").value = $("ocPass").value = "";
  }));
  $("testBtn").addEventListener("click", () => api().TestServers());
  $("pingBtn").addEventListener("click", () => api().PingServers());
  $("speedBtn").addEventListener("click", () => api().SpeedTest([]));
  $("updSubsBtn").addEventListener("click", () => api().UpdateSubscriptions());
  $("failover").addEventListener("change", () => api().SetServerOptions(Number($("autoHours").value), $("failover").checked));
  $("autoHours").addEventListener("change", () => api().SetServerOptions(Number($("autoHours").value), $("failover").checked));

  // Windows integration (tray menu can change these too)
  $("autostart").checked = await api().GetAutostart();
  $("autostart").addEventListener("change", async (e) => {
    try { await api().SetAutostart(e.target.checked); } catch { e.target.checked = !e.target.checked; }
  });
  $("quitBtn").addEventListener("click", () => api().Quit());
  window.runtime.EventsOn("settings", (s) => { settings = s; fillSettings(); });
  setInterval(async () => { $("autostart").checked = await api().GetAutostart(); }, 3000);
  ["socks", "dns"].forEach((id) => $(id).addEventListener("change", save));
  setInterval(preflight, 4000);
});
