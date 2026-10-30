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
 --go-grpc_out=. --go-grpc_opt=module=github.com/batchstream/weir-protocol,require_unimplemented_servers=false \
 api/weir/v1/store.proto

# Keep the named-struct-literal convention reproducible in generated Go code.
work_dir=$(mktemp -d "${TMPDIR:-/tmp}/weir-protocol-generate.XXXXXX")
trap 'rm -f "$work_dir"/*.go; rmdir "$work_dir"' EXIT HUP INT TERM
awk '
 /^\tcodes "google.golang.org\/grpc\/codes"$/ { next }
 /^\tstatus "google.golang.org\/grpc\/status"$/ { next }
 /^\/\/ This type alias is provided for backwards compatibility/ { aliasComments++; next }
 /^type StoreService_Execute(Client|Server) = / { aliases++; next }
 /^\/\/ All implementations should embed UnimplementedStoreServiceServer$/ { next }
 /^\/\/ for forward compatibility\.$/ { next }
 /^\/\/ UnimplementedStoreServiceServer / { scaffolding=1; next }
 scaffolding && /^func RegisterStoreServiceServer\(/ {
  scaffolding=0; registration=1; registrations++
  print; print "\ts.RegisterService(&StoreService_ServiceDesc, srv)"; print "}"; next
 }
 scaffolding { next }
 registration && /^func _StoreService_/ { registration=0 }
 registration { next }
 $0 == "\treturn &storeServiceClient{cc}" {
  print "\tclient := &storeServiceClient{cc}"; print "\treturn client"; count++; next
 }
 index($0,"\treturn srv.(StoreServiceServer).Execute(&grpc.GenericServerStream") == 1 {
  print "\tserverStream := &grpc.GenericServerStream[ExecuteRequests, ExecuteResponses]{ServerStream: stream}"
  print "\treturn srv.(StoreServiceServer).Execute(serverStream)"; count++; next
 }
 { print }
 END { if (count != 2 || aliases != 2 || aliasComments != 2 || registrations != 1) exit 1 }
' api/weir/v1/store_grpc.pb.go > "$work_dir/store_grpc.pb.go"
mv "$work_dir/store_grpc.pb.go" api/weir/v1/store_grpc.pb.go
awk '
  $0 == "\ttype x struct{}" { print; print "\tpackageMarker := x{}"; next }
  { if (sub(/reflect.TypeOf\(x\{\}\)/,"reflect.TypeOf(packageMarker)")) count++; print }
  END { if (count != 1) exit 1 }
 ' api/weir/v1/store.pb.go > "$work_dir/generated.pb.go"
mv "$work_dir/generated.pb.go" api/weir/v1/store.pb.go
"$gofmt_bin" -w api/weir/v1/store.pb.go api/weir/v1/store_grpc.pb.go
