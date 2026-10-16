# Weir public protocol

Shared public schemas, generated Go types, validation, and owned DNS helpers for
Weir servers and clients. Requires Go 1.27.1.

```sh
go get github.com/batchstream/weir-protocol@v0.12.0
```

The dependency direction is one-way: Weir and the Go SDK depend on this module.
This module depends on neither repository, including through test dependencies.
It contains no peer discovery protocol, server lifecycle, backend adapter, or SDK.

- `api/weir/v1`: unary `weir.v1.StoreService.ResolveStore` and bidirectional
  `Execute` for Read, Mutate, Scan, Native, Query, and Count.
- `api/protocol`: common envelope, endpoint, resource, scan, and query validation.
- `api/netlimit`: standard Go DNS transport with joined cancellation.

The public schema lives in `api/weir/v1/store.proto`. Go imports and `go_package` use
`github.com/batchstream/weir-protocol`. Servers and SDKs consume the module's
generated types. Node-to-node peer schemas and execution DTOs belong to the
Weir server repository.

## Streaming semantics

Execute selects one Store and operation kind for its lifetime. Each Read or Mutate
request carries one record. Servers aggregate admitted records into database
batches; clients send and consume incrementally without collecting a wire batch.
There is no Weir-imposed item or payload-size limit on business calls or individual
records. Backend protocols govern accepted documents and commands; gRPC carries
one request or event per message. Native carries a required opaque Document.
NativeHead carries optional opaque metadata and a content type for subsequent
body chunks. Request and metadata bytes are opaque to the public protocol. Their content types select adapter-owned
formats; new adapters need no protobuf fields or schema changes. Empty request
data is permitted. Unknown valid content types are left to the selected adapter.

Each request is validated before execution. Earlier requests may have effects
when a later request fails validation. The first record index is 1; every later
request advances it by one. UINT64_MAX is invalid. Every record produces one
ExecuteResponse with its ordinal and typed result, in input order.
Servers and clients enforce Store/kind consistency and index sequences. Stateless
validators check each envelope and nested fields. Requests and responses reject unknown
fields throughout the message tree. Responses require declared result/event
variants, enum values and valid completion evidence under the current schema.

ReadResult.missing confirms document absence after a successful read.
TARGET_NOT_FOUND reports a required adapter-owned target that does not exist; it is
an individual Failure, not a missing document result.
Clients can consume results incrementally; SDK convenience collectors may retain
results at the caller's request. Each response confirms one record.

Store names and resource paths are separate fields. Resource paths use canonical percent-encoded segments without a scheme
or leading slash. `ParseRelativeResource` validates and decodes these paths;
`EncodeSegment` constructs individual segments. NativeRequest permits an empty
resource for backend-root operations, including when used as a Query/Count source.
Nonempty Native resources remain canonical
prefixes; the request bytes select the backend command, path and parameters.
Weir imposes no Native operation allowlist; backend authorization applies.

A mutation stream is not a transaction. Mutations to the same resource execute in
input order across requests, including after a failed item; different resources may
run in parallel. Ordering across streams follows the database semantics. APPLIED also includes satisfied no-op semantics such as Lua keep or deleting an
already missing document. An APPLIED result may also
carry a subsequent acknowledgement failure; other outcomes require a Failure.
An individually received MutationResult retains its application evidence if the
stream later fails. A submitted mutation without a result may have applied.
Clients must never automatically replay unacknowledged mutations.

`MutateRequest.options` carries `MutationOptions` uniformly for every action.
Its `visibility` uses `MutationVisibility`: absent options or the zero value
APPLICATION_ACKNOWLEDGED require application acknowledgement. QUERY_VISIBLE
additionally confirms visibility to subsequent normal queries when APPLIED has no
Failure. APPLIED with a Failure retains application evidence but does not confirm
visibility, so replay is still unsafe. Search adapters wait for backend refresh;
adapters without deferred query visibility satisfy this when the mutation is
acknowledged. No-op mutations must also establish the requested visibility.
Unknown option fields and undeclared visibility values are rejected.

Scan uses an opaque adapter-owned filter expression and an optional typed
Projection. Projection requires a uniform include/exclude mode
and nonempty dot-separated field paths; duplicate, ancestor-overlapping,
control characters and wildcard paths are invalid. Literal field names such as
`$field` are allowed by the protocol; adapters enforce their own field rules. An absent Projection returns full
documents. The projection participates in traversal identity.

