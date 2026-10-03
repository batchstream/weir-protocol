#!/bin/sh
set -eu
cd "$(dirname "$0")/.."

# Tools are supplied externally; generated code is independent of checkout paths.
protoc_bin=$(command -v "${PROTOC:-protoc}")
go_plugin=$(command -v "${PROTOC_GEN_GO:-protoc-gen-go}")
grpc_plugin=$(command -v "${PROTOC_GEN_GO_GRPC:-protoc-gen-go-grpc}")
gofmt_bin=$(command -v "${GOFMT:-gofmt}")
test "$("$protoc_bin" --version)" = 'libprotoc 33.4'
test "$("$go_plugin" --version)" = 'protoc-gen-go v1.36.11'
test "$("$grpc_plugin" --version)" = 'protoc-gen-go-grpc 1.5.1'

"$protoc_bin" \
 --plugin=protoc-gen-go="$go_plugin" --plugin=protoc-gen-go-grpc="$grpc_plugin" \
 --go_out=. --go_opt=module=github.com/batchstream/weir-protocol \
 --go-grpc_out=. --go-grpc_opt=module=github.com/batchstream/weir-protocol \
 api/weir/v1/store.proto
"$protoc_bin" --plugin=protoc-gen-go="$go_plugin" \
 --go_out=. --go_opt=module=github.com/batchstream/weir-protocol \
 api/weir/search/v1/http.proto

# Keep the named-struct-literal convention reproducible in generated Go code.
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/weir-protocol-generate.XXXXXX")
trap 'rm -f "$work_dir"/*.go; rmdir "$work_dir"' EXIT HUP INT TERM
awk '
 $0 == "\treturn &storeServiceClient{cc}" {
  print "\tclient := &storeServiceClient{cc}"; print "\treturn client"; count++; next
 }
 index($0,"\treturn srv.(StoreServiceServer).Execute(m, &grpc.GenericServerStream") == 1 {
  print "\tserverStream := &grpc.GenericServerStream[ExecuteRequest, ExecuteResponse]{ServerStream: stream}"
  print "\treturn srv.(StoreServiceServer).Execute(m, serverStream)"; count++; next
 }
 { print }
 END { if (count != 2) exit 1 }
' api/weir/v1/store_grpc.pb.go > "$work_dir/store_grpc.pb.go"
mv "$work_dir/store_grpc.pb.go" api/weir/v1/store_grpc.pb.go
for go_file in api/weir/v1/store.pb.go api/weir/search/v1/http.pb.go; do
 awk '
  $0 == "\ttype x struct{}" { print; print "\tpackageMarker := x{}"; next }
  { if (sub(/reflect.TypeOf\(x\{\}\)/,"reflect.TypeOf(packageMarker)")) count++; print }
  END { if (count != 1) exit 1 }
 ' "$go_file" > "$work_dir/generated.pb.go"
 mv "$work_dir/generated.pb.go" "$go_file"
done
"$gofmt_bin" -w api/weir/v1/store.pb.go api/weir/v1/store_grpc.pb.go api/weir/search/v1/http.pb.go
