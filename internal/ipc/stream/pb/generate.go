//go:build tools

package streampb

//go:generate protoc --go_out=. --go_opt=paths=source_relative ollie_stream.proto
