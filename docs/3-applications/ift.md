---
title: "IFT: how it works"
description: "A fungible token that moves between chains by burning it on one and minting it on the other, controlled by its issuer on every chain."
---

An Interchain Fungible Token (IFT) is a fungible token that moves between chains by burning it on one and minting it on the other. An issuer deploys an IFT contract on every chain where they want the asset to exist. Each deployment is an ERC20 token in IBC-Solidity. Unlike an escrow bridge, which locks the asset on one chain and issues a wrapped claim on another, an IFT burns on the source and mints the issuer's own token on arrival to the destination. Nothing has to be redeemed, and no wrapper has to be unwound. A holder on any of those chains holds the issuer's real asset, and it is the same asset however many chains it has crossed.

IFT deployments have to be linked before tokens can move between them. Each side's authority registers the other as a bridge. A deployment mints for an arriving transfer only when it has registered that counterparty IFT contract. Registering is a privileged call, reserved to the authority, but sending is open to any holder. IFT is built on top of GMP, so the cross-chain leg travels as a [General Message Passing (GMP)](/applications/gmp) call from one deployment to the other: the source deployment burns the amount, and the counterparty deployment mints it to the receiver. A failed transfer refunds the sender, so a completed transfer leaves the total across the two chains unchanged. While a packet is in flight the amount is already burned and not yet minted, so the combined supply is temporarily lower.

## The transfer flow

A transfer starts with a holder, who calls `iftTransfer` with the client for the counterparty chain, a receiver on that chain, an amount, and optionally a timeout. The timeout is an absolute Unix timestamp in seconds, not a duration.

The source IFT contract then:

1. Validates the inputs.
2. Burns the amount from the sender.
3. Looks up the bridge for that client.
4. Builds the destination chain's mint call with the bridge's encoder.
5. Sends the call over GMP, addressed to the counterparty contract.
6. Stores a pending transfer under the packet sequence GMP returned.
7. Emits its transfer-initiated event.

The pending transfer is the refund ticket. It records the sender and the amount, keyed by client and sequence. It is the IFT contract's only record of the burned tokens, and it stands until they are minted on the destination, or reminted on the source chain after an error acknowledgement or a timeout. The packet itself is tracked separately, as a commitment in the router's store.

The call that crosses the wire is the destination chain's mint call, built on the source chain before anything is sent. For an EVM counterparty the encoder parses the receiver string the holder passed to `iftTransfer` into an address, and ABI-encodes `iftMint`:

```solidity
abi.encodeCall(IIFT.iftMint, (receiverAddr, amount))
```

That payload is what the source IFT contract hands to GMP, addressed to the counterparty contract from its bridge:

```solidity
struct GMPPacketData {
    string sender;    // the source IFT contract, checksummed hex, filled in by GMP
    string receiver;  // the counterparty IFT contract, from the bridge
    bytes salt;       // empty for IFT
    bytes payload;    // the encoded iftMint call
    string memo;      // empty for IFT
}
```

The payload says where to mint and how much, specified by the holder. The sender field says which contract asked, and GMP fills it from the caller, so a sender cannot claim to be another contract.

<Note>
The packet data's receiver is the contract GMP calls on the destination chain. The payload's receiver is the holder who ends up with the tokens.
</Note>

On the destination chain a GMP account executes the payload, an address GMP derives from three things: the local client, the sender string in the packet, and the salt. The account is what calls `iftMint`, so the destination IFT contract can ask GMP whose account is calling.

The destination IFT contract checks four things before it mints:

- The caller is a GMP account.
- A bridge exists for that account's client.
- The account's sender string matches the counterparty address in that bridge exactly.
- The salt is empty, which leaves each counterparty one account to mint from.

Those checks are why the destination contract can trust the mint. The light client verified the packet, the account's identity says who sent it, and the bridge registration says whose mint calls this contract accepts.

When an acknowledgement is proven on the source chain, the router hands it to GMP, which reads the universal error acknowledgement as a failure and anything else as a success. On success GMP calls the source IFT contract, which deletes the pending transfer and emits its transfer-completed event. Because it had already burned the amount, nothing further is needed, and it never reads what the acknowledgement contains.

## Minting authority

