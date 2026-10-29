# Weir public protocol

Shared public schemas, generated Go types and validation for Weir servers and
clients. Requires Go 1.27.1.

```sh
go get github.com/batchstream/weir-protocol@v0.15.0
```

The dependency direction is one-way: Weir and the Go SDK depend on this module.
This module depends on neither repository, including through test dependencies.
It contains no server lifecycle, backend adapter, or SDK.

- `api/weir/v1`: the bidirectional `weir.v1.StoreService.Execute` RPC for Read,
  Mutate, and Native. Clients send it to a Weir server that hosts the Store; any
  other server fails the RPC with NOT_FOUND.
- `api/protocol`: common envelope, Store name, resource, record, and Native
  validation.

The public schema lives in `api/weir/v1/store.proto`. Servers and SDKs consume
the module's generated Go types.

## Streaming semantics

Execute selects one Store and operation kind for its lifetime. Each Read or
Mutate request carries one record. Servers aggregate admitted records into
database batches; clients send and consume incrementally without collecting a
wire batch. The protocol imposes no item or payload-size limit on business calls
or individual records; servers may bound the size of one message. Backend
protocols govern accepted documents and commands; gRPC carries one request or
event per message. Native carries a required opaque Document. NativeHead carries
optional opaque metadata and a content type for subsequent body chunks. Request
and metadata bytes are opaque to the public protocol. Their content types select
adapter-owned formats; new adapters need no protobuf fields or schema changes.
Empty request data is permitted. Unknown valid content types are left to the
selected adapter.

Each request is validated before execution. Earlier requests may have effects
when a later request fails validation. The first record index is 1; every later
request advances it by one. UINT64_MAX is invalid. Every record produces one
ExecuteResponse with its ordinal and typed result, in input order.
Servers and clients enforce Store/kind consistency and index sequences. Stateless
validators check each envelope and nested fields. Requests and responses reject unknown
fields throughout the message tree. Responses require declared result/event
variants, enum values and valid completion evidence.

ReadResult.missing confirms document absence after a successful read.
TARGET_NOT_FOUND reports a required adapter-owned target that does not exist; it is
an individual Failure, not a missing document result.
Clients can consume results incrementally; SDK convenience collectors may retain
results at the caller's request. Each response confirms one record.

Store names and resource paths are separate fields. Resource paths use canonical percent-encoded segments without a scheme
or leading slash. `ParseRelativeResource` validates and decodes these paths;
`EncodeSegment` constructs individual segments. NativeRequest permits an empty
resource for backend-root operations. Nonempty Native resources are canonical
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

## Native backend commands

Native accepts exactly one Command at index 1, followed by client half-close.
Every event uses index 1. A complete response emits NativeHead, zero or more
nonempty body chunks, and NativeEnd with RESPONSE_COMPLETE; its body may be empty.
A failure may emit NativeEnd before any head. Body chunks always follow a head.
NativeHead contains optional opaque metadata and the body's content type.
NativeEnd reports response transport completion, never whether a mutation was
applied. Its evidence may be retained when a later RPC error occurs. Backend
error replies remain native response bodies; clients interpret backend status
and application semantics. Clients must not automatically replay Native calls
whose effects are unknown.

Queries, scans, and counts use Native backend commands. The client owns native
filters, projection, sorting, page sizes, offsets, cursors, and counting semantics.
For MongoDB, send ordered BSON commands such as find, aggregate, getMore, and
count. For Search, send native HTTP requests such as _search, _count, and PIT
requests. Responses retain the complete native reply, including cursor, hit, and
count metadata; Weir does not extract documents or synthesize a page checkpoint.
Clients manage continuation requests and backend cursor/PIT cleanup. Native does
not promise cross-call session affinity, cursor portability, a snapshot, or a
Weir continuation-token format. Backend protocols and permissions apply.

Read and Mutate provide record batching, per-resource ordering, and application
evidence. Native is the direct opaque command path; it does not run record
batching or Lua transforms. This keeps query execution and pagination policy in
the backend and its client.

## Validation rules

Requests and responses reject unknown fields throughout the message tree.
FailureCode accepts only declared nonzero values. MutationOutcome and
NativeCompletion require known evidence values. A known Failure never discards
APPLIED evidence or establishes retry safety.

Native content types and backend expressions retain adapter-owned
semantics. Each adapter documents its accepted formats and limits; opaque bytes
do not waive that contract. Lua helpers, conversions and execution semantics
follow the Lua contract above.

## Validate

```sh
go test -race ./...
go vet ./...
```

Default tests use in-memory protocol data. They do not contact external
endpoints or start databases. Descriptor tests protect the public RPC and field
contract. CI also checks that the module graph contains neither the server nor
the SDK.

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
service interface. Regeneration is byte reproducible.

## Publish a stable version

After the reviewed change is merged, an authorized maintainer can publish:

```sh
gh workflow run release.yml --ref main -f version=vX.Y.Z
```

The workflow accepts only stable `vMAJOR.MINOR.PATCH` versions, runs CI on the
`main` commit, then creates the version tag at that commit and its GitHub release.
Creating the tag fails when the version already exists. Protocol consumers should
pin the published version rather than a branch name.
