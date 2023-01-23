# Best Practices When Designing Middleware

## Namespacing the Stack

Each layer of the middleware should reserve a unique name in the stack so that it can key into specific information it needs in the version, packet data, or acknowledgement. Since this will not be in the portID which is shared across the stack, it will not be enforced by IBC. The Cosmos-SDK, for example, can accomplish uniqueness by using the ModuleName to reference the different middleware in the stack.

## Version Negotiation For Middleware

Middleware will only need to negotiate versions if they expect the counterparty chain to have compatible middleware on the stack as well. Middleware that can execute unilaterally **do not** need version negotiation.

Middleware must take care to ensure that the application logic can execute its own version negotiation without interference from the nesting middleware. In order to do this, the middleware will format the version in a JSON-encoded string. Any information intended for the middleware will be keyed on the middleware namespace. The version intended for the underlying app will be keyed on `"app_version"`. The application version may as well be a JSON-encoded string, possibly including further middleware and app versions, if the application stack consists of multiple milddlewares wrapping a base application. The version values themselves may be more complex than static strings, they can be JSON encoded structs with multiple fields that the counterparties must agree on.  The format of the version string is as follows:

```json
{
    "<middleware_name>": "<middleware_version_value>",
    "app_version": "<application_version_value>",
}
```

The `<middleware_name>` key in the JSON struct should be replaced by the actual name of the key for the corresponding middleware (e.g. `fee` for ICS-29 fee middleware).

In the application callbacks, the middleware can unmarshal the version string and retrieve the middleware and application versions. It must do its own version negotiation on `<middleware_version_value>` and then hand over `<application_version_value>` to the nested application's callback. This is only relevant if the middleware expects a compatible counterparty middleware at the same level on the counterparty stack. Middleware that only executes on a single side of the channel MUST NOT modify the channel version.

## Packet Data Structuring For Middleware

Packet senders may choose to send input data not just to the base application but also the middlewares in the stack. The packet data should be structured in the same way that the middleware stack is; i.e. nested from the top level middleware to the base application.

Similar to the version negotiation, the `app_packet_data` may be marshalling packet data for underlying middleware as well.

```json
{
    "<middleware_name>": "<middleware_packet_data>",
    "app_packet_data": "<application_packet_data>",
}
```

On `SendPacket`, the execution flow goes from the base application to the top layer middleware. Thus at each step, the middleware can choose to wrap the application packet data with its own packet data before returning to the `ICS4Wrapper`.

On `ReceivePacket`, the middleware must process its own packet data, and then pass the `app_packet_data` to the underlying application. It may also choose to use the `app_packet_data` as part of its logic execution if it knows how to unmarshal the underlying data.

Since the `OnRecvPacket` callback expects the full packet to be passed in, the packet data must be mutated like so:

```typescript
packetData = unmarshal(packet.data)

// do whatever logic with middleware packet data
handleMiddlewareData(packetData.MiddlewareData)

// reassign packet data bytes before passing packet to next handler
packet.Data = packetData.AppPacketData

return im.app.OnRecvPacket(ctx, packet, relayer)
```

The state machine must provide a way for users to pass in input data that can be marshalled into the full nested packet data. This will enable users to send data not just to the base application but also to higher-level middleware(s). Since this is state-machine specific, it will not be specified here.

## Acknowledgement Structuring For Middleware

Middleware may also add on to acknowledgements. This will be done in the exact same way as version and packet data.

```json
{
    "<middleware_name>": "<middleware_acknowledgement",
    "app_acknowledgement": "<app_acknowledgement>",
}
```

On `WriteAcknowledgment`, the execution flow goes from the base application to the top layer middleware. Thus at each step, the middleware can choose to wrap the application acknowledgement with its own acknowledgement before returning to the `ICS4Wrapper`.

On `AcknowledgePacket`, the middleware must process its own acknowledgement, and then pass the `app_acknowledgement` to the underlying application. It may also choose to use the `app_acknowledgement` as part of its logic execution if it knows how to unmarshal the underlying data.

