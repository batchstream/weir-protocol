# Development

How to validate the module, regenerate code and publish a version.

## Module boundaries

The dependency direction is one-way: Weir and the Go SDK depend on this module.
This module depends on neither repository, including through test dependencies.
It contains no server lifecycle, backend adapter, or SDK.

## Validate

```sh
go test -race ./...
go vet ./...
```

Default tests use in-memory protocol data. They do not contact external
endpoints or start databases. Descriptor tests protect the public RPC and field
contract. CI also checks that the module graph contains neither the server nor
the SDK and that `go.mod` replaces no module.

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
It refuses a version already tagged at another commit. Protocol consumers should
pin the published version rather than a branch name.
