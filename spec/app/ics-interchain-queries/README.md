---
ics: TBA
title: Interchain Query
stage: draft
category: IBC/APP
requires: 25, 26, 27
kind: instantiation
author: Joe Schnetzler <schnetzlerjoe@gmail.com>
created: 2022-01-06
modified: 2022-02-12
---

## Synopsis

This documents aims to document the structure, plan and implementation of the Interchain Queries Module allowing for cross-chain querying of state from IBC enabled chains.

### Motivation

Interchain Accounts (ICS-27) brings one of the most important features IBC offers, cross chain transactions (on-chain). Limited in this functionality is the querying of state from one chain, on another chain. Adding interchain querying via the Interchain Query module, gives unlimited flexibility to chains to build IBC enabled protocols around Interchain Accounts and beyond.

### Definitions 

- `Querying Chain`: The chain that is interested in getting data from another chain (Queried Chain). The Querying Chain is the chain that implements the Interchain Query Module into their chain.
- `Queried Chain`: The chain that's state is being queried via the Querying Chain. The Queried Chain gets queried via a relayer utilizing its RPC client which is then submitted back to the Querying Chain.
- `Key`: A Key in the querying chain module is a user specified query identifier for a query so a user can identify various query types. For example, if you were to query stakers/delegators on Gaia (Cosmos Hub) and store it in state. You can set the `key` as `stakers` and thus query via the key later on as needed.

``Note:`` that a query in state can only have one unique key-id representation. For example, if a query with key = stakers & id = 0 is already in state, the query will fail if you try to add another query with key = stakers & id = 0. A query with key = stakers & id = 1 will succeed however (as long as that is not in state already).

### Desired Properties

- Permissionless: A Querying Chain can query a chain and implement cross-chain querying without any approval from a third party or chain governance.

- Minimal Querying Chain Work: A Queried Chain has to do no implementation work or add any module to enable cross chain querying. By utilizing an RPC client on a relayer, this is possible.

- Modular: Adding cross-chain querying should be as easy as implementing a module in your chain.

- Control Queried Data: The Querying Chain should have ultimate control on how to handle queried data. Like querying for a certain query form/type.

- Incentivization: In order to incentivize relayers for participating in interchain queries, a bounty is paid.

## Technical Specification

### General Design 

The Querying Chain starts with the implementation of the Interchain Query Module by adding the module into their chain.

The general flow for interchain queries starts with a Cross Chain Query Request from the Querying Chain which is listened to by relayers. Upon recognition of a cross chain query, relayers utilize a ABCI Query Request to query data from the Queried Chain. Upon success, the relayer submits a `MsgSubmitQueryResult` to the Querying chain with the success flag as 1.

On failure of a query, relayers submit `MsgSubmitQueryResult` with the `success` flag as 0 to the Querying chain. Alternatively on timeout based on the height of the querying chain, the querying chain will submit `SubmitQueryTimeoutResult` with the timeout height specified.

### Data Structures

A CrossChainABCIQueryRequest data type is used to specify the query. Included in this is the `Path` which is the path field of the query i.e: /custom/auth/account. `Key` is the data key to name the query i.e: `pools` or `stakers`. `Id` is the id of the query with each key to id being unique i.e: stakers-0 or stakers-1275 (this example follows key-id format). `TimeoutHeight` specifies the timeout height on the querying chain to timeout the query. `Bounty` is a bounty that is given to the relayer for participating in the query. `ClientId` is used to identify the chain of interest. 

```go
type CrossChainABCIQueryRequest struct {
	Path           string
	Key            string
	Id             string
	TimeoutHeight  uint64
	Bounty         sdk.Coin
	ClientId       string
}
```

```go
type QueryResult struct {
	Data     []byte
	Key      string
	Id       string
	Height   uint64
	ClientId string
	Success  bool
	Proof    ProofOps
}
```

```go
type QueryTimeoutResult struct {
	Key            string
	Id       	   string
	TimeoutHeight  string
	ClientId       string
	Proof          ProofOps
}
```

### Keepers

```go
func CrossChainABCIQueryRequest(
	QueryRequest CrossChainABCIQueryRequest
) {
  //Keeper to initiate interchain query request. Can be imported into any module and called as needed.
}
```

At the beginning of each block, the querying module checks for pending queries and if the timeout on the querying chain is hit, a timeout result is submitted to state on-chain.

```go
func SubmitQueryTimeoutResult(
	QueryTimeout QueryTimeoutResult
) {
  //Keeper to submit a query timeout result. Applied at the beggining of each block and timedout when the querying chain hits timeout height.
}
```

### Messages

The querying chain has messages that the relayer can submit depending on a success/failed query.

```go
func MsgSubmitQueryResult(

) *cobra.Command {
  //Msg to submit a query result
}
```

The querying chain will have messages to get a list of interchain queries as well as get a query by type and id.

```go
func MsgGetQueries(

) *cobra.Command {
  //Msg to get a list of queries
}
```

```go
// Add -- max-height flag to allow for querying the highest height for a query (most recent)?
func MsgGetQuery(

) *cobra.Command {
  //Msg to get a query via id and key (i.e: id: 1 and key: OsmosisPool)
}
```

### Events

The querying chain will emit an `EmitQueryEvent` which will signal the relayer to go perform an interchain query and submit the results on-chain.

```go
func EmitQueryEvent(ctx sdk.Context, timeoutHeight exported.Height) {
	//Event to trigger query event on relayer
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
		)
	})
}
```