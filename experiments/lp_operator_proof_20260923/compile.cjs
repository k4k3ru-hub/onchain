// Research only: independently compile archived source with an explicit compiler.
const fs = require("node:fs");
const zlib = require("node:zlib");
const crypto = require("node:crypto");
const solc = require(process.argv[2]);
const raw = zlib.gunzipSync(fs.readFileSync(process.argv[3]));
const source = JSON.parse(raw);
if (!solc.version().startsWith(source.compilation.compilerVersion)) throw Error("compiler mismatch");
const input = structuredClone(source.stdJsonInput);
input.settings.outputSelection = {"*": {"*": ["abi", "storageLayout", "evm.bytecode", "evm.deployedBytecode"]}};
const output = JSON.parse(solc.compile(JSON.stringify(input)));
if ((output.errors || []).some(e => e.severity === "error")) throw Error("compilation failed");
const target = source.compilation.fullyQualifiedName;
const i = target.lastIndexOf(":");
const contract = output.contracts[target.slice(0,i)][target.slice(i+1)];
for (const [field, compiled] of [["creationBytecode", contract.evm.bytecode], ["runtimeBytecode", contract.evm.deployedBytecode]]) {
  if (compiled.object !== source[field].recompiledBytecode.replace(/^0x/, "")) throw Error(field+" mismatch");
  if (Object.keys(compiled.linkReferences).length) throw Error("external library links unsupported");
}
const evidence = {compiler:solc.version(), target, sourceSHA256:crypto.createHash("sha256").update(raw).digest("hex"), contract};
fs.writeFileSync(process.argv[4], JSON.stringify(evidence, null, 2)+"\n");
process.stdout.write(JSON.stringify({compiler:evidence.compiler,target,creationBytes:contract.evm.bytecode.object.length/2,runtimeBytes:contract.evm.deployedBytecode.object.length/2,creationAndRuntimeRecompiled:true})+"\n");