LuaTransform supplies Source and optional Input with Lua 5.4 semantics and no
runtime selector. The entry point, return actions, helper set and document
conversion rules are stable parts of the contract.
Source returns exactly one function, called as `function(current, incoming)`.
Missing current or omitted Input is nil; documents are ordinary Lua tables.
The callback returns exactly one object to create/replace, or an explicit
`weir.keep()`, `weir.delete()` or `weir.reject(message)` action. Keep/delete on
missing are successful no-ops; reject is PRECONDITION_FAILED/NOT_APPLIED.
Nil, missing/multiple returns, arrays and scalar results are invalid. A callback
error or invalid result never writes. Explicit null is
`weir.null()`. See the server's [Lua guide](https://github.com/batchstream/weir/blob/main/docs/lua.md).

Scan, Native, Query, and Count accept one Command at index 1, followed by client half-close, and
emit typed events at index 1. Scan checkpoints require a matching document count, a terminal
ScanEnd and final gRPC OK. NativeEnd is transport evidence and may be retained when
a later RPC error occurs. Document.content_type identifies the adapter-owned payload format;
read requests do not negotiate a different representation. Backend document limits apply; streams preserve
incremental consumption for Scan and Native responses. A Scan continuation is
bound to its Store, backend profile, and traversal settings. Its checksum detects
corruption only and provides no authentication or authorization. Requests still
validate their Store and adapter-owned resources.

## Query and Count

Query reads one independent offset-based page; Count returns one result. Both
reuse `Execute`, Store routing, admission, and the adapter-owned `NativeRequest`
format. They do not introduce a second RPC path or retain a backend session
between calls. Query uses `Command.query` and emits `Event.document` followed by
one `Event.query_end`. Count uses `Command.count` and emits one
`Event.count_result`. All request and response indexes are 1.

Query page zero selects page 1; page size zero selects 100. There is no Weir
document-count ceiling. `QueryOffset` computes `(page-1)*page_size` in uint64;
adapters reject offsets outside their backend's range before converting them to
signed native values. Backend result windows also apply to the extra result used
to determine `has_more`. Resource exhaustion fails the page; it never silently
returns a shorter successful page. Query does not run Count.

The optional ordered `sort` list replaces native find/Search sorting; an absent
list preserves it. Sort paths are distinct dot-separated native field names,
without wildcards, control characters, or empty segments. Projection follows the
same include/exclude rules as Scan: an explicit INCLUDE or EXCLUDE mode and
nonempty fields are required. Unlike Sink, an empty field list is invalid; omit
the projection to preserve the native query's projection. For aggregate, explicit
sort and projection follow the supplied pipeline, before Query pagination.

MongoDB sources use a database resource and `application/bson` containing an
ordered `find` or read-only `aggregate` command. Query replaces find skip/limit;
Count ignores find pagination and counts matches. Aggregate Count counts the
output of the supplied pipeline. Write stages such as `$out` and `$merge` are
unsupported. Query documents are native BSON objects. These are the Query/Count
adapter requirements; protocol publication alone does not add backend support.

Search sources use an index resource and `application/http`, with the same
origin-form HTTP/1.1 framing as Native, and GET or POST `/_search`. Query replaces
`from`/`size`, while explicit sort and projection replace `sort`/`_source`.
Documents are JSON `_source` objects, matching Weir Scan; they are not Sink's
complete search hits. A query that disables `_source`, or a returned hit without
stored `_source`, must fail instead of substituting an empty object. Count counts
matching documents before pagination or collapse. Partial and timed-out backend
results fail both operations. Exact counts set `estimated=false`; adapters that
return estimates must set `estimated=true`. The first MongoDB and Search
implementations can use exact counts throughout.

`QueryEnd.returned_count` reports the documents actually emitted for this page;
`CountResult.count` reports the total number of matches before Query pagination.
For example, with 250 matching documents and no concurrent changes, page 2 with
`page_size=100` returns `returned_count=100` and `has_more=true`; an independent
Count returns `count=250`. QueryEnd also carries an optional failure and `has_more`.
When `has_more=true`, the page must contain the requested number of documents;
failed pages set `has_more=false`. Accept pagination metadata only after checking
the document count and receiving final gRPC OK. Count failure results contain
zero count and `estimated=false`; successful counts also require final gRPC OK.
Both operations are independent observations: concurrent writes can shift page
positions and make a separate Count differ from Query. Choose a stable sort with
a unique tie-breaker when predictable pages matter. Use Scan for continuation
based traversal.

Shared validation checks the envelope, projection, sort paths, and recursive
unknown fields. Adapters must validate read-only commands and supported options
before execution; embedding NativeRequest does not grant unrestricted Native
execution semantics. Invalid syntax or malformed options use INVALID_ARGUMENT;
unsupported commands, options, or write stages use UNSUPPORTED; exceeded
execution or output capacity uses RESOURCE_EXHAUSTED. Operation failures use the
terminal QueryEnd/CountResult Failure. Invalid Execute envelopes and transport
failures can terminate the RPC without a valid terminal event. Existing Failure
codes retain cancellation, deadline, permission, and backend failure semantics.

