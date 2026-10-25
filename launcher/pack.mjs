// node pack.mjs --launcher PATH --out PATH --exec NAME=PATH [--file NAME=PATH]...
//               [--interp NAME:LOADER] [--cafile NAME]
//
// Appends the files and a text header to the static launcher (launcher.c).
// The --exec file is the one run; files unpack under these names.
//
// --interp NAME:LOADER points NAME's interpreter at the unpacked LOADER. NAME
// must already have a long placeholder interpreter (patchelf
// --set-interpreter), which the launcher overwrites in place at unpack time.
// A musl libc NEEDED entry becomes "libc.so" in place, a name musl's loader
// resolves to itself without a library search. Both edits keep the file
// layout: Bun keeps its JS payload inside the ELF, and relayouts (patchelf
// --set-rpath, --replace-needed) break it.
//
// --cafile NAME: the launcher sets SSL_CERT_FILE to the unpacked NAME when no
// CA store is configured or present.
import { createHash } from "node:crypto";
import { readFileSync, writeFileSync } from "node:fs";
import { parseArgs } from "node:util";

const { values: args } = parseArgs({
  options: {
    launcher: { type: "string" },
    out: { type: "string" },
    exec: { type: "string" },
    file: { type: "string", multiple: true, default: [] },
    interp: { type: "string" },
    cafile: { type: "string" },
  },
});

const files = [args.exec, ...args.file].map((spec) => {
  const [name, path] = spec.split("=");
  if (!/^[A-Za-z0-9._-]+$/.test(name) || !path) throw new Error(`bad file spec: ${spec}`);
  return { name, data: readFileSync(path) };
});
const byName = (name) => {
  const f = files.find((f) => f.name === name);
  if (!f) throw new Error(`no packed file named ${name}`);
  return f;
};
const header = [`exec ${files[0].name}`];

if (args.interp) {
  const [name, loader] = args.interp.split(":");
  byName(loader);
  const elf = byName(name).data;
  const u16 = (o) => elf.readUInt16LE(o);
  const u32 = (o) => elf.readUInt32LE(o);
  const u64 = (o) => Number(elf.readBigUInt64LE(o));
  if (elf.readUInt32BE(0) !== 0x7f454c46 || elf[4] !== 2 || elf[5] !== 1)
    throw new Error(`${name} is not a little-endian ELF64 file`);

  let interpOff, interpLen;
  for (let i = 0, ph = u64(0x20); i < u16(0x38); i++, ph += u16(0x36)) {
    if (u32(ph) === 3 /* PT_INTERP */) {
      interpOff = u64(ph + 8);
      interpLen = u64(ph + 32);
    }
  }
  if (interpOff === undefined) throw new Error(`${name} has no PT_INTERP`);

  const sh = (i) => u64(0x28) + i * u16(0x3a);
  const shstr = u64(sh(u16(0x3e)) + 0x18);
  let dynstr;
  for (let i = 0; i < u16(0x3c); i++) {
    const at = shstr + u32(sh(i));
    if (elf.toString("latin1", at, at + 8) === ".dynstr\0")
      dynstr = elf.subarray(u64(sh(i) + 0x18), u64(sh(i) + 0x18) + u64(sh(i) + 0x20));
  }
  if (!dynstr) throw new Error(`${name} has no .dynstr`);
  const libc = /\0(libc\.musl-[a-z0-9_]+\.so\.1)\0/.exec(dynstr.toString("latin1"));
  if (!libc) throw new Error(`${name} doesn't link musl libc`);
  dynstr.write("libc.so\0".padEnd(libc[1].length + 1, "\0"), libc.index + 1, "latin1");

  header.push(`interp ${name} ${loader} ${interpOff} ${interpLen}`);
}

if (args.cafile) {
  byName(args.cafile);
  header.push(`cafile ${args.cafile}`);
}

const hash = createHash("sha256");
for (const f of files) {
  hash.update(f.data);
  header.push(`file ${f.name} ${f.data.length}`);
}
header.unshift(`id ${hash.digest("hex").slice(0, 16)}`);

const text = header.join("\n") + "\n";
const tail = `PACK2 ${String(Buffer.byteLength(text)).padStart(9, "0")}\n`;
writeFileSync(
  args.out,
  Buffer.concat([readFileSync(args.launcher), ...files.map((f) => f.data), Buffer.from(text), Buffer.from(tail)]),
  { mode: 0o755 },
);
process.stdout.write(text);
