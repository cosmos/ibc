---
ics: 31
title: Cross-chain Queries
stage: draft
category: IBC/APP
requires: 2, 5, 18, 23, 24
kind: instantiation
author: Joe Schnetzler <schnetzlerjoe@gmail.com>, Manuel Bravo <manuel@informal.systems>
created: 2022-01-06
modified: 2022-07-28
---

## Synopsis

This standard document specifies the data structures and state machine handling logic of the Cross-chain Queries module, which allows for cross-chain querying between IBC enabled chains.

## Overview and Basic Concepts

### Motivation

We expect on-chain applications to depend on reads from other chains, e.g., a particular application on a chain may need to know the current price of the token of a second chain. While the IBC protocol enables on-chain applications to talk to other chains, using it for simply querying the state of chains would be too expensive: it would require to maintain an open channel between the querying chain and any other chain, and use the full IBC stack for every query request. Note that the latter implies exchanging packets between chains and therefore committing transactions at the queried chain, which may disrupt its operation if the load of query requests is high. Cross-chain queries solve this issue. It enables on-chain applications to query the state of other chains seamlessly: without involving the queried chain, and requiring very little from the querying chain.

### Definitions 

`Querying chain`: The chain that is interested in getting data from another chain (queried chain). The querying chain is the chain that implements the Cross-chain Queries module.

`Queried chain`: The chain whose state is being queried. The queried chain gets queried via a relayer utilizing its RPC client which is then submitted back to the querying chain.

`Cross-chain Queries Module`: The module that implements the cross-chain querying protocol. Only the querying chain integrates it.

`Height` and client-related functions are as defined in ICS 2.

`newCapability` and `authenticateCapability` are as defined in ICS 5.

`CommitmentPath` and `CommitmentProof` are as defined in ICS 23.

`Identifier`, `get`, `set`, `delete`, `getCurrentHeight`, and module-system related primitives are as defined in ICS 24.

`Fee` is as defined in ICS 29.

## System Model and Properties

### Assumptions

- **Safe chains:** Both the querying and queried chains are safe. This means that, for every chain, the underlying consensus engine satisfies safety (e.g., the chain does not fork) and the execution of the state machine follows the described protocol.

- **Live chains:** Both the querying and queried chains MUST be live, i.e., new blocks are eventually added to the chain.

- **Censorship-resistant querying chain:**  The querying chain cannot selectively omit valid transactions.

> For example, this means that if a relayer submits a valid transaction to the querying chain, the transaction is guaranteed to be eventually included in a committed block. Note that Tendermint does not currently guarantee this.

- **Correct relayer:** There is at least one live relayer between the querying and queried chains where the relayer correctly follows the protocol.

> In the context of this specification, this implies that for every query request coming from the querying chain, there is at least one relayer that (i) picks the query request up, (ii) executes the query at the queried chain, and (iii) submits the result in a transaction, together with a valid proof, to the querying chain.

The above assumptions are enough to guarantee that the query protocol returns results to the application if the querying chain waits unboundly for query results. Nevertheless, this specification considers the case when the querying chain times out after a fixed period of time. Thus, to guarantee that the query protocol always returns query results to the application, the 
specification requires additional assumptions: both the querying chain and at least one correct relayer have to behave timely.

- **Timely querying chain:** There exists an upper-bound in the time elapsed between the moment a transaction is submitted to the chain and when the chain commits a block including it.

- **Timely relayer:** For correct and live relayers, there exists an upper-bound in the time elapsed between the moment a relayer picks a query request and when the relayer submits the query result.

> Note then that to guarantee that the query protocol always returns results to the application, the timeout bound at the querying chain should be at least equal to the sum of the upper-bounds of assumptions **Timely querying chain** and **Timely relayer**. This would guarantee that the relayer submits and the querying chain process a query result transaction within the specified timeout bound.

### Desired Properties

#### Permissionless

The querying chain can query a chain without permission from the latter and implement cross-chain querying without any approval from a third party or chain governance. Note that since there is no prior negotiation between chains, the querying chain cannot assume that queried data will be in an expected format.

#### Minimal queried chain Work

Any chain that provides query support can act as a queried chain, requiring no implementation work or any extra module. This is possible by utilizing an RPC client on a relayer.

#### Modular

Supporting cross-chain queries should be as easy as implementing a module in your chain.

#### Incentivization

