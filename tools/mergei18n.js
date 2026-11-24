// Adds or updates translations in frontend/i18n.json.
// tools/tr-extra.json: [["English key", "Persian", "Chinese", "Arabic"], …]
// (find the keys that need it with `node tools/extract.js`).
// `node tools/mergei18n.js prune` also drops keys the code no longer uses.
const fs = require("fs");
const path = require("path");
const { execFileSync } = require("child_process");
const file = path.join(__dirname, "..", "cmd", "v2tun", "frontend", "i18n.json");
const dict = JSON.parse(fs.readFileSync(file, "utf8"));
const extra = path.join(__dirname, "tr-extra.json");
const verbs = (s) => (s.match(/%[.\d]*[a-z]|\{\d\}/g) || []).sort().join(",");
if (fs.existsSync(extra)) {
  for (const [en, fa, zh, ar] of JSON.parse(fs.readFileSync(extra, "utf8").replace(/^\uFEFF/, ""))) {
    for (const v of [fa, zh, ar]) if (verbs(v) !== verbs(en)) throw new Error(`placeholders differ: ${en} -> ${v}`);
    dict[en] = { fa, zh, ar };
  }
}
if (process.argv[2] === "prune") {
  const unused = JSON.parse(execFileSync("node", [path.join(__dirname, "extract.js"), "unused"]));
  for (const k of unused) delete dict[k];
  console.log("removed:", unused.length);
}
const sorted = Object.fromEntries(Object.entries(dict).sort(([a], [b]) => a.localeCompare(b)));
fs.writeFileSync(file, JSON.stringify(sorted, null, 1) + "\n");
console.log("i18n.json:", Object.keys(sorted).length, "strings");
