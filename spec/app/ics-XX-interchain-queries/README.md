---
ics: TBD
title: Interchain Queries
stage: Draft
category: IBC/APP
requires: 25, 26
kind: instantiation
author: Ali Zahid Raja <ali@polymerlabs.org>, Ehsan Saradar <ehsan@quasar.fi>
created: 2022-06-22
modified: 2022-08-30
---

## Synopsis

This document serves as a guide for a better understanding of the implementation of interchain queries via ABCI Query

### Motivation

Interchain Queries enable blockchains to query the state of an account on another chain without the need for ICA auth. ICS-27 Interchain Accounts is used for IBC transactions, e.g. to transfer coins from one interchain account to another, whereas Interchain Queries is used for IBC Query, e.g. to query the balance of an account on another chain. 
In short, ICA is cross chain writes while ICQ is cross chain reads.

### Definitions 

- `Host Chain`: The chain where the query is sent. The host chain listens for IBC packets from a controller chain that contain instructions (e.g. cosmos SDK messages) that the interchain account will execute.
- `Querier Chain`: The chain sending the query to the host chain. The controller chain sends IBC packets to the host chain to query information.
- `Interchain Query`: An IBC packet that contains information about the query in the form of ABCI RequestQuery

The chain which sends the query becomes the controller chain, and the chain which receives the query and responds becomes the host chain for the scenario.

### Desired properties

- Permissionless: An interchain query may be created by any actor without the approval of a third party (e.g. chain governance)


## Technical Specification

![model](icq-img.png)


### ABCI Query

ABCI RequestQuery enables blockchains to request information made by the end-users of applications. A query is received by a full node through its consensus engine and relayed to the application via the ABCI. It is then routed to the appropriate module via BaseApp's query router so that it can be processed by the module's query service

ICQ can only return information from stale reads, for a read that requires consensus, ICA (ICS-27) will be used.


#### **SendQuery**

`SendQuery` is used to send an IBC packet containing query information to an interchain account on a host chain.

```go
func (k Keeper) SendQuery(ctx sdk.Context, sourcePort, sourceChannel string, chanCap *capabilitytypes.Capability, 
reqs []abci.RequestQuery, timeoutHeight clienttypes.Height, timeoutTimestamp uint64) (uint64, error) {
	
    sourceChannelEnd, found := k.channelKeeper.GetChannel(ctx, sourcePort, sourceChannel)
	if !found {
		return 0, sdkerrors.Wrapf(channeltypes.ErrChannelNotFound, "port ID (%s) channel ID (%s)", sourcePort, sourceChannel)
	}

	destinationPort := sourceChannelEnd.GetCounterparty().GetPortID()
	destinationChannel := sourceChannelEnd.GetCounterparty().GetChannelID()

	icqPacketData := types.InterchainQueryPacketData{
		Requests: reqs,
	}

	return k.createOutgoingPacket(ctx, sourcePort, sourceChannel, destinationPort, destinationChannel, chanCap, icqPacketData, timeoutTimestamp)
}
```

#### **authenticateQuery**

`authenticateQuery` is called before `executeQuery`.

`authenticateQuery` checks that the query is a part of the whitelisted queries.

```go
func (k Keeper) authenticateQuery(ctx sdk.Context, q abci.RequestQuery) error {
	allowQueries := k.GetAllowQueries(ctx)
	if !types.ContainsQueryPath(allowQueries, q.Path) {
		return sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "query path not allowed: %s", q.Path)
	}
	if !(q.Height == 0 || q.Height == ctx.BlockHeight()) {
		return sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "query height not allowed: %d", q.Height)
	}
	if q.Prove {
		return sdkerrors.Wrapf(sdkerrors.ErrUnauthorized, "query proof not allowed")
	}

	return nil
}
```


#### **executeQuery**

Executes each query sent by the controller chain.

```go
func (k Keeper) executeQuery(ctx sdk.Context, reqs []abci.RequestQuery) ([]byte, error) {
	resps := make([]abci.ResponseQuery, len(reqs))
	for i, req := range reqs {
		if err := k.authenticateQuery(ctx, req); err != nil {
			return nil, err
		}

		resp := k.querier.Query(req)
		// Remove non-deterministic fields from response
		resps[i] = abci.ResponseQuery{
			Code:   resp.Code,
			Index:  resp.Index,
			Key:    resp.Key,
			Value:  resp.Value,
			Height: resp.Height,
		}
	}

	bz, err := types.SerializeCosmosResponse(resps)
	if err != nil {
		return nil, err
	}
	ack := types.InterchainQueryPacketAck{
		Data: bz,
	}
	data, err := types.ModuleCdc.MarshalJSON(&ack)
	if err != nil {
		return nil, sdkerrors.Wrap(err, "failed to marshal tx data")
	}

	return data, nil
}
```

