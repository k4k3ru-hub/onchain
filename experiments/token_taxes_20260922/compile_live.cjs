const fs = require('fs');
const path = require('path');
const wrapper = require(path.join(process.argv[2], 'wrapper'));
const live = JSON.parse(fs.readFileSync('evidence/live.json'));
const compiled = {};
for (const [address, token] of Object.entries(live.tokens)) {
  const start = performance.now();
  const source = token.source;
  if (!source || source.compilation.name === 'FiatTokenProxy') continue;
  const version = source.compilation.compilerVersion.split('+')[0];
  const build = JSON.parse(fs.readFileSync(path.join(process.argv[3], version + '.json')));
  const solc = wrapper(require(path.join(process.argv[3], build.path)));
  if (solc.version().split('.Emscripten')[0] !== source.compilation.compilerVersion) {
    throw new Error('compiler version mismatch');
  }
  const input = JSON.parse(JSON.stringify(source.stdJsonInput));
  input.settings.outputSelection = {'*': {'*': ['evm.deployedBytecode', 'evm.methodIdentifiers'], '': ['ast']}};
  const output = JSON.parse(solc.compile(JSON.stringify(input)));
  const errors = (output.errors || []).filter(x => x.severity === 'error');
  if (errors.length) throw new Error(errors.map(x => x.formattedMessage).join('\n'));
  const identifier = source.compilation.fullyQualifiedName;
  const sep = identifier.lastIndexOf(':');
  const artifact = output.contracts[identifier.slice(0, sep)][identifier.slice(sep+1)];
  const immutableDeclarations = [];
  function visit(node, sourceName) {
    if (!node || typeof node !== 'object') return;
    if (node.nodeType === 'VariableDeclaration' && node.mutability === 'immutable') {
      immutableDeclarations.push({sourceName, id: node.id, name: node.name,
        type: node.typeDescriptions.typeString, src: node.src});
    }
    for (const value of Object.values(node)) {
      if (Array.isArray(value)) value.forEach(child => visit(child, sourceName));
      else if (value && typeof value === 'object') visit(value, sourceName);
    }
  }
  for (const [name, value] of Object.entries(output.sources)) visit(value.ast, name);
  compiled[address] = {compiler: solc.version(), compilerSHA256: build.sha256,
    artifact, immutableDeclarations, compileElapsedMs: Math.round(performance.now() - start)};
  console.log('compiled', address, source.compilation.name);
}
fs.writeFileSync('evidence/live-compiled.json', JSON.stringify(compiled, null, 2) + '\n');
