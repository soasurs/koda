module github.com/soasurs/koda

go 1.26

tool (
	connectrpc.com/connect/cmd/protoc-gen-connect-go
	google.golang.org/protobuf/cmd/protoc-gen-go
)

require (
	connectrpc.com/connect v1.20.0
	github.com/jmoiron/sqlx v1.4.0
	github.com/mattn/go-sqlite3 v1.14.34
	github.com/soasurs/adk v0.0.12
	google.golang.org/protobuf v1.36.11
)

require (
	github.com/google/jsonschema-go v0.4.2 // indirect
	github.com/huandu/go-clone v1.7.3 // indirect
	github.com/huandu/go-sqlbuilder v1.42.1 // indirect
	github.com/huandu/xstrings v1.4.0 // indirect
)
