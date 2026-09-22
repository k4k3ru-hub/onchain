// Research only. Inputs and compiler paths are supplied by the fixed test driver.
const fs = require('node:fs');
const path = require('node:path');
const crypto = require('node:crypto');
const wrapper = require(path.join(process.argv[2], 'wrapper'));
const manifest = JSON.parse(fs.readFileSync(process.argv[3], 'utf8'));
const binary = path.join(path.dirname(process.argv[3]), manifest.path);
const bytes = fs.readFileSync(binary);
if ('0x' + crypto.createHash('sha256').update(bytes).digest('hex') !== manifest.sha256) {
  throw new Error('compiler checksum mismatch');
}
const solc = wrapper(require(binary));
const input = fs.readFileSync(0, 'utf8');
const output = solc.compile(input);
fs.writeFileSync(1, output);
