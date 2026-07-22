# Native Remote Desktop Milestone 2 Synthetic Acceptance

Milestone 2 proves the `desktop.v1` encrypted slice without touching a physical display, native input API, operating-system clipboard, privileged helper, installer, or deployed environment. The canonical fixture is a deterministic 640×360, 30 fps, two-second H.264 stream with 60 access units and SHA-256 `370f9e45607d03c62462a4b5bf94ea2617e9c81b31f0dc54170ebebd577a0d14`.

## Acceptance matrix

| Surface | Evidence | Result |
| --- | --- | --- |
| Browser protocol and identity | Canonical transcript/HKDF/record vectors, non-exportable IndexedDB P-256 identity, tamper/replay/epoch tests | Pass |
| Browser decode | WebCodecs and fragmented-MP4/MSE adapters consume the committed AVC fixture; viewer rendered at desktop and 390 px without overflow | Pass |
| macOS synthetic endpoint | Signed fresh-ECDH handshake, 60 fixture units, permission-scoped input/clipboard echo, reconnect, indicator cleanup | Pass |
| Windows synthetic endpoint | Equivalent ECDH/ECDSA/HKDF/AES-GCM, fixture, permission, reconnect, readiness, indicator, and disposal tests | Pass |
| Process relay | Authenticated browser/agent WebSockets exchange signed handshakes and encrypted codec/video/input/clipboard/statistics messages | Pass |
| Reconnect | Fresh side credentials, epoch, ephemeral keys, record keys/nonces, and sequence zero | Pass |
| Termination | Browser, server, and endpoint termination each close both sockets | Pass |
| Content opacity | Known screen, keystroke, and clipboard markers are absent from relay snapshots, lifecycle/audit metadata, logs, HTTP/control responses, and persistence-shaped metadata | Pass |
| PostgreSQL persistence | Demo PostgreSQL network: desktop subset passed three consecutive runs; full `internal/store` package passed without skips | Pass |

## Repeatable gates

From HankServerside:

```bash
go test ./...
make build
make frontend-test
make frontend-check
make frontend-build
```

From Hankagent:

```bash
swift build
swift run hankkit-selftest
cd HankAgent-Windows
/Users/aaroncampbell/.cache/codex-runtimes/hankagent-dotnet/dotnet test tests/HankAgent.Tests/HankAgent.Tests.csproj
```

The process-level harness is `internal/cloud/desktop_e2e_test.go`. It launches a test HTTP/WebSocket cloud boundary, independently authenticates synthetic browser and agent adapters, completes both identity signatures and ephemeral ECDH, then exchanges only AES-GCM records through the opaque relay. It checks the private markers `synthetic-screen-secret`, `synthetic-keystroke-secret`, and `synthetic-clipboard-secret`; only the two synthetic clients recover them.

The PostgreSQL exit row used an isolated source copy and the demo deployment's PostgreSQL Docker network:

```bash
go test -v ./internal/store -run Desktop -count=3
go test ./internal/store -count=1
```

The source hashes matched the local Milestone 2 files. `scripts/doctor.sh` completed with zero failures and zero warnings, and the public `/healthz` and `/readyz` checks remained healthy. No installation, deployment, commit, or push was part of this gate; the live demo application remained on its existing release.

## Milestone 3 continuity note

The Milestone 3 acceptance audit found that the original process-level relay harness used independent synthetic endpoint/browser adapters; production `WorkerAgent` instances connected their desktop socket but did not yet consume the browser handshake or forward `DesktopHostEvent` messages. Milestone 3 now closes that integration gap on both Swift and .NET: focused production-path tests complete the signed handshake, activate the local indicator and host, encrypt inventory/codec/video messages, and process browser termination. The original synthetic cryptographic and relay evidence remains valid, but production end-to-end claims should use the Milestone 3 bridge tests and `native-viewing-acceptance.md` as the current evidence.
