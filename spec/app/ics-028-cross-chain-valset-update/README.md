# Technical Specification

## Introduction

This document presents a technical specification for the **Cross-Chain Validation** protocol.
The basic idea of the Cross-Chain Validation protocol is to allow validators that are already managed by some existing blockchain (parent blockchain) to secure a "new" blockchain (baby blockchain).
The stake bonded at the parent blockchain guarantees that a validator behaves correctly at the baby blockchain.
Otherwise, the validator is slashed on the parent blockchain.

Therefore, at a high level, we can imagine the Cross-Chain Validation protocol to be concerned with following entities:

  - Parent blockchain: This is a blockchain that "provides" validators. Namely, "provided" validators have some stake at the parent blockchain. Any misbehavior of a validator is slashed on the parent blockchain. Moreover, the parent blockchain manipulates the validator set of a chain that "borrows" validators from it.
  - Baby blockchain: The baby blockchain is a blockchain that is being secured by the parent blockchain. In other words, validators that secure and operate the baby blockchain are bonded on the parent blockchain. Any misbehavior of a validator at the baby blockchain is punished by the parent blockchain (i.e., the validator is slashed on the parent blockchain).

Note that the protocol we present is generalized to multiple baby blockchains.
In other words, a single parent blockchain might have multiple baby blockchains under its "jurisdiction".
Hence, the Cross-Chain Validation protocol is concerned with more than two entities (one parent blockchain and potentially multiple baby blockchains).

### Properties

This subsection is devoted to defining properties that the Cross-Chain Validation protocol ensures.
Recall that the parent blockchain has an ability to demand a change to the validator set of the baby chain.
For time being, this is the only way the validator set of the baby blockchain can be modified (i.e., the baby blockchain does not make any changes to the validator set on its own).

We present the interface of the protocol:

  - Request: \<ChangeValidatorSet, babyChain, V\> - request made by the parent blockchain to change the validator set of the blockchain babyChain using the validator set change V.
  We assume that each validator set change is unique (e.g., each validator set change V could simply have a unique identifier).
  - Indication: \<Mature, babyChain, V\> - indication to the parent blockchain that the validator set change V (i.e., its "effect") has "matured" on the baby blockchain.

A brief explanation: a validator set change V is said to has matured once the unbonding period of 3 weeks has elapsed on the baby blockchain "for that validator set change".
We provide more details on this in the rest of the document.

We aim to achieve the following properties:

- *Liveness*: If the parent blockchain demands a validator set change V for the baby blockchain, then the validator set of the baby blockchain eventually reflects this demand.
- *Validator set change safety*: Suppose that the validator set of the baby blockchain changes from *V* to *V'*. Then, there exists a sequence *seq = V1, V2, ..., Vn*, where *n > 0*, of change validator set demands issued by the parent blockchain such that *apply(V, seq) = V'*.
- *Validator set change liveness*: If \<ChangeValidatorSet, babyChain, V\> is not the last validator set change request issued by the parent blockchain to the blockchain babyChain, then \<Mature, babyChain, V\> is eventually triggered at the parent blockchain.

## High-level design of the Cross-Chain Validation protocol

In this section, we provide an intuition behind our protocol.

We use IBC channels for communication between two blockchains (the parent and the baby blockchain).
Namely, the parent blockchain sends a validator set change via IBC channel to the baby blockchain.
Moreover, the acknowledgement of the received validator set change by the baby blockchain means that the change has matured.
Note that the parent blockchain has the full responsibility of deducing which validators are free to take their money back.
In other words, we do not specify how the stake of validators of the baby blockchain are managed.
However, we devote the last section of the document to this.

Short summary of the protocol:

- Parent blockchain: Once the parent blockchain demands a change of validator set, it sends the validator set change to the baby blockchain via an IBC channel.

- Baby blockchain: Once the baby blockchain receives the validator set change, the baby blockchain applies this change.
Moreover, whenever the validator set of the baby blockchain is modified, the "old" validator set change starts "maturing".
That is, the packet with the old validator set change is **acknowledged** in 3 weeks time to signal that the "unbonding period" has elapsed for this validator set change on the baby blockchain.

## Data Structures

