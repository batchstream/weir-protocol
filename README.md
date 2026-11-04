# Weir public protocol

[![CI](https://github.com/batchstream/weir-protocol/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/batchstream/weir-protocol/actions/workflows/ci.yml) [![Go Reference](https://pkg.go.dev/badge/github.com/batchstream/weir-protocol.svg)](https://pkg.go.dev/github.com/batchstream/weir-protocol) [![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

The public gRPC schema, generated Go types and validation shared by Weir servers
and clients.

## Overview

Applications do not need this module directly. Use the
[Go SDK](https://github.com/batchstream/weir-go). Use this module to write another
client or to change the protocol.

### Contents

| Path | Contents |
| --- | --- |
| `api/weir/v1/store.proto` | The public schema |
| `api/weir/v1` | Generated Go types for the `weir.v1.StoreService.Execute` RPC |
| `api/protocol` | Validation of envelopes, Store names, resources, records and Native requests |

### Terms

| Term | Meaning |
| --- | --- |
| Execute | The only RPC. It is a bidirectional stream. One stream selects one Store and one operation kind. |
| Record | One Read or Mutate request. The first record index is 1. Each later record adds 1. |
| Result | The response to one record. Results arrive in completion order, so match them by index. |
| Native | One opaque backend command, such as a query or a scan. It does not run record batching. |

### Dependency direction

The server and the SDK depend on this module. This module depends on neither.

## Requirements

| Item | Requirement |
| --- | --- |
| Go | 1.27.3 |

## Quick start

This encodes a resource path and validates it.

**Step 1.** Create a module and add the protocol.

```sh
mkdir hello
cd hello
go mod init example.com/hello
go get github.com/batchstream/weir-protocol/api/protocol@v0.17.0
```

**Step 2.** Save this as `main.go`.

```go
package main

import (
	"fmt"
	"log"

	"github.com/batchstream/weir-protocol/api/protocol"
)

func main() {
	resource := protocol.EncodeSegment("my db") + "/records/s:hello"
	fmt.Println(resource)

	segments, err := protocol.ParseRelativeResource(resource)
	if err != nil {
		log.Fatal(err)
	}
	fmt.Println(segments)
}
```

**Step 3.** Run it.

```sh
go run .
```

Expected output:

```text
my%20db/records/s:hello
[my db records s:hello]
```

## Documentation

| Guide | Contents |
| --- | --- |
| [Semantics](docs/semantics.md) | Streams, deadlines, cancellation, mutations, Lua, Native and validation rules |
| [Development](docs/development.md) | Module boundaries, code generation and releases |
| [API reference](https://pkg.go.dev/github.com/batchstream/weir-protocol) | Every exported type and function |

## Development

```sh
go test -race ./...
go vet ./...
```

Default tests use in-memory data. To change the schema, edit
`api/weir/v1/store.proto` and follow [Generate](docs/development.md#generate).

- Contributing: [CONTRIBUTING.md](CONTRIBUTING.md)
- Security reports: [SECURITY.md](SECURITY.md)

## Related repositories

| Repository | Purpose |
| --- | --- |
| [weir](https://github.com/batchstream/weir) | Server |
| [weir-go](https://github.com/batchstream/weir-go) | Go client SDK |
| **[weir-protocol](https://github.com/batchstream/weir-protocol)** | Public gRPC schema and validation (this repository) |
| [weir-charts](https://github.com/batchstream/weir-charts) | Helm chart |
| [weir-tests](https://github.com/batchstream/weir-tests) | System tests and benchmarks |

## License

MIT. See [LICENSE](LICENSE).
