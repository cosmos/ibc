<!-- SPDX-License-Identifier: Community-Spec-1.0 -->

# Community Specification Template 1.0

## How this template applies to the IBC specifications

Start a new specification from [ics-template.md](ics-template.md) and take it through the stages in [PROCESS.md](PROCESS.md). The ICS template is this Working Group's form of the template below. It follows the format set by [ICS 1](ics-001-ics-standard/README.md), and every ICS in this repository is written to it. The template's clauses correspond to ICS sections as follows.

| Community Specification template | ICS section or frontmatter field |
| --- | --- |
| Title, Version, Status | `ics` and `title`; `version`, where the ICS has more than one; `stage` |
| Copyright and license notice | `## Copyright` |
| Introduction | `## Synopsis` and `### Motivation` |
| 1 Scope | `## Synopsis`, with `category` and `kind` |
| 2 Normative references | `requires` and `implements`, which list the ICS numbers the specification depends on or implements |
| 3 Terms and definitions | `### Definitions` |
| 4 onwards, clauses and subclauses | `### Desired Properties` and `## Technical Specification`, with `### Data Structures`, `### Sub-protocols` and `### Properties & Invariants` |
| Annex (informative) | `## Backwards Compatibility`, `## Forwards Compatibility` and `## Example Implementations` |
| Bibliography | `## History` |

As in ICS 1, specifications are written in GitHub-flavoured Markdown, the key words below are interpreted as described in RFC 2119, and data structures are written as simple TypeScript interfaces.

The template below is the Community Specification Template 1.0, with its blanks filled in for this Working Group.

Community Specifications are recommended to be drafted in accordance with international best practices.  Doing so provides clarity, helps adoption, and also eases the transition of this specification to other standards body if so desired.  Accordingly, the recommended template below is based on ISO standard drafting conventions.

To help you, this guide on writing standards was produced by the ISO/TMB and is available at https://www.iso.org/iso/how-to-write-standards.pdf.

A model manuscript of a draft International Standard (known as “The Rice Model”) is available at https://www.iso.org/iso/model_document-rice_model.pdf.

In addition, we recommend using the key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" as described in RFC 2119 - https://tools.ietf.org/html/rfc2119

 
## ICS (number): (title)
### Version (the ICS version, for example ICS 20 v1)
### Status (Strawman, Draft, Candidate, Finalised; see [PROCESS.md](PROCESS.md))

 
© IBC contributors

This specification is subject to the Community Specification License 1.0, available at [https://github.com/CommunitySpecification/1.0](https://github.com/CommunitySpecification/1.0).

If source code is included in the specification, that code is subject to the [Apache 2.0](https://www.apache.org/licenses/LICENSE-2.0) license unless otherwise marked. In the case of any conflict or confusion within this specification between the Community Specification License and the designated source code license, the terms of the Community Specification License shall apply.

 

## Contents

Foreword	i

Introduction	i

1	Scope (mandatory)	1

2	Normative references (mandatory)	1

3	Terms and definitions (mandatory)	1

4	Clause title autonumber	1

5	Clause title	1

5.1	Subclause autonumber	1

5.1.1	Subclause autonumber	1

6	Clause title	1

Annex A (informative)  

Bibliography	1


## Foreword

Attention is drawn to the possibility that some of the elements of this document may be the subject of patent rights. No party shall be held responsible for identifying any or all such patent rights.

Any trade name used in this document is information given for the convenience of users and does not constitute an endorsement.

This document was prepared by the IBC Working Group.

This version (vN) of ICS (number) cancels and replaces version vN-1, which has been technically revised. The main changes compared to the previous version are recorded in the specification's History section.

Known patent licensing exclusions are available in the specification’s repository’s [Notices](03-notices.md) file.

Any feedback or questions on this document should be directed to specifications repository, located at [https://github.com/cosmos/ibc](https://github.com/cosmos/ibc).

THESE MATERIALS ARE PROVIDED “AS IS.” The Contributors and Licensees expressly disclaim any warranties (express, implied, or otherwise), including implied warranties of merchantability, non-infringement, fitness for a particular purpose, or title, related to the materials.  The entire risk as to implementing or otherwise using the materials is assumed by the implementer and user. IN NO EVENT WILL THE CONTRIBUTORS OR LICENSEES BE LIABLE TO ANY OTHER PARTY FOR LOST PROFITS OR ANY FORM OF INDIRECT, SPECIAL, INCIDENTAL, OR CONSEQUENTIAL DAMAGES OF ANY CHARACTER FROM ANY CAUSES OF ACTION OF ANY KIND WITH RESPECT TO THIS DELIVERABLE OR ITS GOVERNING AGREEMENT, WHETHER BASED ON BREACH OF CONTRACT, TORT (INCLUDING NEGLIGENCE), OR OTHERWISE, AND WHETHER OR NOT THE OTHER MEMBER HAS BEEN ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.

Introduction
Type text.

 
## Title (Introductory element — Main element — Part : Part title)

1	Scope (mandatory)

Type text.

2	Normative references (mandatory)

Two options of text (remove the inappropriate option).

1)	The normative references shall be introduced by the following wording.

The following documents are referred to in the text in such a way that some or all of their content constitutes requirements of this document. For dated references, only the edition cited applies. For undated references, the latest edition of the referenced document (including any amendments) applies.

ISO ##### #, General title — Part #: Title of part

ISO ##### ##:20##, General title — Part ##: Title of part

2)	If no references exist, include the following phrase below the clause title:
There are no normative references in this document.

3	Terms and definitions (mandatory)

Four options of text (remove the inappropriate options).

1)	If all the specific terms and definitions are provided in Clause 3, use the following introductory text:

For the purposes of this document, the following terms and definitions apply.

2)	If reference is given to an external document, use the following introductory text:

For the purposes of this document, the terms and definitions given in [external document reference xxx] apply.

3)	If terms and definitions are provided in Clause 3, in addition to a reference to an external document, use the following introductory text:

For the purposes of this document, the terms and definitions given in [external document reference xxx] and the following apply.

4)	If there are no terms and definitions provided, use the following introductory text:

No terms and definitions are listed in this document.

The list below is always included after each option:

ISO and IEC maintain terminological databases for use in standardization at the following addresses:

—	ISO Online browsing platform: available at https://www.iso.org/obp

—	IEC Electropedia: available at http://www.electropedia.org/


3.1

term

text of the definition

Note 1 to entry: Text of the note.

[SOURCE: …]

3.2

term

text of the definition

4	Clause title 

Type text.

5	Clause title

Type text.

Use subclauses if required e.g. 5.1 or 5.1.1. For example:

5.1	Subclause 

5.1.1	Subclause 

6	Clause title

Example of codes, with data structures written as simple TypeScript interfaces as ICS 1 requires:

```typescript
interface Connection {
  state: ConnectionState
  version: Version
  counterpartyIdentifier: Identifier
}
```

## Annex A

(informative)

Annex title e.g. Example of a figure and a table

A.1	Clause title autonumber

Use subclauses if required e.g. A.1.1 or A.1.1.1. For example:

A.1.1	Subclause autonumber

A.1.1.1	Subclause autonumber

Type text.

Dimensions in millimetres

## Bibliography

[1]	ISO ##### #, General title — Part #: Title of part

[2]	ISO ##### ##:20##, General title — Part ##: Title of part
