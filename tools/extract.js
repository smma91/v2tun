// Lists UI strings that are missing from frontend/i18n.json (English keys):
// HTML text and placeholder/title/aria-label attributes, t("…") in app.js,
// and T("…"), trErr("…") and logf(level, "…") in the Go code.
// Usage: node tools/extract.js          -> missing keys (JSON array)
//        node tools/extract.js unused   -> dictionary keys no longer used
const fs = require("fs");
const path = require("path");
const root = path.join(__dirname, "..", "cmd", "v2tun");
const entities = { amp: "&", lt: "<", gt: ">", quot: '"', "#39": "'" };
const norm = (s) => s.replace(/&(amp|lt|gt|quot|#39);/g, (_, e) => entities[e]).replace(/\s+/g, " ").trim();
const out = new Set();

// Text that is the same in every language: names, addresses, technical log
// lines that are only a prefix plus an error.
const same = new Set(["Cisco AnyConnect / ocserv", "Fortinet", "Palo Alto GlobalProtect", "Pulse / Ivanti", "Juniper Network Connect",
  "F5 BIG-IP", "Cloudflare", "Google", "DHCP", "https://...", "example.com", "vpn.example.com", "Subscription «%s»: %v", "resolve %s: %v", "v2tun", "v2rayN", "EN", "FA", "AR", "ZH", "SOCKS5", "HTTP", "DNS", "IPv6", "QUIC", "WebRTC"]);
const literal = (s) => !/[A-Za-z]{2}/.test(s) || same.has(s) || /^[\w .«»\/:-]*: %v$/.test(s) || s === "%v";

const html = fs.readFileSync(path.join(root, "frontend/index.html"), "utf8").replace(/<!--[\s\S]*?-->/g, "").replace(/<(script|style|svg)[\s\S]*?<\/\1>/g, "");
for (const m of html.matchAll(/>([^<>]+)</g)) if (!literal(norm(m[1]))) out.add(norm(m[1]));
for (const m of html.matchAll(/(?:placeholder|title|aria-label)="([^"]+)"/g)) if (!literal(norm(m[1])) && !m[1].includes("&#10;")) out.add(norm(m[1]));

const js = fs.readFileSync(path.join(root, "frontend/app.js"), "utf8");
for (const m of js.matchAll(/\bt\(\s*(["`])((?:\\.|(?!\1).)*)\1/g)) out.add(m[1] === '"' ? JSON.parse('"' + m[2] + '"') : m[2]);

for (const f of fs.readdirSync(root).filter((f) => f.endsWith(".go") && !f.endsWith("_test.go"))) {
  const src = fs.readFileSync(path.join(root, f), "utf8");
  for (const m of src.matchAll(/\b(?:T|trErr|logf\([^,()]+,)\s*\(?\s*"((?:\\.|[^"\\])*)"/g)) out.add(JSON.parse('"' + m[1] + '"'));
}
const used = new Set(out);
for (const s of out) if (literal(s)) out.delete(s);

const dict = JSON.parse(fs.readFileSync(path.join(root, "frontend/i18n.json"), "utf8"));
const res = process.argv[2] === "unused" ? Object.keys(dict).filter((k) => !used.has(k)) : [...out].filter((k) => !dict[k]);
console.log(JSON.stringify(res, null, 1));
