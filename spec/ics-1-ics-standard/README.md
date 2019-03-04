---
ics: 1
title: ICS Specification Standard
stage: proposal
category: meta
author: Christopher Goes <cwgoes@tendermint.com>
created: 2019-02-12
modified: 2019-03-04
---

## What is an ICS?

## Components

### Header

#### Required fields

`ics: #` - ICS number (assigned sequentially)

`title` - ICS title (keep it short & sweet)

`stage` - Current ICS stage, one of the following:
- `proposal` - A standard in the "proposal" stage
- `draft` - A standard in the "draft" stage
- `candidate` - A standard in the "candidate" stage
- `finalized` - A standard in the "finalized" stage

See [README.md](../../README.md) for a description of the ICS acceptance stages.

`category` - ICS category, one of the following:
- `meta` - A standard about the ICS process
- `ibc`  - A standard about the inter-blockchain communication system
- `util` - A standard about utility features, e.g. message signing

`author` - ICS author(s) & contact information (in order of preference: email, Github handle, Twitter handle, other contact methods).
           The first author is the primary "owner" of the ICS and is responsible for advancing it through the standardization process.
           Subsequent author ordering should be in order of contribution amount.

`created` - Date ICS was first created (`YYYY-MM-DD`)

`modified` - Date ICS was last modified (`YYYY-MM-DD`)

#### Optional fields

`requires` - Other ICS standards, referenced by number, which are required or depended upon by this standard.

`required-by` - Other ICS standards, referenced by number, which require or depend upon this standard.

`replaces` - Another ICS standard replaced or supplanted by this standard, if applicable.

`replaced-by` - Another ICS standard which replaces or supplants this standard, if applicable.

### Synopsis

### Specification

### Copyright

Copyright and related rights waived via [CC0](https://creativecommons.org/publicdomain/zero/1.0/).
