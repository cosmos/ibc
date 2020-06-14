The section specifies packet data structure and state machine handling logic for the transfer of fungible tokens over an IBC channel between two modules on separate ledgers. The state machine logic presented allows for safe multi-ledger denomination handling with permissionless channel opening. This logic constitutes a "fungible token transfer bridge module", interfacing between the IBC routing module and an existing asset tracking module on the host ledger.

\vspace{3mm}

### Motivation

&nbsp;

Users of a set of ledgers connected over the IBC protocol might wish to utilise an asset issued on one ledger on another ledger, perhaps to make use of additional features such as exchange or privacy protection, while retaining fungibility with the original asset on the issuing ledger. This application-layer protocol allows for transferring fungible tokens between ledgers connected with IBC in a way which preserves asset fungibility, preserves asset ownership, limits the impact of Byzantine faults, and requires no additional permissioning.

\vspace{3mm}

### Properties

- Preservation of fungibility (two-way peg)
- Preservation of total supply (constant or inflationary on a single source ledger and module)
- Permissionless token transfers, no need to whitelist connections, modules, or denominations
- Symmetric (all ledgers implement the same logic)
- Fault containment: prevents Byzantine-inflation of tokens originating on ledger A, as a result of ledger B's Byzantine behaviour (though any users who sent tokens to ledger B may be at risk)

\vspace{3mm}

### Packet definition

&nbsp;

Only one packet data type, `FungibleTokenPacketData`, which specifies the denomination, amount, sending account, receiving account, and whether the sending ledger is the source of the asset, is required:

```typescript
interface FungibleTokenPacketData {
  denomination: string
  amount: uint256
  sender: string
  receiver: string
}
```

The acknowledgement data type describes whether the transfer succeeded or failed, and the reason for failure (if any):


```typescript
interface FungibleTokenPacketAcknowledgement {
  success: boolean
  error: Maybe<string>
}
```

\vspace{3mm}

### Packet handling semantics

&nbsp;

The protocol logic is symmetric, so that denominations originating on either ledger can be converted to vouchers on the other, and then redeemed back again later.

- When acting as the source ledger, the bridge module escrows an existing local asset denomination on the sending ledger and mints vouchers on the receiving ledger.
- When acting as the sink ledger, the bridge module burns local vouchers on the sending ledgers and unescrows the local asset denomination on the receiving ledger.
- When a packet times-out, local assets are unescrowed back to the sender or vouchers minted back to the sender appropriately.
- Acknowledgement data is used to handle failures, such as invalid denominations or invalid destination accounts. Returning
  an acknowledgement of failure is preferable to aborting the transaction since it more easily enables the sending ledger
  to take appropriate action based on the nature of the failure.

This implementation preserves both fungibility and supply. If tokens have been sent to the counterparty ledger, they can be redeemed back in the same denomination and amount on the source ledger.
The combined supply of unlocked tokens of a particular on both ledgers is constant, since each send-receive packet pair locks and mints the same amount (although the source ledger of a particular
asset could change the supply outside of the scope of this protocol).

\vspace{3mm}

#### Multi-ledger notes

This specification does not directly handle the "diamond problem", where a user sends a token originating on ledger A to ledger B, then to ledger D, and wants to return it through the path `D -> C -> A` — since the supply is tracked as owned by ledger B (and the voucher denomination will be `"{portD}/{channelD}/{portB}/{channelB}/denom"`), ledger C cannot serve as the intermediary. It is not yet clear whether that case should be dealt with in-protocol or not — it may be fine to just require the original path of redemption (and if there is frequent liquidity and some surplus on both paths the diamond path will work most of the time). Complexities arising from long redemption paths may lead to the emergence of central ledgers in the network topology.

In order to track all of the denominations moving around the network of ledgers in various paths, it may be helpful for a particular ledger to implement a registry which will track the "global" source ledger for each denomination. End-user service providers (such as wallet authors) may want to integrate such a registry or keep their own mapping of canonical source ledgers and human-readable names in order to improve UX.
