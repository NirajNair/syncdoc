# syncdoc

A CLI tool that securely synchronizes a file between two devices in real-time.

Edit a file together over an encrypted tunnel — share a code, join, start typing and files get synchronized on each save.

## Features

- 🔒 **End-to-end encrypted** — Noise Protocol (XX handshake, ChaCha20-Poly1305)
- 🔗 **Tunneled connection** — host a session via ngrok, peer joins with a code
- ✌️ **Conflict-free editing** — CRDTs merge concurrent edits without conflicts
- 🖥️ **Cross-platform** — macOS, Linux, Windows (amd64 + arm64)
- 🪶 **Single binary** — no runtime dependencies, just download and run

## Installation

### macOS / Linux

```sh
curl -sSL https://raw.githubusercontent.com/NirajNair/syncdoc/master/install.sh | sh
```

### Windows (PowerShell)

```powershell
irm https://raw.githubusercontent.com/NirajNair/syncdoc/master/install.ps1 | iex
```

### Go install

```sh
go install github.com/NirajNair/syncdoc@latest
```

### Build from source

```sh
git clone https://github.com/NirajNair/syncdoc.git
cd syncdoc
go build -o syncdoc .
```

## Quick Start

### As a host

One person starts a session and shares the generated code:

```sh
syncdoc start
```

Output:

```
Session started! Share this code with your peer:

  d3NzOi8vYWJjMTIzLmduZ3Jvay5pbwonVGhpcyBpcyBhIHRva2Vu

Waiting for peer to connect (120s)...
```

Send the code to your peer over any channel — it contains the address and session token.

### As a peer

The other person joins using that code:

```sh
syncdoc join d3NzOi8vYWJjMTIzLmduZ3Jvay5pbwonVGhpcyBpcyBhIHRva2Vu
```

Once connected, both sides edit `syncdoc.txt` in the current directory. Changes sync in real-time — save your file and the other side updates instantly.

## Commands

### `syncdoc start`

Start a sync session as a host. Creates `syncdoc.txt` if it doesn't exist, opens an ngrok tunnel, and prints a joining code for your peer.

### `syncdoc join <code>`

Join an existing session using the code from the host. Overwrites local `syncdoc.txt` with the host's content, then syncs bidirectionally.

### `syncdoc config show`

Print the current configuration (ngrok token, etc.) as JSON.

### `syncdoc config set-ngrok-token <token>`

Save your ngrok authentication token. Required before running `syncdoc start`.

### Global flags

| Flag | Description |
|------|-------------|
| `--debug` | Show verbose debug output |

## Requirements

- **ngrok account** — [Sign up free](https://dashboard.ngrok.com/signup). Only the host needs an auth token.
- **Supported platforms:**

| OS | Arch |
|----|------|
| macOS | amd64, arm64 |
| Linux | amd64, arm64 |
| Windows | amd64, arm64 |

## FAQ

**How does the connection work?**

The host opens an ngrok tunnel, which acts as a relay. Traffic flows between the two machines through ngrok's servers. Your file content is never stored on ngrok — it's only relayed, end-to-end encrypted.

**Is my content encrypted?**

Yes. All communication is end-to-end encrypted using the Noise Protocol (XX handshake with Curve25519 + ChaCha20-Poly1305). Not even ngrok can read your content.

**Do both people need an ngrok token?**

No. Only the **host** (the person running `syncdoc start`) needs an ngrok auth token. The peer only needs the joining code.

**What file does syncdoc sync?**

It syncs a file called `syncdoc.txt` in the current working directory. The host's file content is used as the starting point — the peer's local copy is overwritten on join.

**Can more than two people edit at once?**

Currently syncdoc supports one host and one peer per session. Multi-peer support may be added in the future.

**What happens if we edit the same line?**

syncdoc uses CRDTs (Conflict-free Replicated Data Types) to merge concurrent edits. Both changes are preserved — no data is lost, no manual conflict resolution needed.

**What if the host goes offline?**

The session ends — the host must be online for syncing to work. Both sides retain their local copy of `syncdoc.txt`.

**Can I change the file name or sync multiple files?**

Not currently. syncdoc syncs `syncdoc.txt` in the current directory. Open a feature request if you need this.

**How do I get debug output?**

Run any command with `--debug`:

```sh
syncdoc start --debug
syncdoc join <code> --debug
```

**How do I report a bug or request a feature?**

Open an issue at [github.com/NirajNair/syncdoc/issues](https://github.com/NirajNair/syncdoc/issues).

## License

[Apache License 2.0](LICENSE)
