// Offline fixtures from @mysten/sui 2.0.0. All addresses, digests, and the seed are
// synthetic PUBLIC TEST DATA. Never fund the deterministic test address.
// Usage: node generate_transaction_vectors.mjs /path/to/npm/project > transaction_vectors.json
import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const load = (name) => {
  const parts = name.split('/');
  const packageName = parts.splice(0, name.startsWith('@') ? 2 : 1).join('/');
  const root = resolve(process.argv[2], 'node_modules', packageName);
  const metadata = JSON.parse(readFileSync(resolve(root, 'package.json'), 'utf8'));
  if (packageName === '@mysten/sui' && metadata.version !== '2.0.0') throw new Error('Expected @mysten/sui 2.0.0');
  const entry = metadata.exports[parts.length ? './' + parts.join('/') : '.'];
  return import(pathToFileURL(resolve(root, typeof entry === 'string' ? entry : entry.import)));
};
const { bcs } = await load('@mysten/sui/bcs');
const { Ed25519Keypair } = await load('@mysten/sui/keypairs/ed25519');
const { TransactionDataBuilder } = await load('@mysten/sui/transactions');
const { messageWithIntent } = await load('@mysten/sui/cryptography');
const { toBase58 } = await load('@mysten/bcs');
const { blake2b } = await load('@noble/hashes/blake2.js');

const seed = Uint8Array.from({ length: 32 }, (_, i) => i);
const key = Ed25519Keypair.fromSecretKey(seed);
const sender = key.toSuiAddress();
const digest = toBase58(Uint8Array.from({ length: 32 }, (_, i) => i + 1));
const object = (id, version = '9007199254740993') => ({ objectId: id, version, digest });
const input = (index) => ({ Input: index });
const pure = (value) => ({ Pure: { bytes: value } });
const base = {
  sender,
  gasData: { payment: [object('0xa1'), object('0xa2', '99')], owner: sender, price: '1000', budget: '10000000' },
};
const definitions = [
  {
    name: 'split_gas_transfer_epoch',
    transaction: { V1: { ...base, expiration: { Epoch: 42 }, kind: { ProgrammableTransaction: {
      inputs: [pure(bcs.u64().serialize('1000000').toBytes()), pure(bcs.Address.serialize('0x77').toBytes())],
      commands: [
        { SplitCoins: { coin: { GasCoin: true }, amounts: [input(0)] } },
        { TransferObjects: { objects: [{ NestedResult: [0, 0] }], address: input(1) } },
      ],
    } } } },
  },
  {
    // Codec coverage only: this is not a runnable Move program.
    name: 'all_input_argument_and_supported_command_variants',
    transaction: { V1: { ...base, expiration: { None: true }, kind: { ProgrammableTransaction: {
      inputs: [
        { Object: { ImmOrOwnedObject: object('0xb1') } },
        { Object: { ImmOrOwnedObject: object('0xb2', '100') } },
        { Object: { SharedObject: { objectId: '0xc1', initialSharedVersion: '756017937', mutable: true } } },
        { Object: { Receiving: object('0xd1', '18446744073709551615') } },
        pure(new Uint8Array()),
      ],
      commands: [
        { MergeCoins: { destination: input(0), sources: [input(1)] } },
        { MoveCall: { package: '0x2', module: 'test', function: 'fixture', typeArguments: [
          'bool', 'u8', 'u16', 'u32', 'u64', 'u128', 'u256', 'address', 'signer',
          'vector<vector<u8>>', '0x2::coin::Coin<0x3::test::Nested<u64, vector<0x4::m::T>>>',
        ], arguments: [input(0), input(2), input(3), input(4)] } },
        { MakeMoveVec: { type: '0x2::coin::Coin<0x2::sui::SUI>', elements: [{ Result: 1 }] } },
      ],
    } } } },
  },
];

const vectors = [];
for (const item of definitions) {
  const raw = bcs.TransactionData.serialize(item.transaction).toBytes();
  const signed = await key.signTransaction(raw);
  vectors.push({
    name: item.name,
    bcs: Buffer.from(raw).toString('base64'),
    signingDigest: Buffer.from(blake2b(messageWithIntent('TransactionData', raw), { dkLen: 32 })).toString('hex'),
    transactionDigest: TransactionDataBuilder.getDigestFromBytes(raw),
    signature: signed.signature,
  });
}
process.stdout.write(JSON.stringify({ sdk: '@mysten/sui@2.0.0', sender, objectDigest: digest, vectors }, null, 2) + '\n');
