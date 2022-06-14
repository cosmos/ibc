---
ics: 31
title: Cross-chain Queries
stage: draft
category: IBC/APP
requires: 2, 5, 18, 23, 24
kind: instantiation
author: Joe Schnetzler <schnetzlerjoe@gmail.com>, Manuel Bravo <manuel@informal.systems>
created: 2022-01-06
modified: 2022-05-11
---

## Synopsis

This standard document specifies the data structures and state machine handling logic of the Cross-chain Querying module, which allows for cross-chain querying between IBC enabled chains.

## Overview and Basic Concepts

### Motivation

Interchain Accounts (ICS-27) brings one of the most important features IBC offers, cross-chain transactions (on-chain). Limited in this functionality is the querying of state from one chain, on another chain. Adding cross-chain querying via the Cross-chain Querying module gives unlimited flexibility to chains to build IBC enabled protocols around Interchain Accounts and beyond.

### Definitions 

`Querying Chain`: The chain that is interested in getting data from another chain (Queried Chain). The Querying Chain is the chain that implements the Cross-chain Querying module.

`Queried Chain`: The chain whose state is being queried. The Queried Chain gets queried via a relayer utilizing its RPC client which is then submitted back to the Querying Chain.

`Cross-chain Querying Module`: The module that implements the Cross-chain Querying protocol. Only the Querying Chain integrates it.

`Height` and client-related functions are as defined in ICS 2.

`newCapability` and `authenticateCapability` are as defined in ICS 5.

`CommitmentPath` and `CommitmentProof` are as defined in ICS 23.

`Identifier`, `get`, `set`, `delete`, `getCurrentHeight`, and module-system related primitives are as defined in ICS 24.

`Fee` is as defined in ICS 29.

## System Model and Properties

### Assumptions

- **Safe chains:** Both the Querying and Queried chains are safe. This means that, for every chain, the underlying consensus engine satisfies safety (e.g., the chain does not fork) and the execution of the state machine follows the described protocol.

- **Live chains:** Both the Querying and Queried chains MUST be live, i.e., new blocks are eventually added to the chain.

- **Censorship-resistant Querying Chain:**  The Querying Chain cannot selectively omit transactions.

- **Correct relayer:** There is at least one correct relayer between the Querying and Queried chains. This is required for liveness. 

### Desired Properties

#### Permissionless

A Querying Chain can query a chain and implement cross-chain querying without any approval from a third party or chain governance. Note that since there is no prior negotiation between chains, the Querying Chain cannot assume that queried data will be in an expected format.

#### Minimal Queried Chain Work

A Queried Chain has to do no implementation work or add any module to enable cross-chain querying. By utilizing an RPC client on a relayer, this is possible.

#### Modular

Adding cross-chain querying should be as easy as implementing a module in your chain.

#### Control Queried Data

The Querying Chain should have ultimate control over how to handle queried data. Like querying for a certain query form/type.

#### Incentivization

A bounty is paid to incentivize relayers for participating in interchain queries: fetching data from the Queried Chain and submitting it (together with proofs) to the Querying

## Technical Specification

### General Design 

The Querying Chain MUST implement the Cross-chain Querying module, which allows the Querying Chain to query state at the Queried Chain. 

Cross-chain querying relies on relayers operating between both chains. When a query request is received by the Querying Chain, the Cross-chain Querying module emits a `sendQuery` event. Relayers operating between the Querying and Queried chains must monitor the Querying chain for `sendQuery` events. Eventually, a relayer will retrieve the query request and execute it, i.e., fetch the data and generate the corresponding proofs, at the Queried Chain. The relayer then submits (on-chain) the result at the Querying Chain. The result is finally registered at the Querying Chain by the Cross-chain Querying module.

A query request includes the height of the Queried Chain at which the query must be executed. The reason is that the keys being queried can have different values at different heights. Thus, a malicious relayer could choose to query a height that has a value that benefits it somehow. By letting the Querying Chain decide the height at which the query is executed, we can prevent relayers from affecting the result data.

### Data Structures

The Cross-chain Querying module stores query requests when it processes them. 

A CrossChainQuery is a particular interface to represent query requests. A request is retrieved when its result is submitted.

```typescript
interface CrossChainQuery struct {
    id: Identifier
    path: CommitmentPath
    localTimeoutHeight: Height
    localTimeoutTimestamp: Height
    queryHeight: Height
    clientId: Identifier
    bounty: Fee
}
```

- The `id` field uniquely identifies the query at the Querying Chain.
- The `path` field is the path to be queried at the Queried Chain.
- The `localTimeoutHeight` field specifies a height limit at the Querying Chain after which a query is considered to have failed and a timeout result should be returned to the original caller.
- The `localTimeoutTimestamp` field specifies a timestamp limit at the Querying Chain after which a query is considered to have failed and a timeout result should be returned to the original caller.
- The `queryHeight` field is the height at which the relayer must query the Queried Chain
- The `clientId` field identifies the Queried Chain.
- The `bounty` field is a bounty that is given to the relayer for participating in the query.

The Cross-chain Querying module stores query results to allow query callers to asynchronously retrieve them. 
In this context, this ICS defines the `QueryResult` type as follows:

```typescript
enum QueryResult {
  SUCCESS,
  FAILURE,
  TIMEOUT,
}
```
- A query that returns a value is marked as `SUCCESS`. This means that the query has been executed at the Queried Chain and there was a value associated to the queried path at the requested height.
- A query that is executed but does not return a value is marked as `FAILURE`. This means that the query has been executed at the Queried Chain, but there was no value associated to the queried path at the requested height.
- A query that timed out before a result is committed at the Querying Chain is marked as `TIMEOUT`.

A CrossChainQueryResult is a particular interface used to represent query results.

```typescript
interface CrossChainQueryResult struct {
    id: Identifier
    result: QueryResult
    data: []byte
}
```

- The `id` field uniquely identifies the query at the Querying Chain.
- The `result` field indicates whether the query was correctly executed at the Queried Chain and if the queried path exists.
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

The result query path is a public path that stores the result of completed queries.

```typescript
function resultQueryPath(id: Identifier): Path {
    return "queriesresult/{id}"
}
```

### Helper functions

The Querying Chain MUST implement a function `generateQueryIdentifier`, which generates a unique query identifier:

```typescript
function generateQueryIdentifier = () -> Identifier
```

### Sub-protocols

#### Query lifecycle

1) When the Querying Chain receives a query request, it calls `CrossChainQueryRequest` of the Cross-chain Querying module. This function generates a unique identifier for the query, stores it in its `privateStore` and emits a `sendQuery` event. Query requests can be submitted by other IBC modules as transactions to the Querying Chain or simply executed as part of the `BeginBlock` and `EndBlock` logic.
2) A correct relayer listening to `sendQuery` events from the Querying Chain will eventually pick the query request up and execute it at the Queried Chain. The result is then submitted (on-chain) to the Querying Chain.
3) When the query result is committed at the Querying Chain, this calls the `CrossChainQueryResult` function of the Cross-chain Querying module.
4) The `CrossChainQueryResult` first retrieves the query from the `privateStore` using the query's unique identifier. It then proceeds to verify the result using its local client. If it passes the verification, the function removes the query from the `privateStore` and stores the result in a public path.
5) The query caller can then asynchronously retrieve the query result. The function `PruneCrossChainQueryResult` allows a query caller to prune the result from the store once it retrieves it.

#### Normal path methods

The `CrossChainQueryRequest` function is called when the Cross-chain Querying module at the Querying Chain receives a new query request.

