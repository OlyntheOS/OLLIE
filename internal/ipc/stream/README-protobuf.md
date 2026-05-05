# Protobuf IPC Notes

This directory includes a protobuf schema and build-tagged codec.

## Generate code

1. Install `protoc` and the Go protobuf plugin.
2. Run:

```bash
protoc --go_out=. --go_opt=paths=source_relative internal/ipc/stream/pb/ollie_stream.proto
```

3. Build with:

```bash
go test -tags=protobuf ./internal/ipc/stream
```

The `ProtoCodec` is guarded by the `protobuf` build tag. Without it, the codec returns an error to avoid accidental use.
