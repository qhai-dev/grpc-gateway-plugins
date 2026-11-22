module grpcgateway.conformance

// This module is deliberately separate from the plugin's own module:
// grpc-gateway's runtime and the generated Go types are dependencies of the
// conformance check, and pulling them into the plugin would make the plugin depend
// on the gateway it targets.

go 1.23

require (
	github.com/grpc-ecosystem/grpc-gateway/v2 v2.24.0
	google.golang.org/genproto/googleapis/api v0.0.0-20241223144023-3abc09e42ca8
	google.golang.org/protobuf v1.36.3
)

require (
	golang.org/x/net v0.33.0 // indirect
	golang.org/x/sys v0.28.0 // indirect
	golang.org/x/text v0.21.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20241223144023-3abc09e42ca8 // indirect
	google.golang.org/grpc v1.68.0 // indirect
)