We devote this section to defining the data structures used to represent the states of the baby blockchain, as well as the packets exchanged by the two blockchains.

### Application data

#### Baby blockchain

- MaturingQueue: Keeps track of validator set changes that are currently maturing.
- lastObserved: Packet of the IBC channel that was observed during the last read of the content of the channel; initializated to nil.

#### Packets

- ChangeValidatorSet(V): Packet that contains the validator set change for the baby blockchain.

## Transitions

In this section, we informally discuss the state transitions that occur in our protocol.
We observe state transitions that are driven by a user (i.e., the new staking module on the parent chain), driven by the relayer and driven by elapsed time.

  - User-driven state transitions: These transitions "start" the entire process of changing the validator set of the baby blockchain.
  We assume that the will to change the validator set of the baby blockchain babyChain is expressed by invoking \<ChangeValidatorSet, babyChain, V\>.

  - Time-driven state transitions: These transitions are activated since some time has elapsed. As we will present in the rest of the document, time-driven state transitions help us determine when the maturing period, measured at the baby blockchain, has elapsed for a validator set change.

In the rest of the document, we will discuss the aforementioned state transitions in more detail.

## Function Definitions

### Parent blockchain

This subsection will present the functions executed at the parent blockchain.

```golang
// expresses will to modify the validator set of the baby blockchain;
func changeValidatorSet(
  babyChainId: ChainId
  valSetUpdate: Validator[]
) {
	// create the ChangeValidatorSet packet
  	ChangeValidatorSet data = ChangeValidatorSet{valSetUpdate}

  	// obtain the destination port of the baby blockchain
  	destPort = getPort(babyChainId)

  	// send the packet
  	handler.sendPacket(Packet{timeoutHeight, timeoutTimestamp, destPort, destChannel, sourcePort, sourceChannel, data}, getCapability("port"))
}
```

- Expected precondition
  - There exists a blockchain with *babyChainId* identifier
  - All validators from *valSetUpdate* are managed by the parent blockchain
- Expected postcondition
  - The ChangeValidatorSet packet is created
- Error condition
  - If the precondition is violated

```golang
func onAcknowledgePacket(packet: Packet) {
  // the packet is of ChangeValidatorSet type
  assert(packet.type = ChangeValidatorSet)
  
  trigger (Mature, packet.receiver, packet.validatorSetUpdate)
}
```

- Expected precondition
  - The packet is of the *ChangeValidatorSet* type
- Expected postcondition
  - The indication Mature is triggered
- Error condition
  - If the precondition is violated

### Baby blockchain

```golang
// initialization function
func init() {
  MaturingQueue = new local FIFO queue
  lastObserved = nil
}
```


```golang
// function used in End-Block method
// returns all the packets added to the IBC channel since the last invocation of this method
// we assume that IBC channel could be modelled as a queue
func observeChanges() { 
  // get the content of the IBC channel
  content = channel.read()

  // check whether the last observed element is in content
  if (!content.contains(lastObserved)) {
    // update the lastObserved to the last element of content if content is not empty
    if (!content.empty()) lastObserved = content.last()

    // return the entire content
    return content
  } else {
    // set "old" lastObserved
    oldLastObserved = lastObserved

    // update the lastObserved to the last element of content
    lastObserved = content.last()

    // return the entire content starting from oldLastObserved (excluding oldLastObserved)
    return content.startFrom(oldLastObserved)
  }
}
```

- Expected precondition
  - None
- Expected postcondition
  - Returns the packets added to the IBC channel since the last observeChanges() operation was invoked.
- Error condition
  - None

