# Weir public protocol

Shared public schemas, generated Go types, validation, and bounded DNS helpers for
Weir servers and clients. Requires Go 1.27.1.

```sh
go get github.com/batchstream/weir-protocol@v0.3.0
```

The dependency direction is one-way: Weir and the Go SDK depend on this module.
This module depends on neither repository, including through test dependencies.
It contains no peer discovery protocol, server lifecycle, backend adapter, or SDK.

- `api/weir/v1`: `weir.v1.StoreService.ResolveStore`, unary `Read`/`Mutate`
  batches and server-streaming `Execute` for one Scan or Native request.
- `api/weir/search/v1`: public Search HTTP descriptor DTOs.
- `api/protocol`: common envelope, endpoint, resource and scan validation.
- `api/netlimit`: bounded standard Go DNS transport.

The public schemas live in `api/weir/v1/store.proto` and
`api/weir/search/v1/http.proto`. Go imports and `go_package` use
`github.com/batchstream/weir-protocol`. Servers and SDKs consume the module's
generated types. Node-to-node peer schemas and execution DTOs belong to the
Weir server repository.

## Batch and streaming semantics

Every Read or Mutate batch selects one Store and uses canonical relative resource
paths. Requests are validated together before any backend operation starts. Both
request and response limits are 32 MiB of complete protobuf encoding, including
envelopes. There is no separate item-count limit. Results match input positions.
Read results distinguish missing documents and individual backend failures.

Store names and resource paths are separate fields. Resource paths are bounded
at 4096 encoded bytes and use canonical percent-encoded segments without a scheme
or leading slash. `ParseRelativeResource` validates and decodes these paths;
`EncodeSegment` constructs individual segments.

A mutation batch is not a transaction. Mutations to the same resource execute in
input order, including after a failed item; different resources may run in parallel.
Ordering across RPCs follows the database semantics. An APPLIED result may also
carry a subsequent acknowledgement failure; other outcomes require a Failure.
A failed unary RPC provides no individual acknowledgements, so every submitted
mutation may have applied. Clients must never automatically replay that batch.

Execute accepts one Command containing Scan or Native and streams one typed Event
per response. Scan checkpoints require a matching document count, a terminal
ScanEnd and final gRPC OK. NativeEnd is transport evidence and may be retained when
a later RPC error occurs. Documents remain bounded at 2 MiB; streams preserve
incremental consumption for Scan and Native responses. A Scan continuation is
bound to its Store, backend profile, and traversal settings. Its checksum detects
corruption; authorization is checked independently on every request.

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
gh workflow run release.yml --ref main -f version=v0.3.0
```

The workflow accepts only stable `vMAJOR.MINOR.PATCH` versions and runs the complete
offline CI before publication. Its read-only preflight requires a clean checkout,
the validated commit to still be the current remote `main`, and no existing tag
for that version. It then creates the GitHub release and tag at that exact commit.
The preflight script itself never publishes, tags, or pushes. Protocol consumers
should pin the published version rather than a branch name.
