# Weir public protocol

Shared public schemas, generated Go types, validation, and bounded DNS helpers for
Weir servers and clients. Requires Go 1.27.1.

```sh
go get github.com/batchstream/weir-protocol@v0.8.0
```

The dependency direction is one-way: Weir and the Go SDK depend on this module.
This module depends on neither repository, including through test dependencies.
It contains no peer discovery protocol, server lifecycle, backend adapter, or SDK.

- `api/weir/v1`: unary `weir.v1.StoreService.ResolveStore` and bidirectional
  `Execute` for Read, Mutate, Scan, and Native.
- `api/protocol`: common envelope, endpoint, resource and scan validation.
- `api/netlimit`: bounded standard Go DNS transport.

The public schema lives in `api/weir/v1/store.proto`. Go imports and `go_package` use
`github.com/batchstream/weir-protocol`. Servers and SDKs consume the module's
generated types. Node-to-node peer schemas and execution DTOs belong to the
Weir server repository.

## Streaming semantics

Execute selects one Store and operation kind for its lifetime. Each Read or Mutate
request carries one record. Servers aggregate admitted records into database
batches; clients send and consume incrementally without collecting a wire batch.
There is no item or byte bound on the total call. Documents are bounded at 2 MiB;
Native carries a required Document request bounded at 8 MiB of data, within the
9 MiB Command limit. NativeHead carries optional Document metadata bounded at
64 KiB of data and a content type for subsequent chunks. Request and metadata
bytes are opaque to the public protocol. Their content types select adapter-owned
formats; new adapters need no protobuf fields or schema changes. Empty request
data is permitted. Unknown valid content types are left to the selected adapter.

Each request is validated before execution. Earlier requests may have effects
when a later request fails validation. The first record index is 1; every later
request advances it by one. UINT64_MAX is invalid. Every record produces one
bounded ExecuteResponse with its ordinal and typed result, in input order.
Servers and clients enforce Store/kind consistency and index sequences. Stateless
validators check each envelope, nested fields, and bounds. Requests reject unknown
fields throughout the message tree. Responses tolerate additive ancillary fields
within the same byte bounds, while still requiring recognized result/event variants
and valid completion evidence.

ReadResult.missing confirms document absence after a successful read.
TARGET_NOT_FOUND reports a required adapter-owned target that does not exist; it is
an individual Failure, not a missing document result.
Clients can consume results incrementally; SDK convenience collectors may retain
results at the caller's request. Each response confirms one record.

Store names and resource paths are separate fields. Resource paths are bounded
at 4096 encoded bytes and use canonical percent-encoded segments without a scheme
or leading slash. `ParseRelativeResource` validates and decodes these paths;
`EncodeSegment` constructs individual segments.

A mutation stream is not a transaction. Mutations to the same resource execute in
input order across requests, including after a failed item; different resources may
run in parallel. Ordering across streams follows the database semantics. APPLIED also includes satisfied no-op semantics such as Lua keep or deleting an
already missing document. An APPLIED result may also
carry a subsequent acknowledgement failure; other outcomes require a Failure.
An individually received MutationResult retains its application evidence if the
stream later fails. A submitted mutation without a result may have applied.
Clients must never automatically replay unacknowledged mutations.

Scan uses an opaque adapter-owned filter expression and an optional typed
Projection. Projection requires a uniform include/exclude mode
and nonempty bounded dot-separated field paths; duplicate, ancestor-overlapping,
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

Scan and Native accept one Command at index 1, followed by client half-close, and
emit typed events at index 1. Scan checkpoints require a matching document count, a terminal
ScanEnd and final gRPC OK. NativeEnd is transport evidence and may be retained when
a later RPC error occurs. Document.content_type identifies the adapter-owned payload format;
read requests do not negotiate a different representation. Documents remain bounded at 2 MiB; streams preserve
incremental consumption for Scan and Native responses. A Scan continuation is
bound to its Store, backend profile, and traversal settings. Its checksum detects
corruption only and provides no authentication or authorization. Requests still
validate their Store and adapter-owned resources.

## Continuation contract

Scan is read-only and returns documents without independent resource identity.
Continuation format 1 binds the Store, backend profile, resource, filter presence,
content type and exact bytes, and projection presence, mode and ordered fields.
Page size and the supplied token are excluded. The traversal fingerprint is
SHA-256 over named fields, with each name and value prefixed by a 32-bit big-endian
byte length. Its `format` field is `weir.scan.traversal.v1`; it never depends on
protobuf serialization or unknown fields. Fixed vectors protect this encoding.

The internal token envelope has an explicit version. Unsupported versions,
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
automatically restarts traversal. Rolling upgrades must keep reading issued token
and checkpoint formats for their advertised validity.

## Evolution rules

The current schema and semantics are the baseline. There are no legacy runtime
modes or migration paths. Field numbers, types and meanings are permanent;
removed fields reserve their numbers and names. Semantically new request fields,
operations or options require an explicit contract/RPC version because requests
fail closed. There is no implicit feature negotiation.

Response additions may carry ancillary data without changing the selected
result/event, required fields, completion evidence or byte bounds. A response
containing only an unrecognized oneof variant is invalid. Future positive
FailureCode values are preserved as opaque generic failures; zero and negative
values are invalid. Clients must not infer retry safety from an unknown code or
discard APPLIED evidence when it accompanies a Failure. MutationOutcome and
NativeCompletion are fixed evidence vocabularies and reject unknown values.

Native content types, backend expressions and filters retain adapter-owned
semantics. Each adapter must document and preserve its accepted formats and
limits; opaque bytes do not waive that contract. Changes to Lua helpers,
conversions or execution semantics also require an explicit contract version.

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
project's named struct style in generated code. Regeneration is byte reproducible.

## Publish a stable version

After the reviewed change is merged, an authorized maintainer can publish:

```sh
gh workflow run release.yml --ref main -f version=v0.8.0
```

The workflow accepts only stable `vMAJOR.MINOR.PATCH` versions and runs the complete
offline CI before publication. Its read-only preflight requires a clean checkout,
the validated commit to still be the current remote `main`, and no existing tag
for that version. It then creates the GitHub release and tag at that exact commit.
The preflight script itself never publishes, tags, or pushes. Protocol consumers
should pin the published version rather than a branch name.
