The section specifies packet data structure and state machine handling logic for the transfer of fungible tokens over an IBC channel between two modules on separate ledgers. The state machine logic presented allows for safe multi-ledger denomination handling with permissionless channel opening. This logic constitutes a "fungible token transfer bridge module", interfacing between the IBC routing module and an existing asset tracking module on the host ledger.

\vspace{3mm}

### Motivation

\vspace{3mm}

Users of a set of ledgers connected over the IBC protocol might wish to utilise an asset issued on one ledger on another ledger, perhaps to make use of additional features such as exchange or privacy protection, while retaining fungibility with the original asset on the issuing ledger. This application-layer protocol allows for transferring fungible tokens between ledgers connected with IBC in a way which preserves asset fungibility, preserves asset ownership, limits the impact of Byzantine faults, and requires no additional permissioning.

\vspace{3mm}

### Properties

\vspace{3mm}

- Preservation of fungibility (two-way peg)
- Preservation of total supply (constant or inflationary on a single source ledger and module)
- Permissionless token transfers, no need to whitelist connections, modules, or denominations
- Symmetric (all ledgers implement the same logic)
- Fault containment: prevents Byzantine-inflation of tokens originating on ledger A, as a result of ledger B's Byzantine behaviour (though any users who sent tokens to ledger B may be at risk)

\vspace{3mm}

### Packet definition

\vspace{3mm}

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

\vspace{3mm}

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

### Fault containment

\vspace{3mm}

Ledgers could fail to follow the fungible transfer token protocol outlined here in one of two ways: the full nodes running the consensus algorithm could diverge from the light client, or the ledger's state machine could incorrectly implement the escrow & voucher logic (whether inadvertently or intentionally). Consensus divergence should eventually result in evidence of misbehaviour which can be used to freeze the client, but may not immediately do so (and no guarantee can be made that such evidence would be submitted before more packets), so from the perspective of the protocol's goal of isolating faults these cases must be handled in the same way. No guarantees can be made about asset recovery — users electing to transfer tokens to a ledger take on the risk of that ledger failing — but containment logic can easily be implemented on the interface boundary by tracking incoming and outgoing supply of each asset, and ensuring that no ledger is allowed to redeem vouchers for more tokens than it had initially escrowed. In essence, particular channels can be treated as accounts, where a module on the other end of a channel cannot spend more than it has received. Since isolated Byzantine sub-graphs of a multi-ledger fungible token transfer system will be unable to transfer out any more tokens than they had initially received, this prevents any supply inflation of source assets, and ensures that users only take on the consensus risk of ledgers they intentionally connect to.

\vspace{3mm}

### Multi-ledger transfer paths

\vspace{3mm}

This protocol does not directly handle the "diamond problem", where a user sends a token originating on ledger A to ledger B, then to ledger D, and wants to return it through the path `D -> C -> A` — since the supply is tracked as owned by ledger B (and the voucher denomination will be `"{portD}/{channelD}/{portB}/{channelB}/denom"`), ledger C cannot serve as the intermediary. This is necessary due to the fault containment desiderata outlined above. Complexities arising from long redemption paths may lead to the emergence of central ledgers in the network topology or automated markets to exchange assets with different redemption paths.

In order to track all of the denominations moving around the network of ledgers in various paths, it may be helpful for a particular ledger to implement a registry which will track the "global" source ledger for each denomination. End-user service providers (such as wallet authors) may want to integrate such a registry or keep their own mapping of canonical source ledgers and human-readable names in order to improve UX.
