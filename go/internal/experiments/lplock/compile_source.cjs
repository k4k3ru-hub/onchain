// Verification utility. Compiler and downloaded sources remain outside the SDK.
const fs = require("node:fs");
const solc = require(process.argv[2]);
const source = JSON.parse(fs.readFileSync(process.argv[3], "utf8"));
if (!solc.version().startsWith(source.compilation.compilerVersion)) {
  throw new Error("failed to compile discovered source: compiler_version=mismatch");
}
const input = structuredClone(source.stdJsonInput);
input.settings.outputSelection = {
  "*": {
    "": ["ast"],
    "*": ["abi", "storageLayout", "evm.deployedBytecode"],
  },
};
const result = JSON.parse(solc.compile(JSON.stringify(input)));
if ((result.errors || []).some((item) => item.severity === "error")) {
  throw new Error("failed to compile discovered source: source=invalid");
}
const qualifiedName = source.compilation.fullyQualifiedName;
const split = qualifiedName.lastIndexOf(":");
const contract = result.contracts[qualifiedName.slice(0, split)][qualifiedName.slice(split + 1)];
if (contract.evm.deployedBytecode.object !== source.runtimeBytecode.recompiledBytecode.replace(/^0x/, "")) {
  throw new Error("failed to verify recompiled runtime: bytecode=mismatch");
}
// Independently compare compiler output to the runtime bytes that discovery
// matched against RPC. Only compiler-declared immutable insertion sites may
// differ; those repeated sites must contain the same observed value.
const deployed = contract.evm.deployedBytecode;
if (Object.keys(deployed.linkReferences || {}).length) {
  throw new Error("failed to verify runtime: external_library_links=unsupported");
}
const local = Buffer.from(deployed.object, "hex");
const onchain = Buffer.from(source.runtimeBytecode.onchainBytecode.replace(/^0x/, ""), "hex");
if (local.length !== onchain.length) {
  throw new Error("failed to verify runtime: bytecode_length=mismatch");
}
const immutableBytes = new Set();
for (const sites of Object.values(deployed.immutableReferences || {})) {
  let value;
  for (const { start, length } of sites) {
    const current = onchain.subarray(start, start + length).toString("hex");
    if (value !== undefined && value !== current) {
      throw new Error("failed to verify runtime: repeated_immutable=mismatch");
    }
    value = current;
    for (let i = start; i < start + length; i++) immutableBytes.add(i);
  }
}
for (let i = 0; i < local.length; i++) {
  if (local[i] !== onchain[i] && !immutableBytes.has(i)) {
    throw new Error("failed to verify runtime: executable_bytecode=mismatch");
  }
}
fs.writeFileSync(process.argv[4], JSON.stringify(result));