### Packet Data

`InterchainQueryPacketData` is comprised of raw query.

```proto
message InterchainQueryPacketData  {
    bytes data = 1;
}
```

`InterchainQueryPacketAck` is comprised of an ABCI query response with non-deterministic fields left empty (e.g. Codespace, Log, Info and ...).

```proto
message InterchainQueryPacketAck {
	bytes data = 1;
}
```

`CosmosQuery` contains a list of tendermint ABCI query requests. It should be used when sending queries to an SDK host chain.
```proto
message CosmosQuery {
  repeated tendermint.abci.RequestQuery requests = 1 [(gogoproto.nullable) = false];
}
```

`CosmosResponse` contains a list of tendermint ABCI query responses. It should be used when receiving responses from an SDK host chain.
```proto
message CosmosResponse {
  repeated tendermint.abci.ResponseQuery responses = 1 [(gogoproto.nullable) = false];
}
```


### Packet relay

`onRecvPacket` is called by the routing module when a packet addressed to this module has been received.

```go
func (k Keeper) OnRecvPacket(ctx sdk.Context, packet channeltypes.Packet) ([]byte, error) {
	var data types.InterchainQueryPacketData

	if err := types.ModuleCdc.UnmarshalJSON(packet.GetData(), &data); err != nil {
		// UnmarshalJSON errors are indeterminate and therefore are not wrapped and included in failed acks
		return nil, sdkerrors.Wrapf(types.ErrUnknownDataType, "cannot unmarshal ICQ packet data")
	}

	reqs, err := types.DeserializeCosmosQuery(data.GetData())
	if err != nil {
		return nil, err
	}

	response, err := k.executeQuery(ctx, reqs)
	if err != nil {
		return nil, err
	}
	return response, err
}
```


### Sample Query

To get the balance of an address we can use the following
In order to send a packet from a module, first you need to prepare your query Data and encapsule it in an ABCI RequestQuery. Then you can use SerializeCosmosQuery to construct the Data of InterchainQueryPacketData packet data. After these steps you should use ICS 4 wrapper to send your packet to the host chain through a valid channel.

```go
q := banktypes.QueryAllBalancesRequest{
    Address: "cosmos1tshnze3yrtv3hk9x536p7znpxeckd4v9ha0trg",
    Pagination: &query.PageRequest{
        Offset: 0,
        Limit: 10,
    },
}

reqs := []abcitypes.RequestQuery{
	{
		Path: "/cosmos.bank.v1beta1.Query/AllBalances",
		Data: k.cdc.MustMarshal(&q),
	},
}

bz, err := icqtypes.SerializeCosmosQuery(reqs)
if err != nil {
	return 0, err
}
icqPacketData := icqtypes.InterchainQueryPacketData{
	Data: bz,
}

packet := channeltypes.NewPacket(
		icqPacketData.GetBytes(),
		sequence,
		sourcePort,
		sourceChannel,
		destinationPort,
		destinationChannel,
		clienttypes.ZeroHeight(),
		timeoutTimestamp,
)

// Send the `packet` with ICS-4 interface
```

### Sample Acknowledgement Response

Successful acknowledgment will be sent back to querier module as InterchainQueryPacketAck. The Data field should be deserialized to and array of ABCI ResponseQuery with DeserializeCosmosResponse function. Responses are sent in the same order as the requests.


```go
switch resp := ack.Response.(type) {
	case *channeltypes.Acknowledgement_Result:
		var ackData icqtypes.InterchainQueryPacketAck
		if err := icqtypes.ModuleCdc.UnmarshalJSON(resp.Result, &ackData); err != nil {
			return sdkerrors.Wrap(err, "failed to unmarshal interchain query packet ack")
		}

        resps, err := icqtypes.DeserializeCosmosResponse(ackData.Data)
        if err != nil {
            return sdkerrors.Wrap(err, "failed to unmarshal interchain query packet ack to cosmos response")
        }

		if len(resps) < 1 {
			return sdkerrors.Wrap(sdkerrors.ErrInvalidRequest, "no responses in interchain query packet ack")
		}

		var r banktypes.QueryAllBalancesResponse
		if err := k.cdc.Unmarshal(resps[0].Value, &r); err != nil {
			return sdkerrors.Wrapf(err, "failed to unmarshal interchain query response to type %T", resp)
		}

        // `r` is the response of your query
...
```


## Other Implementations

Another implementation of Interchain Queries is by the use of KV store which can be seen implemented here by [QuickSilver](https://github.com/ingenuity-build/quicksilver/tree/main/x/interchainquery)

The implementation works even if the host side hasn't implemented ICQ, however, it does not fully leverage the IBC standards. 

## History

June 22, 2022 - Draft

August 30, 2022 - Major Revisions, added ICQ to IBC-test