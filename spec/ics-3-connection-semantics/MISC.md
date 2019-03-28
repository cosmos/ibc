(reference ICS 2: consensus primitives)

To facilitate an IBC connection, the two blockchains must provide the following proofs:

1. Given a trusted `H_h` and `C_h` and an attributable update message `U_h`,  
   it is possible to prove `H_h'` where `C_h' == C_h` and `dt(now, H_h) < P`
2. Given a trusted `H_h` and `C_h` and an attributable change message `X_h`,  
   it is possible to prove `H_h'` where `C_h' /= C_h` and `dt(now, H_h) < P`
3. Given a trusted `H_h` and a Merkle proof `M_kvh` it is possible to prove `V_kh`

It is possible to make use of the structure of BFT consensus to construct extremely lightweight and provable messages `U_h'` and `X_h'`. The implementation of these requirements with Tendermint consensus is defined in [Appendix E](appendices.md#appendix-e-tendermint-header-proofs). Another algorithm able to provide equally strong guarantees (such as Casper) is also compatible with IBC but must define its own set of update and change messages.

The Merkle proof `M_kvh` is a well-defined concept in the blockchain space, and provides a compact proof that the key value pair `(k, v)` is consistent with a Merkle root stored in `H_h`. Handling the case where `k` is not in the store requires a separate proof of non-existence, which is not supported by all Merkle stores. Thus, we define the proof only as a proof of existence. There is no valid proof for missing keys, and we design the algorithm to work without it. 

Blockchains supporting IBC must implement Merkle proof verification:


