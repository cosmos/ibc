---
ics: TBA
title: Interchain Query
stage: draft
category: IBC/APP
requires: 2, 18, 23, 24
kind: instantiation
author: Joe Schnetzler <schnetzlerjoe@gmail.com>, Manuel Bravo <manuel@informal.systems>
created: 2022-01-06
modified: 2022-05-03
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

`CommitmentPath` and `CommitmentProof` are as defined in ICS 23.

`Identifier`, `get`, `set`, `delete`, `getCurrentHeight`, and module-system related primitives are as defined in ICS 24.

## System Model and Properties

### Assumptions

- **Safe chains:** Both the Querying and Queried chains are safe.

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

A bounty is paid to incentivize relayers for participating in interchain queries.

## Technical Specification

### General Design 

The Querying Chain MUST implement the Cross-chain Querying module, which allows the Querying Chain to query state at the Queried Chain. 

Cross-chain querying relies on relayers operating between both chains. When a query request is received by the Cross-chain Querying module of the Querying Chain, this emits a `sendQuery` event. Relayers operating between the Querying and Queried chains must monitor the Querying chain for `sendQuery` events. Eventually, a relayer will retrieve the query request and execute it at the Queried Chain. The relayer then submits (on-chain) the result at the Querying Chain. The result is finally stored at the Querying Chain by the Cross-chain Querying module.

A query request includes the height of the Queried Chain at which the query must be executed. The reason is that the keys being queried can have different values at different heights. Thus, a malicious relayer could choose to query a height that has a value that benefits it somehow. By letting the Querying Chain decide the height at which the query is executed, we can prevent relayers from affecting the result data.

### Data Structures

The Cross-chain Querying module stores query requests when it processes them. 

A CrossChainQuery is a particular interface to represent query requests. A request is retrieved when its result is submitted.

```typescript
interface CrossChainQuery struct {
    id: Identifier
	path: CommitmentPath
	timeoutHeight: Height
    queryHeight: Height
    clientId: Identifier
	bounty: sdk.Coin
}
```

- The `id` field uniquely identifies the query at the Querying Chain.
- The `path` field is the path to be queried at the Queried Chain.
- The `timeoutHeight` field  specifies a height limit at the Querying Chain after which a query is considered to have failed and a timeout result should be returned to the original caller.
- The `queryHeigth` field is the height at which the relayer must query the Queried Chain
- The `bounty` field is a bounty that is given to the relayer for participating in the query.
- The `clientId` field identifies the Queried Chain.

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
- A query that timeout before a result is committed at the Querying Chain is marked as `TIMEOUT`.

A CrossChainQueryResult is a particular interface used to represent query replies.

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

#### Ongoing query path

The ongoing query path is a private path that stores the state of ongoing cross-chain queries.

```typescript
function ongoingQueryPath(id: Identifier): Path {
    return "ibcquery/{id}"
}
```
#### Result query path

The result query path is a public path that stores the result of completed queries.

```typescript
function resultQueryPath(id: Identifier): Path {
    return "ibcqueryresult/{id}"
}
```

### Helper functions

The Querying Chain MUST implement a function `generateQueryIdentifier`, which generates a unique query identifier:

```typescript
function generateQueryIdentifier = () -> Identifier
```

### Sub-protocols

#### Query lifecycle

1) When the Cross-chain Querying module receives a query request, it calls `CrossChainQueryRequest`. This function generates a unique identifier for the query, stores it in its `privateStore` and emits a `sendQuery` event. Query requests can be submitted by other IBC modules as transactions to the Querying Chain or simply executed as part of the `BeginBlock` and `EndBlock` logic.
2) A correct relayer listening to `sendQuery` events from the Querying Chain will eventually pick the query request up and execute it at the Queried Chain. The result is then submitted (on-chain) to the Querying Chain.
3) When the query result is committed at the Querying Chain, it is handed to the Cross-chain Querying module.
4) The Cross-chain Querying module calls `CrossChainQueryResult`. This function first retrieves the query from the `privateStore` using the query's unique identifier. It then proceeds to verify the result using its local client. If it passes the verification, the function removes the query from the `privateStore` and stores the result in a public path.
5) The query caller can then asynchronously retrieve the query result.

#### Normal path methods

The `CrossChainQueryRequest` function is called when the Cross-chain Querying module at the Querying Chain receives a new query request.

```typescript
function CrossChainQueryRequest(
  path: CommitmentPath,
  queryHeigth: Heigth,
  timeoutHeigth: Height,
  clientId: Identifier,
  bounty: sdk.Coin,
  ): Identifier {

    // Check that there exists a client of the Queried Chain. The client will be used to verify the query result.
    abortTransactionUnless(queryClientState(clientId) !== null)

    // Generate a unique query identifier.
    queryIdentifier = generateQueryIdentifier()

    // Create a query request record.
    query = CrossChainQuery{queryIdentifier,
                            path,
                            queryHeigth,
                            timeoutHeigth, 
                            clientId,
                            bounty}

    // Store the query in the local, private store.
    privateStore.set(ongoingQueryPath(queryIdentifier), query)

    // Log the query request.
    emitLogEntry("sendQuery", query)

    // Returns the query identifier.
    return queryIdentifier
}
```
- **Precondition**
  - There exists a client with `clientId` identifier.
- **Postcondition**
  - The query request is stored in the `privateStore`.
  - A `sendQuery` event is emitted.

The `CrossChainQueryResult` function is called when the Cross-chain Querying module at the Querying Chain receives a new query reply.

```typescript
function CrossChainQueryResult(
  queryId: Identifier,
  data: []byte
  proof: CommitmentProof,
  proofHeight: Height,
  success: boolean,
  delayPeriodTime: uint64,
  delayPeriodBlocks: uint64
  ) {

    // Retrieve query state from the local, private store using the query's identifier.
    query = privateStore.get(queryPath(queryIdentifier))
    abortTransactionUnless(query !== null)

    // Retrieve client state of the Queried Chain.
    client = queryClientState(query.clientId)
    abortTransactionUnless(client !== null)

    // Check that the relier executed the query at the requested height at the Queried Chain.
    abortTransactionUnless(query.queryHeigth !== proofHeight)

    // Verify query result using the local light client of the Queried Chain. If success, then verify that the data is indeed the value associated with query.path at query.queryHeight at the Queried Chain. Otherwise, verify that query.path does not exist at query.queryHeight at the Queried Chain.
    if (success) {    
        abortTransactionUnless(client.verifyMemership(
            client,
            proofHeight,
            delayPeriodTime,
            delayPeriodBlocks,
            proof,
            query.path,
            data
        ))
        result = FOUND
    } else {
        abortTransactionUnless(client.verifyNonMemership(
            client,
            proofHeight,
            delayPeriodTime,
            delayPeriodBlocks,
            proof,
            query.path,
        ))
        result = NOTFOUND
    }

    // Delete the query from the local, private store.
    query = privateStore.delete(ongoingQueryPath(queryId))

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

#### Timeouts

Query requests have associated a `timeoutHeight` field that specifies the height limit at the Querying Chain after which a query is considered to have failed. 

The Querying Chain calls the `checkQueryTimeout` function to check whether a specific query has timeout. 

> There are several alternatives on how to handle timeouts. For instance, the relayer could submit on-chain timeout notifications to the Querying Chain. Since the relayer is untrusted, for each of these notifications the Cross-chain Querying module of the Querying Chain MUST call the `checkQueryTimeout` to check if the query has indeed timeout. An alternative could be to make the Cross-chain Querying module responsible for checking  
if any query has timeout by iterating over the ongoing queries at the beginning of a block and calling `checkQueryTimeout`. This is an implementation detail that this specification does not cover.

```typescript
function checkQueryTimeout(
    queryId: Identifier
){
    // Retrieve the query state from the local, private store using the query's identifier.
    query = privateStore.get(queryPath(queryIdentifier))
    abortTransactionUnless(query !== null)

    // Get the current height.
    currentHeight = getCurrentHeight()

    
    if (currentHeight > query.timeoutHeight) {
        // Delete the query from the local, private store if it has timeout
        query = privateStore.delete(ongoingQueryPath(queryId))

        // Create a query result record.
        resultRecord = CrossChainQuery{queryIdentifier,
                                       TIMEOUT,
                                       null} 

        // Store the result in a public path.
        provableStore.set(resultQueryPath(queryIdentifier), resultRecord)
    }
}
```
- **Precondition**
  - There is a query request stored in the `privateStore` identified by `queryId`.
- **Postcondition**
  - If the query has indeed timeout, then
    - the query request identified by `queryId` is deleted from the `privateStore`;
    - the fact that the query has timeout is recorded in the `provableStore`.