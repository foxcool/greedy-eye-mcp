module github.com/foxcool/greedy-eye-mcp

go 1.25.5

require (
	connectrpc.com/connect v1.19.1
	github.com/foxcool/greedy-eye v0.0.3
	github.com/mark3labs/mcp-go v0.54.1
	golang.org/x/net v0.55.0
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/santhosh-tekuri/jsonschema/v6 v6.0.2 // indirect
	github.com/spf13/cast v1.7.1 // indirect
	github.com/yosida95/uritemplate/v3 v3.0.2 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/api v0.0.0-20260128011058-8636f8732409 // indirect
)

// Local sibling checkout: the api/v1 package was moved out of internal/ but is
// not yet published in a tag. Works offline, no GOPRIVATE needed.
replace github.com/foxcool/greedy-eye => ../greedy-eye