A bounty is paid to incentivize relayers for participating in cross-chain queries: fetching data from the queried chain and submitting it (together with proofs) to the querying chain.

## Technical Specification

### General Design 

The querying chain must implement the Cross-chain Queries module, which allows the querying chain to query state at the queried chain. 

Cross-chain queries rely on relayers operating between both chains. When a query request is received by the querying chain, the Cross-chain Queries module emits a `sendQuery` event. Relayers operating between the querying and queried chains must monitor the querying chain for `sendQuery` events. Eventually, a relayer will retrieve the query request and execute it, i.e., fetch the data and generate the corresponding proofs, at the queried chain. The relayer then submits the result in a transaction to the querying chain. The result is finally registered at the querying chain by the Cross-chain Queries module. 

A query request includes the height of the queried chain at which the query must be executed. The reason is that the keys being queried can have different values at different heights. Thus, a malicious relayer could choose to query a height that has a value that benefits it somehow. By letting the querying chain decide the height at which the query is executed, we can prevent relayers from affecting the result data.

> Note that this mechanism does not prevent cross-chain MEV (maximal extractable value): this still creates an opportunity for altering the state on the queried chain if the height is in the future in order to change the results of the query.

### Data Structures

The Cross-chain Queries module stores query requests when it processes them. 

A CrossChainQuery is a particular interface to represent query requests. A request is retrieved when its result is submitted.

```typescript
interface CrossChainQuery {
    id: Identifier
    path: CommitmentPath
    localTimeoutHeight: Height
    localTimeoutTimestamp: uint64
    queryHeight: Height
    clientId: Identifier
    bounty: Fee
}
```

- The `id` field uniquely identifies the query at the querying chain.
- The `path` field is the path to be queried at the queried chain.
- The `localTimeoutHeight` field specifies a height limit at the querying chain after which a query is considered to have failed and a timeout result should be returned to the original caller.
- The `localTimeoutTimestamp` field specifies a timestamp limit at the querying chain after which a query is considered to have failed and a timeout result should be returned to the original caller.
- The `queryHeight` field is the height at which the relayer must query the queried chain
- The `clientId` field identifies the querying chain's client of the queried chain.
- The `bounty` field is a bounty that is given to the relayer for participating in the query.

The Cross-chain Queries module stores query results to allow query callers to asynchronously retrieve them. 
In this context, this standard defines the `QueryResult` type as follows:

```typescript
enum QueryResult {
  SUCCESS,
  FAILURE,
  TIMEOUT
}
```

- A query that returns a value is marked as `SUCCESS`. This means that the query has been executed at the queried chain and there was a value associated to the queried path at the requested height.
- A query that is executed but does not return a value is marked as `FAILURE`. This means that the query has been executed at the queried chain, but there was no value associated to the queried path at the requested height.
- A query that timed out before a result is committed at the querying chain is marked as `TIMEOUT`.

A `CrossChainQueryResult` is a particular interface used to represent query results.

```typescript
interface CrossChainQueryResult {
    id: Identifier
    result: QueryResult
    data: []byte
}
```

- The `id` field uniquely identifies the query at the querying chain.
- The `result` field indicates whether the query was correctly executed at the queried chain and if the queried path exists.
- The `data` field is an opaque bytestring that contains the value associated with the queried path in case `result = SUCCESS`.

### Store paths

#### Query path

The query path is a private path that stores the state of ongoing cross-chain queries.

```typescript
function queryPath(id: Identifier): Path {
    return "queries/{id}"
}
```

#### Result query path

The result query path is a private path that stores the result of completed queries.

```typescript
function queryResultPath(id: Identifier): Path {
    return "result/queries/{id}"
}
```

### Helper functions

The querying chain MUST implement a function `generateIdentifier`, which generates a unique query identifier:

```typescript
function generateIdentifier = () -> Identifier
```

### Sub-protocols

#### Query lifecycle

1) When the querying chain receives a query request, it calls `CrossChainQueryRequest` of the Cross-chain Queries module. This function generates a unique identifier for the query, stores it in its `privateStore` and emits a `sendQuery` event. Query requests can be submitted as transactions to the querying chain or simply executed as part of the `BeginBlock` and `EndBlock` logic. Typically, query requests will be issued by other IBC modules.
2) A correct relayer listening to `sendQuery` events from the querying chain will eventually pick the query request up and execute it at the queried chain. The result is then submitted in a transaction to the querying chain.
3) When the query result is committed at the querying chain, this calls the `CrossChainQueryResponse` function of the Cross-chain Queries module.
4) The `CrossChainQueryResponse` first retrieves the query from the `privateStore` using the query's unique identifier. It then proceeds to verify the result using its local client. If it passes the verification, the function removes the query from the `privateStore` and stores the result in the private store.