```typescript
function CrossChainQueryRequest(
  path: CommitmentPath,
  queryHeight: Height,
  localTimeoutHeight: Height,
  clientId: Identifier,
  bounty: Fee,
  ): [Identifier, CapabilityKey] {

    // Check that there exists a client of the Queried Chain. The client will be used to verify the query result.
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

The `CrossChainQueryResult` function is called when the Cross-chain Querying module at the Querying Chain receives a new query reply.
We pass the address of the relayer that submitted the query result to the Querying Chain to optionally provide some rewards. This provides a foundation for fee payment, but can be used for other techniques as well (like calculating a leaderboard).

```typescript
function CrossChainQueryResult(
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

    // Retrieve client state of the Queried Chain.
    client = queryClientState(query.clientId)
    abortTransactionUnless(client !== null)

    // Check that the relier executed the query at the requested height at the Queried Chain.
    abortTransactionUnless(query.queryHeight !== proofHeight)

    // Check that localTimeoutHeight is 0 or greater than the current height.
    abortTransactionUnless(query.localTimeoutHeight === 0 || query.localTimeoutHeight > getCurrentHeight())
    // Check that localTimeoutTimestamp is 0 or greater than the current timestamp.
    abortTransactionUnless(query.localTimeoutTimestamp === 0 || query.localTimeoutTimestamp > currentTimestamp()) 


    // Verify query result using the local light client of the Queried Chain. If success, then verify that the data is indeed the value associated with query.path at query.queryHeight at the Queried Chain. Otherwise, verify that query.path does not exist at query.queryHeight at the Queried Chain.
    if (data !== null) {    
        abortTransactionUnless(client.verifyMemership(
            client,
            proofHeight,
            delayPeriodTime,
            delayPeriodBlocks,
            proof,
            query.path,
            data
        ))
        result = SUCCESS
    } else {
        abortTransactionUnless(client.verifyNonMemership(
            client,
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

    // Store the result in a public path.
    provableStore.set(resultQueryPath(queryIdentifier), resultRecord)

}
```
- **Precondition**
  - There exists a client with `clientId` identifier.
  - There is a query request stored in the `privateStore` identified by `queryId`.
- **Postcondition**
  - The query request identified by `queryId` is deleted from the `privateStore`.
  - The query result is stored in the `provableStore`.

The `PruneCrossChainQueryResult` function is called when the caller of a query has retrieved the result and wants to delete it.

```typescript
function PruneCrossChainQueryResult(
  queryId: Identifier,
  queryCapability: CapabilityKey
  ) {

    // Retrieve the query result from the provable store using the query's identifier.
    resultRecord = privateStore.get(resultQueryPath(queryIdentifier))
    abortTransactionUnless(resultRecord !== null)

    // Abort the transaction unless the caller has the right to clean the query result
    abortTransactionUnless(authenticateCapability(queryId, queryCapability))

    // Delete the query result from the public store.
    privateStore.delete(resultQueryPath(queryId))
}
```
- **Precondition**
  - There is a query result stored in the `provableStore` identified by `queryId`.
  - The caller has the right to clean the query result
- **Postcondition**
  - The query result identified by `queryId` is deleted from the `provableStore`.

#### Timeouts

Query requests have associated a `localTimeoutHeight` and a `localTimeoutTimestamp` field that specifies the height and timestamp limit at the Querying Chain after which a query is considered to have failed. 

The Querying Chain calls the `checkQueryTimeout` function to check whether a specific query has timed out. 

> There are several alternatives on how to handle timeouts. For instance, the relayer could submit on-chain timeout notifications to the Querying Chain. Since the relayer is untrusted, for each of these notifications the Cross-chain Querying module of the Querying Chain MUST call the `checkQueryTimeout` to check if the query has indeed timed out. An alternative could be to make the Cross-chain Querying module responsible for checking  
if any query has timed out by iterating over the ongoing queries at the beginning of a block and calling `checkQueryTimeout`. This is an implementation detail that this specification does not cover.

We pass the relayer address just as in `CrossChainQueryResult` to allow for possible incentivization here as well.

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

    // Check that localTimeoutHeight or localTimeoutTimestamp has passed on the Querying Chain (locally)
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

    // Store the result in a public path.
    provableStore.set(resultQueryPath(queryIdentifier), resultRecord)
}
```
- **Precondition**
  - There is a query request stored in the `privateStore` identified by `queryId`.
- **Postcondition**
  - If the query has indeed timed out, then
    - the query request identified by `queryId` is deleted from the `privateStore`;
    - the fact that the query has timed out is recorded in the `provableStore`.

## History

January 6, 2022 - First draft

May 11, 2022 - Major revision

June 14, 2022 - Adds pruning, localTimeoutTimestamp and adds relayer address for incentivization

## Copyright

All content herein is licensed under [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0).