```golang
// End-Block method executed at the end of each block
func endBlock(block: Block) {
  // get time
  time = block.time

  // This is nothing more than a simple timer for a packet; namely, this piece of code ensures
  // that the packet is acknowledged in 3 weeks time
  
  // finish maturing for mature validator set changes
  while (!MaturingQueue.isEmpty()) {
    // peak the first queue entry
    startTime = MaturingQueue.peak()

    if (startTime + UNBONDING_PERIOD >= time) {
      // remove from the maturing queue
      startTime = MaturingQueue.dequeue()

      // acknowledge the packet
      acknowledgeTheFirstUnacknowledgedPacketOfTheChannel();

    } else {
      break
    }
  }

  // get the new changes sent by the parent
  changes = observeChanges()

  // if there are no changes, return current validator set
  if (changes.empty()) {
    // validator set remains the same
    return nil
  }

  // get the old validator set
  oldValSet = block.validatorSet

  // get the new validator set; init to the old one
  newValSet = oldValSet

  // start maturing for the old validator set change;
  // means "Acknowledge 3 weeks from now"
  MaturingQueue.enqueue(time)

  // update the validator set
  while (!changes.isEmpty()) {
    valSetUpdate = changes.dequeue()

    // update the new validator set
    newValSet = applyValidatorUpdate(newValSet, valSetUpdate)

    if (!changes.isEmpty()) {
      // start maturing previously seen validtor set change
      MaturingQueue.enqueue(time)
    }
  }

  ABCI.updateValidatorSet(newValSet - oldValSet)
}
```

- Expected precondition
  - Every transaction from the *block* is executed
- Expected postcondition
  - Maturing starts for the old validator set change
  - Maturing finishes for all validator set changes that started maturing more than 3 weeks before *time = block.time*. 
  - The new validator set *newValSet* is pushed to the Tendermint protocol and *newValSet* reflects all the change validator set demands from the *block*
- Error condition
  - If the precondition is violated


## Correctness Arguments

Here we provide correctness arguments for the liveness, validator set change safety and liveness properties.

### Liveness
Suppose that the IBC communication indeed successfully relays the change validator set demand to the baby blockchain.
Therefore, the validator set of the baby blockchain eventually reflects this demand.
This indeed happens at the end of a block, since every observed demand is applied in order for the baby blockchain to calculate the new validator set.
Hence, the property is satisfied.

### Validator set change safety
Suppose that the validator set of the baby blockchain changes from *V* to *V'* in two consecutive blocks.
By construction of the protocol, we conclude that there exists a sequence of change validator set demands issued by the parent blockchain that result in *V'* when applied to *V*.
Recursively, we conclude that this holds for any two validator sets (irrespectively of the "block distance" between them).

### Validator set change liveness

Since the demand V is not the last issued demand, we conclude that eventually the maturing period is started for V.
The maturing period eventually elapses and the V is acknowledged at that moment.
As soon as the acknowledgement is "observed" by the parent blockchain, the indication is triggered.

## How Can The Parent Blockchain Know Which Validators Are Unbonded - Discussion

As we have shown thus far, we provide slightly different notion of unbonding.
Namely, the baby blockchain simply informs the parent blockchain when a validator set change has become stale (i.e., has matured).
Then, the parent blockchain has the responsibility of discovering which validators could take their money back.
We now give some insights into this.

Let the parent blockchain maintain a local queue IssuedChanges that, at the beginning, contains a single validator set change that is identical to the initial validator set of the baby blockchain.
Whenever a new validator set change is sent to the baby blockchain, that validator set change is enqueued to the IssuedChanges queue.
Lastly, whenever a validator set change packet is acknowledged, the validator set change is dequeued from the IssuedChanges queue.
Let us take a closer look at the IssuedChanges queue.

Remark: If the IssuedChanges queue has a single element V, then the parent blockchain **knows** the validator set of the baby blockchain.
Indeed, this means that no validator set change is issued after V and all validator set changes issued before V are acknowledged.
Hence, the remark holds.

Similarly, if the IssuedChanges queue contains more than a single element, the parent blockchain cannot know the validator set of the baby blockchain.

Hence, the following mechanism for the parent blockchain to discover which validators can take their money back:

- If IssuedChanges.size() = 1, then the validator set of the baby blockchain is known and it is clear which validators have unbonded.
- If IssuedChanges.size() > 1, then the situation is slightly more complicated:
	- No validator that is "added" by **any** validator set change in IssuedChanges can be given money back. The reason is that this validator set change could have indeed taken place at the baby blockchain.
	- However, if there is a validator set change in IssuedChanges that "removes" a validator, this validator cannot take its money back. The reason is that this validator set change might not have reached the baby blockchain yet.

Hence, the parent blockchain needs to take into account the worst possible scenario in order to remain perfectly secure throughout the execution.




