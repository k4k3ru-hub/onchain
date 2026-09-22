// Research compiler driver. An existing, explicitly supplied solcjs is used.
const fs = require('fs');
const solc = require(process.argv[2]);
const source = fs.readFileSync('Fixtures.sol', 'utf8');
const input = {
  language: 'Solidity', sources: {'Fixtures.sol': {content: source}},
  settings: {optimizer: {enabled: true, runs: 200}, evmVersion: 'paris',
    outputSelection: {'*': {'*': ['abi', 'evm.bytecode.object',
      'evm.deployedBytecode.object', 'evm.methodIdentifiers', 'storageLayout']}}}
};
const output = JSON.parse(solc.compile(JSON.stringify(input)));
const errors = (output.errors || []).filter(e => e.severity === 'error');
if (errors.length) throw new Error(errors.map(e => e.formattedMessage).join('\n'));
fs.writeFileSync('evidence/compiled.json', JSON.stringify({
  compiler: solc.version(), input, contracts: output.contracts['Fixtures.sol']
}, null, 2) + '\n');
console.log('compiled', Object.keys(output.contracts['Fixtures.sol']).join(', '));
