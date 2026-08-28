const fs = require("fs");
const path = require("path");
const root = process.cwd();
let fixed = 0;
function isUtf16(buf) {
  if (buf.length < 2) return false;
  if (buf[0] === 0xff && buf[1] === 0xfe) return true;
  if (buf.length >= 4 && buf[1] === 0 && buf[3] === 0) return true;
  return false;
}
function fixFile(filePath) {
  const buf = fs.readFileSync(filePath);
  if (!isUtf16(buf)) return;
  const name = path.basename(filePath);
  const BINARY_EXT = /\.(ico|png|jpe?g|gif|webp|woff2?|ttf|eot)$/i;
  if (BINARY_EXT.test(name)) {
    fs.unlinkSync(filePath);
    fixed += 1;
    console.log("Removed corrupted binary:", filePath);
    return;
  }
  let text = buf.toString("utf16le");
  if (text.charCodeAt(0) === 0xfeff) text = text.slice(1);
  fs.writeFileSync(filePath, text, "utf8");
  fixed += 1;
  console.log("Fixed:", filePath);
}
function shouldFix(name) {
  return (
    /\.(go|mod|sum|yml|yaml|ps1|ts|tsx|sql|md|js)$/i.test(name) ||
    name === "Dockerfile" ||
    name.startsWith(".env")
  );
}
function walk(dir) {
  if (!fs.existsSync(dir)) return;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const full = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      if (entry.name === "node_modules" || entry.name === ".git") continue;
      walk(full);
    } else if (shouldFix(entry.name)) {
      fixFile(full);
    }
  }
}
for (const d of ["services", "apps", "infra", "packages", "scripts", "docs"]) {
  walk(path.join(root, d));
}
for (const f of [".env.example", ".env", "README.md"]) {
  const full = path.join(root, f);
  if (fs.existsSync(full)) fixFile(full);
}
console.log("fixed", fixed);