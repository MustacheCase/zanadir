// Runs a Go test binary built for js/wasm under Node, on web/scan/memfs.js
// instead of Node's fs, so the tests use the file system the page gives Go.
// Usage: node wasm/run.js zanadir.test.wasm [-test.v ...]
"use strict";

const real = { process, fs: require("fs"), path: require("path") };
const wasm = real.fs.readFileSync(real.process.argv[2]);

require(real.path.join(__dirname, "..", "web", "scan", "memfs.js"));
require(real.path.join(__dirname, "..", "dist", "wasm_exec.js"));

const go = new Go();
go.argv = ["zanadir.test", ...real.process.argv.slice(3)];
go.env = { HOME: "/", TMPDIR: "/tmp" };
go.exit = (code) => real.process.exit(code);
WebAssembly.instantiate(wasm, go.importObject).then(({ instance }) => go.run(instance)).catch((err) => {
	console.error(err);
	real.process.exit(1);
});
