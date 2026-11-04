# Contributing to the Weir protocol

Focused fixes, documentation improvements, tests and proposals are welcome. Schema
changes reach every client and server, so open an
[issue](https://github.com/batchstream/weir-protocol/issues/new/choose) first and link the coordinated
[server](https://github.com/batchstream/weir) and [SDK](https://github.com/batchstream/weir-go) changes.

## Report a problem

Use the issue forms and include the protocol version, the server or SDK that
consumes it, the expected result and a minimal reproduction. Remove credentials and
private data from logs and configuration.

For vulnerabilities, follow [SECURITY.md](SECURITY.md).

## Work locally

Install the Go version declared in [go.mod](go.mod).

```sh
git clone https://github.com/batchstream/weir-protocol.git
cd weir-protocol
go test -race ./...
go vet ./...
```

Default tests use in-memory protocol data. To regenerate code, install the
[exact tool versions](docs/development.md#generate), then run `./scripts/generate.sh` and
`git diff --exit-code`.

## Prepare a pull request

- Keep the change focused and explain the observed problem and resulting behavior.
- Add a regression test when fixing behavior.
- Commit generated code from `scripts/generate.sh` together with the `.proto` change.
- Keep the dependency direction one-way: this module depends on neither the server nor the SDK.
