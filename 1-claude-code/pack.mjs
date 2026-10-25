// node pack.mjs LAUNCHER LOADER CLAUDE OUT
//
// CLAUDE must already have a long placeholder interpreter (patchelf
// --set-interpreter); the launcher overwrites it in place at unpack time.
// Its musl libc NEEDED entry becomes "libc.so" in place, a name musl's loader
// resolves to itself without a library search. Both edits keep the file
// layout: Bun keeps its JS payload inside the ELF, and relayouts (patchelf
// --set-rpath, --replace-needed) break it.
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";

const [launcherPath, loaderPath, claudePath, outPath] = process.argv.slice(2);
const launcher = readFileSync(launcherPath);
const loader = readFileSync(loaderPath);
const claude = readFileSync(claudePath);

const u16 = (o) => claude.readUInt16LE(o);
const u32 = (o) => claude.readUInt32LE(o);
const u64 = (o) => Number(claude.readBigUInt64LE(o));

if (claude.readUInt32BE(0) !== 0x7f454c46 || claude[4] !== 2 || claude[5] !== 1)
  throw new Error("claude is not a little-endian ELF64 file");

let interpOff, interpLen;
for (let i = 0, ph = u64(0x20); i < u16(0x38); i++, ph += u16(0x36)) {
  if (u32(ph) === 3 /* PT_INTERP */) {
    interpOff = u64(ph + 8);
    interpLen = u64(ph + 32);
  }
}
if (interpOff === undefined) throw new Error("claude has no PT_INTERP");

const sh = (i) => u64(0x28) + i * u16(0x3a);
const shstr = u64(sh(u16(0x3e)) + 0x18);
let dynstr;
for (let i = 0; i < u16(0x3c); i++) {
  const name = claude.toString("latin1", shstr + u32(sh(i)), shstr + u32(sh(i)) + 8);
  if (name === ".dynstr\0") dynstr = claude.subarray(u64(sh(i) + 0x18), u64(sh(i) + 0x18) + u64(sh(i) + 0x20));
}
if (!dynstr) throw new Error("claude has no .dynstr");

const libc = /\0(libc\.musl-[a-z0-9_]+\.so\.1)\0/.exec(dynstr.toString("latin1"));
if (!libc) throw new Error("claude doesn't link musl libc");
dynstr.write("libc.so\0".padEnd(libc[1].length + 1, "\0"), libc.index + 1, "latin1");

const id = createHash("sha256").update(loader).update(claude).digest("hex").slice(0, 16);
const trailer = `CLAUDEPACK1 ${id} ${loader.length} ${claude.length} ${interpOff} ${interpLen}\n`.padEnd(128, " ");
writeFileSync(outPath, Buffer.concat([launcher, loader, claude, Buffer.from(trailer)]), { mode: 0o755 });
console.log(trailer.trim());
