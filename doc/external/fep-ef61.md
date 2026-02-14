---
slug: "ef61"
authors: silverpill <@silverpill@mitra.social>
type: implementation
status: DRAFT
dateReceived: 2023-12-06
trackingIssue: https://codeberg.org/fediverse/fep/issues/209
discussionsTo: https://codeberg.org/silverpill/feps/issues
---
# FEP-ef61: Portable Objects

## Summary

Portable [ActivityPub][ActivityPub] objects with server-independent IDs.

## Motivation

Usage of HTTP(S) URIs as identifiers has a major drawback: when the server disappears, everyone who uses it loses their identity and data.

The proposed solution should satisfy the following constraints:

- User's identity and data should not be tied to a single server.
- Users should have a choice between full control over their identity and data, and delegation of control to a trusted party.
- Implementing the solution in existing software should be as simple as possible. Changes to ActivityPub data model should be kept to a minimum.
- The solution should be compatible with existing and emerging decentralized identity and storage systems.
- The solution should be transport-agnostic.

## History

[Nomadic identity](https://joinfediverse.wiki/index.php?title=Nomadic_identity/en) mechanism makes identity independent from a server and was originally part of the Zot federation protocol.

[Streams](https://codeberg.org/streams/streams) (2021) made nomadic accounts available via the [Nomad protocol](https://codeberg.org/streams/streams/src/commit/11f5174fdd3dfcd8714974f93d8b8fc50378a193/spec/Nomad/Home.md), which supported ActivityStreams serialisation.

[FEP-c390](https://codeberg.org/fediverse/fep/src/branch/main/fep/c390/fep-c390.md) (2022) introduced a decentralized identity solution compatible with ActivityPub. It enabled permissionless migration of followers between servers, but didn't provide full data portability.

## Requirements

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in [RFC-2119][RFC-2119].

## Identifiers

An [ActivityPub][ActivityPub] object can be made portable by using an identifier that is not tied to a single server. This proposal describes a new identifier type that has this property and is compatible with the [ActivityPub] specification.

### 'ap' URIs

'ap' URI is constructed according to the [RFC-3986] specification, but with a [Decentralized Identifier][DID] in place of the authority:

```text
ap://did:example:abcdef/path/to/object?name=value#fragment-id
\_/  \________________/ \____________/ \________/ \_________/
 |           |                |            |           |
scheme   authority           path        query     fragment
```

- The URI scheme MUST be `ap`.
- The authority component MUST be a valid [DID]. Colons and other reserved characters MAY be [percent-encoded][RFC-3986-PercentEncoding].
- The path is REQUIRED. It MUST be treated as an opaque string.
- The query is OPTIONAL. To avoid future conflicts, implementers SHOULD NOT use parameter names that are not defined in this proposal.
- The fragment is OPTIONAL.

> [!WARNING]
> An 'ap' URI is not a valid [RFC-3986] URI if reserved characters in the authority component are not percent-encoded. Nevertheless, this form is considered canonical.

>[!NOTE]
>ActivityPub specification [requires][ActivityPub-ObjectIdentifiers] identifiers to have an authority "belonging to that of their originating server". The authority of 'ap' URI is a DID, which does not belong to any particular server.

>[!WARNING]
>The URI scheme might be changed to `ap+ef61` in a future version of this document, because these identifiers are not intended to be used for all ActivityPub objects, but only for portable ones.

### Comparing 'ap' URIs

Two 'ap' URIs are equivalent when their canonical forms are identical.

To produce a canonical 'ap' URI, the following operations MUST be performed:

- If the URI is a [compatible identifier](#compatible-ids), convert it into an 'ap' URI.
- If the authority component is percent-encoded, decode it.
- Remove query component.

### DID methods

Implementers MUST support the [did:key] method. Other DID methods SHOULD NOT be used, as it might hinder interoperability.

>[!NOTE]
>The following additional DID methods are being considered: [did:web](https://w3c-ccg.github.io/did-method-web/), [did:dns](https://danubetech.github.io/did-method-dns/), [did:webvh](https://identity.foundation/didwebvh/) (formerly `did:tdw`) and [did:fedi](https://arcanican.is/excerpts/did-method-fedi.html).

To maintain backward compatibility with existing [ActivityPub][ActivityPub] implementations that rely on an origin-based security model and do not canonicalize IDs before comparison, implementers MUST generate DIDs using the base58-btc alphabet, even though the specification [allows both base58-btc and base64url][did:key-syntax]. Using both alphabets in practice could prevent such servers from recognizing that a post whose `attributedTo` value is `https://base64url.example/.well-known/apgateway/did:key:u7QGwDY2Tjn93PVFWWq02piP1NE9_XRlg-c8-jhJiDqKBDw/actor` belongs to `https://base58.example/.well-known/apgateway/did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/actor`.

DID documents SHOULD contain Ed25519 public keys represented as verification methods with `Multikey` type (as defined in the [Controlled Identifiers][Multikey] specification).

Any [DID URL][DID-URL] capabilities of a DID method MUST be ignored when working with 'ap' URIs.

### Dereferencing 'ap' URIs

To dereference an 'ap' URI, the client MUST make HTTP GET request to a gateway endpoint at [well-known] location `/.well-known/apgateway`. The `ap://` prefix MUST be removed from the URI and the rest of it appended to a gateway URI. The client MUST specify an `Accept` header with the `application/ld+json; profile="https://www.w3.org/ns/activitystreams"` media type.

Example of a request to a gateway:

```
GET https://social.example/.well-known/apgateway/did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/path/to/object
```

ActivityPub objects identified by 'ap' URIs can be stored on multiple servers simultaneously.

If object identified by 'ap' URI is stored on the server, it MUST return a response with status `200 OK` containing the requested object. The value of a `Content-Type` header MUST be `application/ld+json; profile="https://www.w3.org/ns/activitystreams"`.

If object identified by 'ap' URI is not stored on the server, it MUST return `404 Not Found`.

If object is not public, the server MUST return `404 Not Found` unless the request has a HTTP signature and the signer is allowed to view the object.

>[!NOTE]
>This document describes web gateways, which use HTTP transport. However, the data model and authentication mechanism are transport-agnostic and other types of gateways could exist.

## Authentication and authorization

Authentication and authorization are performed in accordance with [FEP-fe34] origin-based security model, but with two important differences:

- Cryptographic origins are used. They are similar to web origins described in [RFC-6454] but computed using a different algorithm.
- Authentication via fetching from an origin is not possible. The main authentication method is verification of a signature.

The origin of an 'ap' URI is identical to the authority component of its canonical form (i.e. it is a DID without percent encoding).

The origin of a [DID URL][DID-URL] is identical to its `did` component.

Actors, activities and objects identified by 'ap' URIs MUST contain [FEP-8b32] integrity proofs. Collections identified by 'ap' URIs MAY contain integrity proofs. If collection doesn't contain an integrity proof, [another authentication method](#collections) MUST be used.

The value of `verificationMethod` property of the proof MUST be a [DID URL][DID-URL] where the DID matches the authority component of the 'ap' URI.

>[!NOTE]
>This document uses terms "actor", "activity", "collection" and "object" according to the classification given in [FEP-2277].

## Portable actors

One [DID subject][DID-Subject] can control multiple actors (which are differentiated by the path component of an 'ap' URI).

An actor object identified by 'ap' URI MUST have a `gateways` property containing an ordered list of gateways where the latest version of that actor object can be retrieved. Each item in the list MUST be an HTTP(S) URI with empty path, query and fragment components. The list MUST contain at least one item.

Gateways are expected to be the same for all actors under a DID authority and MAY be also specified in the DID document as [services][DID-Services].

Example:

```json
{
  "@context": [
    "https://www.w3.org/ns/activitystreams",
    "https://w3id.org/security/data-integrity/v1",
    "https://w3id.org/fep/ef61"
  ],
  "type": "Person",
  "id": "ap://did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/actor",
  "inbox": "ap://did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/actor/inbox",
  "outbox": "ap://did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/actor/outbox",
  "gateways": [
    "https://server1.example",
    "https://server2.example"
  ],
  "proof": {
    "type": "DataIntegrityProof",
    "cryptosuite": "eddsa-jcs-2022",
    "created": "2023-02-24T23:36:38Z",
    "verificationMethod": "did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2#z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2",
    "proofPurpose": "assertionMethod",
    "proofValue": "..."
  }
}
```

### Location hints

When ActivityPub object containing a reference to another actor is being constructed, implementations SHOULD provide a list of gateways where specified actor object can be retrieved. This list MAY be provided using the `gateways` query parameter. Each gateway address MUST be URI-endcoded, and if multiple addresses are present they MUST be separated by commas.

Example:

```
ap://did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/actor?gateways=https%3A%2F%2Fserver1.example,https%3A%2F%2Fserver2.example
```

This URI indicates that object can be retrieved from two gateways:

- `https://server1.example`
- `https://server2.example`

> [!IMPORTANT]
> When comparing 'ap' URIs, query parameters are discarded and canonical URIs are used.

### Inboxes and outboxes

Portable inboxes and outboxes function as described in the [ActivityPub] specification. These endpoints are also used to synchronize activities between gateways used by an actor.

Servers specified in the `gateways` property of an actor object MUST accept POST requests targeting its inbox collection.

Example:

```
POST https://social.example/.well-known/apgateway/did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/actor/inbox
```

Activities delivered to an inbox might be not portable. If the server does not accept deliveries on behalf of an actor, it MUST return `404 Not Found`.

Upon receiving an activity in actor's inbox, the server SHOULD forward it to inboxes located on other servers where actor's data is stored. An activity MUST NOT be forwarded from inbox more than once.

Servers specified in the `gateways` property of an actor object MAY accept POST requests targeting its outbox collection. Such servers MUST implement [FEP-ae97].

Activities delivered to an outbox are performed by a portable actor and therefore MUST be portable too. The server MUST verify them as described in section [Authentication and authorization](#authentication-and-authorization) and then process them as described in [FEP-ae97]. Clients MAY deliver activities to multiple outboxes, located on different servers.

Upon receiving an activity in actor's outbox, the server SHOULD forward it to outboxes located on other servers where actor's data is stored. An activity MUST NOT be forwarded from outbox more than once.

### Collections

Collections associated with portable actors (such as inbox and outbox collections) MAY not have [FEP-8b32] integrity proofs. Consuming implementations MUST NOT process unsecured collections retrieved from servers that are not listed in the `gateways` array of the actor document.

## Portable objects

Example:

```json
{
  "@context": [
    "https://www.w3.org/ns/activitystreams",
    "https://w3id.org/security/data-integrity/v1",
    "https://w3id.org/fep/ef61"
  ],
  "type": "Note",
  "id": "ap://did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/objects/dc505858-08ec-4a80-81dd-e6670fd8c55f",
  "attributedTo": "ap://did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/actor?gateways=https%3A%2F%2Fserver1.example,https%3A%2F%2Fserver2.example",
  "inReplyTo": "ap://did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK/objects/f66a006b-fe66-4ca6-9a4c-b292e33712ec",
  "content": "Hello!",
  "attachment": [
    {
      "type": "Image",
      "url": "hl:zQmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n",
      "mediaType": "image/png",
      "digestMultibase": "zQmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n"
    }
  ],
  "to": [
    "ap://did:key:z6MkhaXgBZDvotDkL5257faiztiGiC2QtKLGpbnnEGta2doK/actor"
  ],
  "proof": {
    "type": "DataIntegrityProof",
    "cryptosuite": "eddsa-jcs-2022",
    "created": "2023-02-24T23:36:38Z",
    "verificationMethod": "did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2#z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2",
    "proofPurpose": "assertionMethod",
    "proofValue": "..."
  }
}
```

## Media

Integrity of an external resource is attested with a digest. When a portable object contains a reference to an external resource (such as image), it MUST also contain a [`digestMultibase`](https://w3c.github.io/vc-data-integrity/#resource-integrity) property representing the integrity digest of that resource. The digest MUST be computed using the SHA-256 algorithm.

The URI of an external resource SHOULD be a [hashlink][Hashlinks].

Example of an `Image` attachment:

```json
{
  "type": "Image",
  "url": "hl:zQmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n",
  "mediaType": "image/png",
  "digestMultibase": "zQmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n"
}
```

After retrieving a resource, the client MUST verify its integrity by computing its digest and comparing the result with the value encoded in `digestMultibase` property.

Resources attached to portable objects using hashlinks can be stored by gateways. To retrieve a resource from a gateway, the client MUST make an HTTP GET request to the gateway endpoint at [well-known] location `/.well-known/apgateway`. The value of a hashlink URI MUST be appended to the gateway base URI.

Example of a request:

```
GET https://social.example/.well-known/apgateway/hl:zQmdfTbBqBPQ7VNxZEYEj14VmRuZBkqFbiwReogJgS1zR1n
```

## Compatibility

<a name="compatible-ids"></a>
### Identifiers

'ap' URIs might not be compatible with existing [ActivityPub][ActivityPub] implementations. To provide backward compatibility, gateway-based HTTP(S) URIs of objects can be used instead of their canonical identifiers:

```
https://social.example/.well-known/apgateway/did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/path/to/object
```

Publishers MUST use the first gateway from actor's `gateways` list when constructing compatible identifiers. Consuming implementations that support 'ap' URIs MUST remove the part of the URI preceding `did:` and re-construct the canonical identifier. Objects with the same canonical identifier, but located on different gateways MUST be treated as different instances of the same object.

Publishers MUST NOT add the `gateways` query parameter to object IDs if compatible identifiers are used.

When HTTP signatures are necessary for communicating with other servers, each gateway that makes requests on behalf of an actor SHOULD use a separate secret key. The corresponding public keys MUST be added to actor document using the `assertionMethod` property as described in [FEP-521a].

### WebFinger addresses

WebFinger address of a portable actor can be obtained by the reverse discovery algorithm described in section 2.2 of [ActivityPub and WebFinger][WebFinger] report, but instead of taking the hostname from the identifier, it MUST be taken from the first gateway in actor's `gateways` array.

## Discussion

(This section is non-normative.)

### Discovering locations

#### Arbitrary paths

The `gateways` array can contain HTTP(S) URIs with a path component, thus enabling discovery based on the ["follow your nose"](https://indieweb.org/follow_your_nose) principle, as opposed to discovery based on a [well-known] location.

Example of a compatible object ID if the gateway endpoint is `https://social.example/ap`:

```
https://social.example/ap/did:key:z6MkrJVnaZkeFzdQyMZu1cgjg7k1pZZ6pvBQ7XJPt4swbTQ2/path/to/object
```

#### Alternatives to `gateways` property

This proposal makes use of the `gateways` property, but the following alternatives are being considered:

- `gateways` property in actor's `endpoints` mapping
- `aliases` and [`sameAs`](https://schema.org/sameAs) (containing HTTP(S) URIs of objects)
- `alsoKnownAs` (used for account migrations, so the usage of this property may cause issues)
- `url` (with `alternate` [relation type](https://html.spec.whatwg.org/multipage/links.html#linkTypes))

#### DID services

Instead of specifying gateways in actor document, they can be specified in [DID] document using [DID services](https://www.w3.org/TR/did-core/#services). This approach is not compatible with generative DID methods such as `did:key`, which might be necessary for some types of applications.

### Media access control

The proposed approach to referencing media with hashlinks does not support access control: anybody who knows the hash can retrieve the file.

To work around this limitation, a different kind of identifier can be used where digest is combined with the `ap://` identifier of its parent document. The gateway will not serve media unless parent document ID is provided, and will check whether request signer has permission to view the document and therefore the attached media.

### Compatibility

The following alternatives to gateway-based compatible IDs are being considered:

1. Use regular HTTP(S) URIs but specify the canonical 'ap' URI using the `url` property (with `canonical` relation type, as proposed in [FEP-fffd][FEP-fffd]). For pointers to other objects such as `inReplyTo` property, an embedded object with `url` property can be used instead of a plain URI.
2. Alter object ID depending on the capabilities of the peer (which can be reported by [NodeInfo][NodeInfo] or some other mechanism).

## Implementations

- [Streams](https://codeberg.org/streams/streams/src/commit/6ec6780c7515a638b1ff818559af646fc8e21d94/FEDERATION.md#fediverse-feps)
- [Mitra](https://codeberg.org/silverpill/mitra) (gateway only)
- [fep-ae97-client](https://codeberg.org/silverpill/fep-ae97-client) (client)
- [Forte](https://codeberg.org/fortified/forte/src/commit/ade73e4ed05d0ea2b001abd8e3f2e94c856ac99f/FEDERATION.md#fediverse-feps)
- [tootik](https://github.com/dimkr/tootik/blob/v0.19.0/FEDERATION.md#data-portability)

## References

- Christine Lemmer Webber, Jessica Tallon, [ActivityPub][ActivityPub], 2018
- S. Bradner, [Key words for use in RFCs to Indicate Requirement Levels][RFC-2119], 1997
- T. Berners-Lee, R. Fielding, L. Masinter, [Uniform Resource Identifier (URI): Generic Syntax][RFC-3986], 2005
- Manu Sporny, Dave Longley, Markus Sabadello, Drummond Reed, Orie Steele, Christopher Allen, [Decentralized Identifiers (DIDs) v1.0][DID], 2022
- Dave Longley, Manu Sporny, Markus Sabadello, Drummond Reed, Orie Steele, Christopher Allen, [Controlled Identifiers v1.0][ControlledIdentifiers], 2025
- Dave Longley, Dmitri Zagidulin, Manu Sporny, [The did:key Method v0.7][did:key], 2022
- M. Nottingham, [Well-Known Uniform Resource Identifiers (URIs)][well-known], 2019
- silverpill, [FEP-8b32: Object Integrity Proofs][FEP-8b32], 2022
- silverpill, [FEP-ae97: Client-side activity signing][FEP-ae97], 2023
- silverpill, [FEP-fe34: Origin-based security model][FEP-fe34], 2024
- A. Barth, [The Web Origin Concept][RFC-6454], 2011
- silverpill, [FEP-2277: ActivityPub core types][FEP-2277], 2025
- M. Sporny, L. Rosenthol, [Cryptographic Hyperlinks][Hashlinks], 2021
- silverpill, [FEP-521a: Representing actor's public keys][FEP-521a], 2023
- a, Evan Prodromou, [ActivityPub and WebFinger][WebFinger], 2024
- Adam R. Nelson, [FEP-fffd: Proxy Objects][FEP-fffd], 2023
- Jonne Haß, [NodeInfo][NodeInfo], 2014

[ActivityPub]: https://www.w3.org/TR/activitypub/
[ActivityPub-ObjectIdentifiers]: https://www.w3.org/TR/activitypub/#obj-id
[RFC-2119]: https://datatracker.ietf.org/doc/html/rfc2119.html
[RFC-3986]: https://datatracker.ietf.org/doc/html/rfc3986.html
[RFC-3986-PercentEncoding]: https://datatracker.ietf.org/doc/html/rfc3986#section-2.1
[DID]: https://www.w3.org/TR/did-core/
[DID-Subject]: https://www.w3.org/TR/did-1.0/#did-subject
[did:key]: https://w3c-ccg.github.io/did-key-spec/
[did:key-syntax]: https://w3c-ccg.github.io/did-key-spec/#did-key-identifier-syntax
[DID-URL]: https://www.w3.org/TR/did-core/#did-url-syntax
[DID-Services]: https://www.w3.org/TR/did-1.0/#services
[ControlledIdentifiers]: https://www.w3.org/TR/cid/
[Multikey]: https://www.w3.org/TR/cid/#Multikey
[well-known]: https://datatracker.ietf.org/doc/html/rfc8615
[FEP-8b32]: https://codeberg.org/fediverse/fep/src/branch/main/fep/8b32/fep-8b32.md
[FEP-ae97]: https://codeberg.org/fediverse/fep/src/branch/main/fep/ae97/fep-ae97.md
[FEP-fe34]: https://codeberg.org/fediverse/fep/src/branch/main/fep/fe34/fep-fe34.md
[RFC-6454]: https://www.rfc-editor.org/rfc/rfc6454.html
[FEP-2277]: https://codeberg.org/fediverse/fep/src/branch/main/fep/2277/fep-2277.md
[Hashlinks]: https://datatracker.ietf.org/doc/html/draft-sporny-hashlink-07
[FEP-521a]: https://codeberg.org/fediverse/fep/src/branch/main/fep/521a/fep-521a.md
[WebFinger]: https://swicg.github.io/activitypub-webfinger/
[FEP-fffd]: https://codeberg.org/fediverse/fep/src/branch/main/fep/fffd/fep-fffd.md
[NodeInfo]: https://nodeinfo.diaspora.software/

## Copyright

CC0 1.0 Universal (CC0 1.0) Public Domain Dedication

To the extent possible under law, the authors of this Fediverse Enhancement Proposal have waived all copyright and related or neighboring rights to this work.
