---
ics: TBA
title: Interchain Query
stage: draft
category: IBC/APP
requires: 25, 26, 27
kind: instantiation
author: Joe Schnetzler <schnetzlerjoe@gmail.com>
created: 2022-01-06
modified: 2020-01-09
---

## Synopsis

This documents aims to document the structure, plan and implementation of the Interchain Queries Module allowing for cross-chain querying of state from IBC enabled chains.

### Motivation

Interchain Accounts (ICS-27) brings one of the most important features IBC offers, cross chain transactions (on-chain). Limited in this functionality is the querying of state from one chain, on another chain. Adding interchain querying via the Interchain Query module, gives unlimited flexibility to chains to build IBC enabled protocols around Interchain Accounts and beyond.

### Definitions 

- `Querying Chain`: The chain that is interested in getting data from another chain (Queried Chain). The Querying Chain is the chain that implements the Interchain Query Module into their chain.
- `Queried Chain`: The chain that's state is being queried via the Querying Chain. The Queried Chain gets queried via a relayer utilizing its RPC client which is then submitted back to the Querying Chain.

### Desired Properties

- Permissionless: A Querying Chain can query a chain and implement cross-chain querying without any approval from a third party or chain governance.

- Minimal Querying Chain Work: A Queried Chain has to do no implementation work or add any module to enable cross chain querying. By utilizing an RPC client on a relayer, this is possible.

- Modular: Adding cross-chain querying should be as easy as implementing a module in your chain.

- Control Queried Data: The Querying Chain should have ultimate control on how to handle queried data.

- Incentivization: In order to incentivize relayers for participating in interchain queries, a bounty is paid.

## Technical Specification

### General Design 

The Querying Chain starts with the implementation of the Interchain Query Module by adding the module into their chain.

The general flow for interchain queries starts with a Cross Chain Query Request from the Querying Chain which is listened to by relayers. Upon recognition of a cross chain query, relayers utilize a ABCI Query Request to query data from the Queried Chain. Upon success, the relayer submits a `MsgSubmitQueryResult` to the Querying chain.

On failure of a query, relayers submit `MsgSubmitQueryErrorResult` to the Querying chain. Alternatively on timeout relayers submit `MsgSubmitQueryTimeoutResult`.

### Data Structures

A CrossChainABCIQueryRequest data type is used to specify the query. Included in this is the.

```go
type CrossChainABCIQueryRequest struct {
	Data          []byte
	Path          string
	timeoutHeight uint64
	Bounty        sdk.Coin
	ChainId       string
}
```

```go
type QueryResult struct {
	Data    []byte
	Height  uint64
	ChainId string
}
```

```go
type QueryErrorResult struct {
	Error   string
	ChainId string
}
```

```go
type QueryTimeoutResult struct {
	timeoutHeight  string
	ChainId        string
}
```

### Messages


```go
func MsgCrossChainABCIQueryRequest(
	
) *cobra.Command {
  //Msg for query request submit message
}
```

```go
func MsgSubmitQueryResult(

) *cobra.Command {
  //Msg to submit a query result
}
```

```go
func MsgSubmitQueryErrorResult(

) *cobra.Command {
  //Msg to submit a query error result
}
```

```go
func MsgSubmitQueryTimeoutResult(

) *cobra.Command {
  //Msg to submit a query timeout result
}
```

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
  //Msg to get a query via id
}
```

### Events

```go
func EmitQueryEvent(ctx sdk.Context, timeoutHeight exported.Height) {
	//Event to trigger query event on relayer
	ctx.EventManager().EmitEvents(sdk.Events{
		sdk.NewEvent(
		)
	})
}
```