# Next4 source-defined packet and database replay

This is the item-3 evidence for the pinned TrinityCore source at `dcdbc0c5d88eb96f412f69c34bd5b9de2eed5df6`. The Go workflow is source-driven: the reference C++ owners define wire and database behavior, while Go provides deterministic JSON trace recording, replay, and failure comparison. A binary reference capture is not required for this conversion.

| Reference owner | Go owner | Matched boundary |
| --- | --- | --- |
| `WorldSocket::ReadDataHandler` | `engine/world/server.go::Server.Handle` | Decode/decrypt the client frame, retain opcode and raw payload, record client-to-server state before opcode dispatch, then preserve malformed-packet failure behavior. |
| `WorldSocket::SendPacket` | `engine/world/server.go::session.write` | Apply outgoing hooks, retain the final opcode and payload, record server-to-client state before frame encoding/encryption, then write the frame. |
| `DatabaseWorkerPool::Query` and prepared `Query` | `engine/database/statement_registry.go`, `engine/database/pool.go` | Preserve raw/prepared query selection, empty-result failure, async submission, and replayable operation/state metadata. |
| `MySQLConnection::ExecuteTransaction` | `engine/database/migrations.go`, `engine/database/database.go` | Preserve ordered execution, commit/rollback failure paths, and redacted execution results; direct and prepared executions record errors, affected rows, and insert IDs. |

Verification:

- `go run ./tools/packettrace -self-check` passes the JSON trace write/load/diff path.
- `go vet ./...` passes.
- `scripts\build.ps1 moreno` passes.
- Historical focused behavioral tests were removed from the public tree by repository policy; the self-check is source-only evidence and does not claim client or full gameplay parity.
