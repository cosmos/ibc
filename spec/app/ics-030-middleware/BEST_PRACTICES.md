# Best Practices When Designing Middleware

## Version Negotiation For Middleware

Middleware must take care to ensure that the application logic can execute its own version negotiation without interference from the nesting middleware. In order to do this, the middleware will format the version in a JSON-encoded string containing the middleware version and the application version (and potentially also other custom parameter fields). The application version may as well be a JSON-encoded string, possibly including further middleware and app versions, if the application stack consists of multiple milddlewares wrapping a base application.  The format of the version string is as follows:

```json
{
    "<middleware_version_key>": "<middleware_version_value>",
    "app_version": "<application_version_value>",
    // ... other custom parameter fields
}
```

The `<middleware_version_key>` key in the JSON struct should be replaced by the actual name of the key for the corresponding middleware (e.g. `fee_version` for ICS-29 fee middleware).

In the application callbacks, the middleware can unmarshal the version string and retrieve the middleware and application versions. It must do its own version negotiation on `<middleware_version_value>` and then hand over `<application_version_value>` to the nested application's callback. This is only relevant if the middleware expects a compatible counterparty middleware at the same level on the counterparty stack. Middleware that only executes on a single side of the channel MUST NOT modify the channel version.

## Packet Data Structuring For Middleware

## Acknowledgement Structuring For Middleware