Both client and server must support these new command/event variants. Older
schema validators reject them instead of silently ignoring unknown semantics;
clients must not downgrade them to unrestricted Native operations.

## Continuation contract

Scan is read-only and returns documents without independent resource identity.
Continuation format 1 binds the Store, backend profile, resource, filter presence,
content type and exact bytes, and projection presence, mode and ordered fields.
Page size and the supplied token are excluded. The traversal fingerprint is
SHA-256 over named fields, with each name and value prefixed by a 32-bit big-endian
byte length. Its `format` field is `weir.scan.traversal.v1`; it never depends on
protobuf serialization or unknown fields. Fixed vectors protect this encoding.

The internal token envelope identifies its current format. Other formats,
request mismatches and corruption fail before the backend checkpoint is consumed.
The checksum is not a signature; a token cannot bypass Store access or resource
validation. Clients keep tokens opaque and accept a checkpoint only after a
successful terminal ScanEnd, matching document count and final gRPC OK.

Tokens are portable across nodes serving the same Store with an equivalent backend
configuration. MongoDB pages use ordered resume state, provide no cross-page
snapshot and impose no protocol token expiry; changes between pages can affect
the traversal. Search pages share a backend point-in-time snapshot with 60 seconds
of validity, refreshed by successful page requests. Backend deletion, PIT expiry
or incompatible configuration can invalidate a continuation. A failure never
automatically restarts traversal. Only the current token and checkpoint formats
are accepted.

## Validation rules

The current schema and semantics define one execution contract. Requests and
responses reject unknown fields throughout the message tree. FailureCode accepts
only declared nonzero values. MutationOutcome and NativeCompletion require known
evidence values. A known Failure never discards APPLIED evidence or establishes
retry safety.

Native content types, backend expressions and filters retain adapter-owned
semantics. Each adapter documents its accepted formats and limits; opaque bytes
do not waive that contract. Lua helpers, conversions and execution semantics
follow the current contract.

## Validate

```sh
module_dir=$(mktemp -d)
cp go.mod go.sum "$module_dir/"
(cd "$module_dir" && GOWORK=off go mod download all)
GOWORK=off GOPROXY=off GOSUMDB=off go mod verify
GOWORK=off GOPROXY=off GOSUMDB=off go test -race -count=1 ./...
GOWORK=off GOPROXY=off GOSUMDB=off go vet ./...
GOWORK=off GOPROXY=off GOSUMDB=off python3 scripts/check_dependencies.py
python3 -m unittest discover -s scripts -p '*_test.py'
```

Prepare the complete module cache in a temporary module copy before disabling
network access. `download all` includes graph/test metadata that lazy runtime
downloads omit; its extra ZIP checksums stay outside the committed `go.sum`.

Default tests use task-owned loopback DNS and in-memory protocol data. They do not
contact external endpoints, start databases, or change host DNS. Descriptor tests
protect the public RPC/field contract and exclude internal peer definitions.
The dependency check examines every selected module, including transitive and
test requirements, rejects Server/SDK dependencies and replacements, examines
all raw module-graph edges including versioned protocol return edges, then checks
the complete production and test package graph. It forces workspace mode off
for every Go subprocess so unrelated checkouts cannot mask module boundaries.

## Generate

Supply these exact tools externally; they are not stored in the repository:

- protoc 33.4
- protoc-gen-go 1.36.11
- protoc-gen-go-grpc 1.5.1

```sh
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.5.1
# Place the official protoc 33.4 compiler on PATH.
./scripts/generate.sh
git diff --exit-code
```

Use `PROTOC`, `PROTOC_GEN_GO`, `PROTOC_GEN_GO_GRPC`, and `GOFMT` to select explicit
external tool paths. The script checks compiler/plugin versions and preserves the
project's named struct style in generated code. It removes unused compatibility
aliases and unimplemented server scaffolding; implementations provide the complete
current service interface. Regeneration is byte reproducible.

## Publish a stable version

After the reviewed change is merged, an authorized maintainer can publish:

```sh
gh workflow run release.yml --ref main -f version=v0.12.0
```

The workflow accepts only stable `vMAJOR.MINOR.PATCH` versions and runs the complete
offline CI before publication. Its read-only preflight requires a clean checkout,
the validated commit to still be the current remote `main`, and no existing tag
for that version. It then creates the GitHub release and tag at that exact commit.
The preflight script itself never publishes, tags, or pushes. Protocol consumers
should pin the published version rather than a branch name.