> The querying chain may execute additional state machine logic when a query result is received. To account for this additional state machine logic and charge a fee to the query caller, an implementation of this specification could use the already existing `bounty` field of the `CrossChainQuery` interface or extend the interface with an additional field.

5) The query caller can then asynchronously retrieve the query result. The function `PruneCrossChainQueryResult` allows a query caller to prune the result from the store once it retrieves it.

#### Normal path methods

The `CrossChainQueryRequest` function is called when the Cross-chain Queries module at the querying chain receives a new query request.

```typescript
function CrossChainQueryRequest(
  path: CommitmentPath,
  queryHeight: Height,
  localTimeoutHeight: Height,
  localTimeoutTimestamp: uint64,
  clientId: Identifier,
  bounty: Fee,
  ): [Identifier, CapabilityKey] {

    // Check that there exists a client of the queried chain. The client will be used to verify the query result.
    abortTransactionUnless(queryClientState(clientId) !== null)

    // Sanity-check that localTimeoutHeight is 0 or greater than the current height, otherwise the query will always time out.
    abortTransactionUnless(localTimeoutHeight === 0 || localTimeoutHeight > getCurrentHeight())
    // Sanity-check that localTimeoutTimestamp is 0 or greater than the current timestamp, otherwise the query will always time out.
    abortTransactionUnless(localTimeoutTimestamp === 0 || localTimeoutTimestamp > currentTimestamp())

    // Generate a unique query identifier.
    queryIdentifier = generateQueryIdentifier()

    // Create a query request record.
    query = CrossChainQuery{queryIdentifier,
                            path,
                            queryHeight,
                            localTimeoutHeight,
                            localTimeoutTimestamp, 
                            clientId,
                            bounty}

    // Store the query in the local, private store.
    privateStore.set(queryPath(queryIdentifier), query)

    queryCapability = newCapability(queryIdentifier)

    // Log the query request.
    emitLogEntry("sendQuery", query)

    // Returns the query identifier.
    return [queryIdentifier, queryCapability]
}
```

- **Precondition**
  - There exists a client with `clientId` identifier.
- **Postcondition**
  - The query request is stored in the `privateStore`.
  - A `sendQuery` event is emitted.

The `CrossChainQueryResponse` function is called when the Cross-chain Queries module at the querying chain receives a new query reply.
We pass the address of the relayer that submitted the query result to the querying chain to optionally provide some rewards. This provides a foundation for fee payment, but can be used for other techniques as well (like calculating a leaderboard).

```typescript
function CrossChainQueryResponse(
  queryId: Identifier,
  data: []byte
  proof: CommitmentProof,
  proofHeight: Height,
  delayPeriodTime: uint64,
  delayPeriodBlocks: uint64,
  relayer: string
  ) {

    // Retrieve query state from the local, private store using the query's identifier.
    query = privateStore.get(queryPath(queryIdentifier))
    abortTransactionUnless(query !== null)

    // Retrieve client state of the queried chain.
    clientState = queryClientState(query.clientId)
    abortTransactionUnless(client !== null)

    // Check that the relier executed the query at the requested height at the queried chain.
    abortTransactionUnless(query.queryHeight !== proofHeight)

    // Check that localTimeoutHeight is 0 or greater than the current height.
    abortTransactionUnless(query.localTimeoutHeight === 0 || query.localTimeoutHeight > getCurrentHeight())
    // Check that localTimeoutTimestamp is 0 or greater than the current timestamp.
    abortTransactionUnless(query.localTimeoutTimestamp === 0 || query.localTimeoutTimestamp > currentTimestamp()) 


    // Verify query result using the local light client of the queried chain.
    // If the response carries data, then verify that the data is indeed the value associated with query.path at query.queryHeight at the queried chain.
    if (data !== null) {    
        abortTransactionUnless(verifyMembership(
            clientState,
            proofHeight,
            delayPeriodTime,
            delayPeriodBlocks,
            proof,
            query.path,
            data
        ))
        result = SUCCESS
    // If the response does not carry any data, verify that query.path does not exist at query.queryHeight at the queried chain.
    } else {
        abortTransactionUnless(verifyNonMembership(
            clientState,
            proofHeight,
            delayPeriodTime,
            delayPeriodBlocks,
            proof,
            query.path,
        ))
        result = FAILURE
    }

    // Delete the query from the local, private store.
    privateStore.delete(queryPath(queryId))

    // Create a query result record.
    resultRecord = CrossChainQuery{queryIdentifier,
                                   result,
                                   data} 

    // Store the result in the local, private store.
    privateStore.set(queryResultPath(queryIdentifier), resultRecord)

}
```

