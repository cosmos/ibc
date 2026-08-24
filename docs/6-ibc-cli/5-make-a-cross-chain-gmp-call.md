---
title: "Make a cross-chain GMP call"
description: "Call a contract on another chain over IBC, and receive the result back as an acknowledgement."
---

This guide uses the General Message Passing (GMP) app to call a contract on another chain. You'll send the call from a source chain and execute it on the destination chain from an account derived from the sender.

## Before you begin

The commands on this page need:

- [Foundry](https://getfoundry.sh) v1.5.1 or later, for `forge` and `cast`
- [jq](https://jqlang.org/download/) installed
- [Git](https://git-scm.com/downloads) installed

Follow the [tutorial](/ibc-cli/tutorial-deploy-ibc-and-send-a-token) to set up a deployment with a GMP app, a client, and a relayer.

Once you have a live connection and running relayer, you'll need the following values from each chain. Run the commands in this guide from the `ibc/link` directory:

```bash
./bin/ibc deploy show 41001 | jq '{router: .core.router, gmp: .gmp.address, clients: [.clients[].clientId]}'
./bin/ibc deploy show 41002 | jq '{router: .core.router, gmp: .gmp.address, clients: [.clients[].clientId]}'

```

Print the private key you imported from the tutorial:

```bash
./bin/ibc keys show deployer --private | jq -r '.privateKey'
```

Export the values from the outputs above:

```bash
export RPC_A=http://localhost:8545
export RPC_B=http://localhost:8745
export CLIENT_A=link-41001-41002
export CLIENT_B=link-41001-41002
export GMP_A=0xA0408F356956Ae9BECa5Ac57040695012fa3CAAB
export GMP_B=0xA0408F356956Ae9BECa5Ac57040695012fa3CAAB
export KEY=0x33fa40f84e854b941c2b0436dd4a256e1df1cb41b9c1c0ccc8446408c19b8bf9
export SENDER=$(cast wallet address --private-key $KEY)
```

## 1. Deploy a contract to call

A GMP call arrives on the destination as a contract call, carrying the packet's payload as its call data.

Every GMP call executes through an account contract that belongs to the sender, so the target reads that account's address as `msg.sender`.

Create a Foundry project in the `ibc/link` directory:

```bash
forge init gmp-guide
```

1. Write a receiver that stores a value it is passed and the caller. Save it in the project as `gmp-guide/src/Remote.sol`:

```solidity
// SPDX-License-Identifier: Apache-2.0
pragma solidity ^0.8.28;

contract Remote {
    uint256 public value;
    address public lastCaller;

    function set(uint256 v) external {
        value = v;
        lastCaller = msg.sender;
    }
}
```

2. Deploy the contract on the destination chain:

```bash
cd gmp-guide
forge create src/Remote.sol:Remote --rpc-url $RPC_B --private-key $KEY --broadcast
cd ..
```

```
Deployer: 0x58A57ed9d8d624cBD12e2C467D34787555bB1b25
Deployed to: 0x890EdF334Fa33373ec945Bdd757C55DfB4e038e6
Transaction hash: 0xb9173f4c897c0621d7756ea81fb731c0b74ebdf0ad7a799ecb25e8abc0be0e2c
```

3. Keep the address it printed:

```bash
export RECEIVER=0x890EdF334Fa33373ec945Bdd757C55DfB4e038e6
```

## 2. Send the call

Sending is a single call to `sendCall` on the source chain's GMP contract:

```solidity
struct SendCallMsg {
    string sourceClient;      // the client to send over
    string receiver;          // destination contract, as a string
    bytes salt;               // used to create multiple accounts from the same sender
    bytes payload;            // call data for the receiver
    uint64 timeoutTimestamp;  // unix seconds, at most one day out
    string memo;              // a memo
}
```

The four steps below build one packet on chain A. It carries a call to `set(42)`, addressed to the `Remote` contract you deployed on chain B.

1. Encode the call to be made as the payload:

```bash
export PAYLOAD=$(cast calldata "set(uint256)" 42)
```

2. Read the source chain's clock and pick a timeout an hour out:

```bash
export TIMEOUT=$(( $(cast block latest --rpc-url $RPC_A --field timestamp) + 3600 ))
```

3. Dry-run the send:

```bash
cast call $GMP_A "sendCall((string,string,bytes,bytes,uint64,string))(uint64)" "($CLIENT_A,$RECEIVER,0x,$PAYLOAD,$TIMEOUT,\"\")" --rpc-url $RPC_A --from $SENDER
```

The number it returns is the sequence the packet would get. 

4. Send the source transaction, keeping the hash so you can relay it:

```bash
export TX=$(cast send $GMP_A "sendCall((string,string,bytes,bytes,uint64,string))(uint64)" "($CLIENT_A,$RECEIVER,0x,$PAYLOAD,$TIMEOUT,\"\")" --rpc-url $RPC_A --private-key $KEY --json | jq -r .transactionHash)
```

```bash
cast receipt $TX status --rpc-url $RPC_A
```

## 3. Relay the packet

Call the relayer to relay the packet:

1. Name the chain the transaction is on and the transaction:

```bash
./bin/ibc relayer relay --chain-id 41001 --tx-hash $TX
```

```json
{
  "packets": [
    {
      "source_client_id": "link-41001-41002",
      "sequence_number": "2",
      "selection": "PACKET_SELECTION_SELECTED"
    }
  ]
}
```

2. Watch it settle. This may take a few seconds:

```bash
./bin/ibc relayer packets --chain-id 41001 --tx-hash $TX
```

Success looks like this:

```json
{
  "packets": [
    {
      "state": "PACKET_STATE_SUCCEEDED",
      "sequence_number": "2",
      "source_client_id": "link-41001-41002",
      "send_tx": {
        "tx_hash": "0x832cb445e9bba060ed1794deef26258986ea7c5b8719d35f0df3bf00d09b8c95",
        "chain_id": "41001"
      },
      "recv_tx": {
        "tx_hash": "0xfe6b97036fe6881d4d4f08f11a0bbd85eb74432c4a82bb8249b6d33fd2605520",
        "chain_id": "41002"
      },
      "ack_tx": {
        "tx_hash": "0x6aaa4665369f3224da4e4c10192742515be49b5f1a3fc3f05de89026164fcc45",
        "chain_id": "41001"
      },
      "timeout_tx": null
    }
  ],
  "has_more": false,
  "next_cursor": ""
}
```

## Verify the call landed

Now verify the state changed on the destination contract:

```bash
cast call $RECEIVER "value()(uint256)" --rpc-url $RPC_B
```

```
42
```

Then read the caller:

```bash
cast call $RECEIVER "lastCaller()(address)" --rpc-url $RPC_B
```

```
0x82589a599B4b5cf636f80242509Ef821519560cD
```

That is your account on the destination chain, derived from the destination chain's client identifier, your sender string, and the salt. 

This can also be computed ahead of time by calling the destination's GMP contract:

```bash
cast call $GMP_B "getOrComputeAccountAddress((string,string,bytes))(address)" "($CLIENT_B,$SENDER,0x)" --rpc-url $RPC_B
```

```
0x82589a599B4b5cf636f80242509Ef821519560cD
```

The mapping works in reverse too, which is how a receiving contract identifies who called it:

```bash
cast call $GMP_B "getAccountIdentifier(address)((string,string,bytes))" 0x82589a599B4b5cf636f80242509Ef821519560cD --rpc-url $RPC_B
```

```
("link-41001-41002", "0x58A57ed9d8d624cBD12e2C467D34787555bB1b25", 0x)
```

The account is a contract that is deployed on the first inbound call for that identifier.

## Contract acknowledgement callbacks

When sending from a contract, the sender can receive the acknowledgement by implementing two callback functions:

```solidity
function onAckPacket(bool success, IIBCAppCallbacks.OnAcknowledgementPacketCallback calldata msg_) external;

function onTimeoutPacket(IIBCAppCallbacks.OnTimeoutPacketCallback calldata msg_) external;
```

Both structs carry the two client identifiers, the sequence, the original payload, and the relayer's address. The acknowledgement path adds the raw acknowledgement bytes. Those decode as a `GMPAcknowledgement` when `success` is true, and carry a fixed error constant when it is not. 

- GMP only calls back if your contract advertises that it can receive callbacks. You can do that by inheriting `IBCCallbackReceiver`.
- Each `sendCall` returns a sequence number. Save it if you need to match an acknowledgement to the call that caused it.