Every IFT deployment has a single authority. In IBC-solidity that authority is either one owner address or an external access manager.

The authority's powers include:

- **Bridges**: it registers and removes the records that link this deployment to its counterparty IFT contracts.
- **Local supply**: it mints to any address on its own chain.
- **The contract's code**: it authorizes upgrades.

Minting happens along only three paths:

- The authority, through the contract's own `mint`.
- The GMP account of a registered counterparty IFT contract, through `iftMint`.
- The contract itself, when GMP reports a failed transfer and it refunds the sender.

Beyond minting, a holder needs no permission. They transfer the token, burn their own balance, and start a cross-chain transfer without any authority check. A cross-chain transfer still travels through GMP, which has an authority of its own.

## Bridge registration

Two IFT contracts on different chains are bridged when each registers the other. Each side's bridge carries what the contract needs to reach the counterparty deployment, and what it needs to recognize its mint calls when they arrive.

In IBC-solidity a bridge holds three fields:

```solidity
struct IFTBridge {
    string clientId;
    string counterpartyIFTAddress;
    IIFTSendCallConstructor iftSendCallConstructor;
}
```

- **The client**: the local [client](/how-ibc-works/clients-and-counterparties) that identifies the counterparty chain.
- **The counterparty address**: the counterparty IFT contract's address on that chain, held as a string.
- **The encoder**: the component that builds mint calls in the form that chain expects. Registration rejects an encoder that is the zero address or does not advertise the encoder interface. The `abi.encodeCall` above is what the EVM encoder produces; another chain's encoder produces that chain's call format.

Both counterparty IFT contracts must register. The source contract reads its bridge to find the counterparty, so a send with no bridge fails on the source chain. The destination contract mints only for a counterparty it has registered itself. A mint with no bridge fails on the destination chain, and the transfer comes back to its sender as a refund.

Bridges are keyed by client. An IFT contract can therefore reach one counterparty contract on each client it has. Registering the same client again replaces the previous entry.

Removing a bridge stops new sends on that client. Incoming mints on that client fail from then on. Transfers this contract already sent still complete or refund, because neither path reads the bridge.

For the full contract surface and the encoders that ship with it, see [IFT contracts](/ibc-solidity-contracts/ift-contracts).

## Failed transfers and refunds

No amount is minted on the destination chain until the light client has verified that the source chain committed the packet. What that proof establishes is the commitment, not the burn. The burn is tied to it by the source contract, which burns and commits in a single transaction, and by the destination's check that the call comes from the registered counterparty's account. Together they mean an accepted mint always has a burn behind it.

A transfer fails in one of two ways, and both end in the same refund:

1. The destination refuses to mint. Its [router](/how-ibc-works/core-router-and-store) catches the revert, keeps the packet receipt, and writes the universal error acknowledgement in place of a result. The source IFT contract refunds once a relayer proves that acknowledgement.
2. The packet is never received. A relayer proves that the destination has no receipt for the packet, at a height whose verified destination timestamp is past the packet's timeout, and the source contract refunds.

For refunds, the source contract then mints the pending amount back to the original sender, deletes the pending transfer, and emits its refunded event.

A refund cannot follow a mint that happened. Both destination outcomes write a packet receipt, the successful mint and the refused one alike, and a timeout needs proof that no receipt exists. So an amount is never minted on one chain and refunded on the other.

Each refund waits on a relayer. The amount stays burned and the record stays open until the acknowledgement or the timeout proof arrives, so a relayer that is down delays a refund rather than losing it.

A successful mint whose acknowledgement is never relayed only leaves a stale pending transfer on the source chain. Both chains already hold the right supply at that point, because the burn and the mint have both happened, and the acknowledgement only clears the record.

```mermaid
flowchart TD
    A["Holder starts a transfer"] --> B["Source contract burns the amount and stores a pending transfer"]
    B --> C["GMP carries the mint call to the counterparty contract"]
    C --> D["Counterparty contract mints"]
    C --> E["Destination refuses: error acknowledgement"]
    C --> F["Packet never received: timeout proof"]
    D --> G["Acknowledgement returns: pending transfer deleted"]
    E --> H["Source contract refunds the sender"]
    F --> H
```