- **Precondition**
  - There exists a client with `clientId` identifier.
  - There is a query request stored in the `privateStore` identified by `queryId`.
- **Postcondition**
  - The query request identified by `queryId` is deleted from the `privateStore`.
  - The query result is stored in the `privateStore`.

The `PruneCrossChainQueryResult` function is called when the caller of a query has retrieved the result and wants to delete it.

```typescript
function PruneCrossChainQueryResult(
  queryId: Identifier,
  queryCapability: CapabilityKey
  ) {

    // Retrieve the query result from the private store using the query's identifier.
    resultRecord = privateStore.get(queryResultPath(queryIdentifier))
    abortTransactionUnless(resultRecord !== null)

    // Abort the transaction unless the caller has the right to clean the query result
    abortTransactionUnless(authenticateCapability(queryId, queryCapability))

    // Delete the query result from the local, private store.
    privateStore.delete(queryResultPath(queryId))
}
```

- **Precondition**
  - There is a query result stored in the `privateStore` identified by `queryId`.
  - The caller has the right to clean the query result
- **Postcondition**
  - The query result identified by `queryId` is deleted from the `privateStore`.

#### Timeouts

Query requests have associated a `localTimeoutHeight` and a `localTimeoutTimestamp` field that specifies the height and timestamp limit at the querying chain after which a query is considered to have failed. 

There are several alternatives on how to handle timeouts. For instance, the relayer could submit timeout notifications as transactions to the querying chain. Since the relayer is untrusted, for each of these notifications, the Cross-chain Queries module of the querying chain MUST call the `checkQueryTimeout` to check if the query has indeed timed out. An alternative could be to make the Cross-chain Queries module responsible for checking if any query has timed out by iterating over the ongoing queries at the beginning of a block and calling `checkQueryTimeout`. In this case, ongoing queries should be stored indexed by `localTimeoutTimestamp` and `localTimeoutHeight` to allow iterating over them more efficiently. These are implementation details that this specification does not cover. 

Assume that the relayer is in charge of submitting timeout notifications as transactions. The `checkQueryTimeout` function would look as follows. Note that
we pass the relayer address just as in `CrossChainQueryResponse` to allow for possible incentivization here as well.

```typescript
function checkQueryTimeout(
    queryId: Identifier,
    relayer: string
){
    // Retrieve the query state from the local, private store using the query's identifier.
    query = privateStore.get(queryPath(queryIdentifier))
    abortTransactionUnless(query !== null)

    // Get the current height.
    currentHeight = getCurrentHeight()

    // Check that localTimeoutHeight or localTimeoutTimestamp has passed on the querying chain (locally)
    abortTransactionUnless(
      (query.localTimeoutHeight > 0 && query.localTimeoutHeight < getCurrentHeight()) ||
      (query.localTimeoutTimestamp > 0 && query.localTimeoutTimestamp < currentTimestamp()))

    // Delete the query from the local, private store if it has timed out
    privateStore.delete(queryPath(queryId))

    // Create a query result record.
    resultRecord = CrossChainQuery{queryIdentifier,
                                   TIMEOUT,
                                   query.caller
                                   null} 

    // Store the result in the local, private store.
    privateStore.set(resultQueryPath(queryIdentifier), resultRecord)
}
```

- **Precondition**
  - There is a query request stored in the `privateStore` identified by `queryId`.
- **Postcondition**
  - If the query has indeed timed out, then
    - the query request identified by `queryId` is deleted from the `privateStore`;
    - the fact that the query has timed out is recorded in the `privateStore`.

## History

January 6, 2022 - First draft

May 11, 2022 - Major revision

June 14, 2022 - Adds pruning, localTimeoutTimestamp and adds relayer address for incentivization

July 28, 2022 - Revision of the assumptions

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
