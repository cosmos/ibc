# Dogfooding the relayer + attestor

Run your own relayer and attestor against an already-deployed set of ibc contracts and attestation light
client pair. This doc covers the current
eth-sepolia / base-sepolia dogfood deployment.

## 1. Clone & build

```bash
git clone https://github.com/cosmos/ibc.git
cd ibc
git checkout df/docs-updates   # until this merges into development
cd link
make build   # produces link/bin/ibc
```

## 2. What you need
- Generate a config file. We'll populate this in the third step:
  ```bash
  ./bin/ibc config new
  ```
- Generate your own relayer signing key and fund it with a little testnet ETH on **both** chains:
  ```bash
  ./bin/ibc keys new ecdsa relayer-gas
  ```
- Import the attestor private keys — don't generate your own, the
  deployed clients only trust these two. Import each with:
  ```bash
  # eth attestor private key: 0x10ecd31f821b806add9a20d2d94951721bd9a7bdee7748e3c59f26f0f0804a4f
  # base attestor private key: 0x0622496729f906200aeea0e790e15816b975c29ea31facaad71bdcc3b2d2a31e
  ./bin/ibc keys import ecdsa <name> --private-key <hex>
  ```
- Ask Dennis to send you IFT tokens.

## 3. Configure `ibc.yml`

Populate the config with the key references from above and information about the deployment below. Running with sqlite and with the attestor in-process with the relayer are recommended for ease of setup. Consult the
[readme](https://github.com/cosmos/ibc/tree/df/docs-updates/link#ibc-link) for a configuration reference and example.

Note you should set `finalityOffset: 1` on both `attestor.attestations[]` entries. Left
at the default, the attestor waits for eth-sepolia's real `"finalized"` tag,
which lags ~12-15 minutes.

## Current deployment

| | eth-sepolia | base-sepolia |
|---|---|---|
| `chainId` | `11155111` | `84532` |
| `rpc` | `https://eth-sepolia.g.alchemy.com/v2/1Ut-SXptWA3Tslnh301Ys` | `https://base-sepolia.g.alchemy.com/v2/1Ut-SXptWA3Tslnh301Ys` |
| `ics26Router` | `0xe20BccD900Fa1B48f46F5a483d9De063b07eDFCC` | `0x04357d2434523a31b6f89e0414053aeafcd10dee` |
| attestation client (`clientId`) | `base-sepolia-dogfood-3` | `eth-sepolia-dogfood-3` |
| IFT contract | `0xa5d1b01b31474c653ef1a03f258f0607cd938a5d` | `0x6ee8F624806210935a06b19A75f92f3c1fe1f4db` |

Attestor keys:

| Watches | Public address | Private key |
|---|---|---|
| eth-sepolia | `0xE461b1Ac84C4eF6E93BD23f1952e9111F2C48342` | `0x10ecd31f821b806add9a20d2d94951721bd9a7bdee7748e3c59f26f0f0804a4f` |
| base-sepolia | `0x7bE12b568c990067F94700c2F61839e8ac2eef7E` | `0x0622496729f906200aeea0e790e15816b975c29ea31facaad71bdcc3b2d2a31e` |

You can validate the config using the cli

```bash
./ibc/bin config validate
```

## 4. Run the relayer and attestor

```bash
./bin/ibc relayer run
./bin/ibc attestor run # if not running in-process with the relayer
```
Look for `{"event":"ready", ...}` to indicate services are ready.

## 5. Send a test transfer

`<IFT_CONTRACT>` and `<clientId>` depend on the direction:

| Sending from | `<IFT_CONTRACT>` | `<clientId>` |
|---|---|---|
| eth-sepolia → base-sepolia | `0xa5d1b01b31474c653ef1a03f258f0607cd938a5d` | `base-sepolia-dogfood-3` |
| base-sepolia → eth-sepolia | `0x6ee8F624806210935a06b19A75f92f3c1fe1f4db` | `eth-sepolia-dogfood-3` |

**Send transfer** (`cast`, from [foundry](https://getfoundry.sh)):
```bash
cast send <IFT_CONTRACT> \
  "iftTransfer(string,string,uint256,uint64)" \
  "<clientId>" "<receiver address>" 1000 "$(($(date +%s) + 3600))" \
  --rpc-url "$RPC_URL" --private-key "$RELAYER_GAS_PRIVATE_KEY"
```

**Submit it to relayer** (`grpcurl` — no local `.proto` needed, reflection
is always on for `ibc relayer run`):
```bash
grpcurl -plaintext \
  -d '{"chainId": "<source chainId>", "txHash": "<tx hash from above>"}' \
  127.0.0.1:3000 ibc.v2.relayer.RelayerApiService/Relay
```

**Poll for completion:**
```bash
grpcurl -plaintext \
  -d '{"chainId": "<source chainId>", "txHash": "<tx hash from above>"}' \
  127.0.0.1:3000 ibc.v2.relayer.RelayerApiService/Status
```
Look for `state: TRANSFER_STATE_COMPLETE`.

**Note:** logging is pretty poor right now. If you submit your transfer to the relayer
and don't see it logging that it's submitting a transaction within a couple of
minutes, you've probably misconfigured something — go back and double check your
`ibc.yml` or ask Dennis for guidance.