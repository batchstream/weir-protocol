# Weir public protocol

Shared public schemas, generated Go types, validation, and bounded DNS helpers for
Weir servers and clients. Requires Go 1.27.1.

```sh
go get github.com/batchstream/weir-protocol@v0.6.0
```

The dependency direction is one-way: Weir and the Go SDK depend on this module.
This module depends on neither repository, including through test dependencies.
It contains no peer discovery protocol, server lifecycle, backend adapter, or SDK.

- `api/weir/v1`: unary `weir.v1.StoreService.ResolveStore` and bidirectional
  `Execute` for Read, Mutate, Scan, and Native.
- `api/weir/search/v1`: typed public Search HTTP request/response DTOs.
- `api/protocol`: common envelope, endpoint, resource and scan validation.
- `api/netlimit`: bounded standard Go DNS transport.

The public schemas live in `api/weir/v1/store.proto` and
`api/weir/search/v1/http.proto`. Go imports and `go_package` use
`github.com/batchstream/weir-protocol`. Servers and SDKs consume the module's
generated types. Node-to-node peer schemas and execution DTOs belong to the
Weir server repository.

## Streaming semantics

Execute selects one Store and operation kind for its lifetime. Each Read or Mutate
request carries one record. Servers aggregate admitted records into database
batches; clients send and consume incrementally without collecting a wire batch.
There is no item or byte bound on the total call. Documents are bounded at 2 MiB;
Native selects a BSON mongodb_command or typed search_http request with a body
bounded at 8 MiB, within the 9 MiB Command limit. Search HTTP metadata is bounded
separately at 64 KiB. NativeHead carries a typed HttpResponse for Search, with no
HTTP field for MongoDB replies.

Each request is validated before execution. Earlier requests may have effects
when a later request fails validation. The first record index is 1; every later
request advances it by one. UINT64_MAX is invalid. Every record produces one
bounded ExecuteResponse with its ordinal and typed result, in input order.
Servers and clients enforce Store/kind consistency and index sequences. Stateless
validators check each envelope, nested fields, and bounds; unknown fields are
rejected throughout the message tree.

ReadResult.missing confirms document absence after a successful read.
TARGET_NOT_FOUND reports a required collection/index that does not exist; it is
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

Scan uses a native BSON filter object or JSON Search query object directly, with
an optional typed Projection. Projection requires a uniform include/exclude mode
and nonempty bounded dot-separated field paths; duplicate, ancestor-overlapping,
operator and wildcard paths are invalid. An absent Projection returns full
documents. The projection participates in traversal identity.

LuaTransform supplies Source and optional Input without a runtime selector. Its
current document can be typed missing: merging missing with an object can create
a document, keep/delete on missing are successful no-ops, and reject is a
PRECONDITION_FAILED/NOT_APPLIED result. Nil or no return means keep; a returned
typed object means replace.

Scan and Native accept one Command at index 1, followed by client half-close, and
emit typed events at index 1. Scan checkpoints require a matching document count, a terminal
ScanEnd and final gRPC OK. NativeEnd is transport evidence and may be retained when
a later RPC error occurs. Document.content_type identifies the adapter-owned BSON, JSON, or profile format;
read requests do not negotiate a different representation. Documents remain bounded at 2 MiB; streams preserve
incremental consumption for Scan and Native responses. A Scan continuation is
bound to its Store, backend profile, and traversal settings. Its checksum detects
corruption only and provides no authentication or authorization. Requests still
validate their Store and adapter-owned resources.

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
gh workflow run release.yml --ref main -f version=v0.6.0
```

The workflow accepts only stable `vMAJOR.MINOR.PATCH` versions and runs the complete
offline CI before publication. Its read-only preflight requires a clean checkout,
the validated commit to still be the current remote `main`, and no existing tag
for that version. It then creates the GitHub release and tag at that exact commit.
The preflight script itself never publishes, tags, or pushes. Protocol consumers
should pin the published version rather than a branch name.
