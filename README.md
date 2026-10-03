# Weir public protocol

Shared public schemas, generated Go types, validation, and bounded DNS helpers for
Weir servers and clients. Requires Go 1.27.1.

```sh
go get github.com/batchstream/weir-protocol@v0.1.0
```

The dependency direction is one-way: Weir and the Go SDK depend on this module.
This module depends on neither repository, including through test dependencies.
It contains no peer discovery protocol, server lifecycle, backend adapter, or SDK.

- `api/weir/v1`: `weir.v1.StoreService.ResolveStore` and finite duplex `Execute`,
  with the public Command/Event business DTOs.
- `api/weir/search/v1`: public Search HTTP descriptor DTOs.
- `api/protocol`: common envelope, endpoint, resource and scan validation.
- `api/netlimit`: bounded standard Go DNS transport.

Proto source paths remain `api/weir/v1/store.proto` and
`api/weir/search/v1/http.proto`. Wire packages, field numbers and RPC method paths
are preserved. Go imports and `go_package` use `github.com/batchstream/weir-protocol`.
The independent module owns these schemas; servers and SDKs consume one generated
set rather than regenerate or copy it. Node-to-node peer schemas remain internal
to the Weir server repository.

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
gh workflow run release.yml --ref main -f version=v0.1.0
```

The workflow accepts only stable `vMAJOR.MINOR.PATCH` versions and runs the complete
offline CI before publication. Its read-only preflight requires a clean checkout,
the validated commit to still be the current remote `main`, and no existing tag
for that version. It then creates the GitHub release and tag at that exact commit.
The preflight script itself never publishes, tags, or pushes. Protocol consumers
should pin the published version rather than a branch name.
