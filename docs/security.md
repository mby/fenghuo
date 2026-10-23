# Security model

[中文](security.zh-CN.md)

## The problem Fenghuo is built around

In 2026 a widely used open-source probe was exploited at scale: an attacker took over
the dashboard (through a path traversal that leaked the JWT secret, or a
permission check that let a low-privileged user create command tasks), then used
the dashboard's own "run command on agents" feature to install backdoors on
every monitored server at once. The agents ran as root, and their connections
were outbound, so firewalls did not help.

Patching each bug does not fix the design: as long as the agent can execute
what the dashboard sends, the dashboard is a single key to every machine.

## Design

**The agent cannot be controlled.** It is a loop that reads `/proc`, POSTs JSON to
`/api/v1/report` and discards the response body. The protocol has no message type
from server to agent. The only thing the agent takes from a response is which
success code it got: `204` (someone is viewing the dashboard) or `202` (nobody is).
It uses that bit to choose between its two locally configured intervals,
`-interval` and `-idle-interval`. The server cannot name an interval, and the worst
a compromised server can do with it is keep agents at the normal interval. Configuration comes only from local systemd environment files:
`/etc/fenghuo/agent.env` and, optionally, `/etc/fenghuo/probes.env` for TCP targets. The admin probe preset only produces commands an operator runs locally. A test
(`cmd/fenghuo-agent/deps_test.go`) fails the build if the agent ever depends on
a third-party package or on `os/exec`, `plugin`, `net/rpc` and similar packages,
or if its code calls `net.Listen` or `ListenAndServe`.

**The agent has nothing worth stealing.** It runs as a systemd `DynamicUser` with an
empty capability set, `ProtectSystem=strict`, `NoNewPrivileges`, `PrivateDevices`,
`MemoryDenyWriteExecute`, a `@system-service` syscall filter and a 64 MB memory
cap. It listens on no port and writes nothing to disk. The token is read from a
root-only `0600` file by systemd, not from the command line.

**The admin surface is separate.** The server has two listeners. The public one
(`127.0.0.1:8080` behind your TLS proxy) serves only the report endpoint and the
read-only status API. The admin API is not even registered there. It lives on a
second listener bound to `127.0.0.1:8081`, reachable through an SSH tunnel.

**Defence in depth on the server:**

- Single admin account; passwords hashed with argon2id (19 MiB, t=2).
- Optional TOTP two-factor authentication with replay protection.
- Login rate limiting: 10 failures per IP and 100 overall per 15 minutes.
- Server-side sessions stored as SHA-256 hashes, sent in `HttpOnly`,
  `SameSite=Strict` cookies (`Secure` over HTTPS). There are no JWTs and no
  signing secret to leak. Changing the password signs out every session.
- Go's `CrossOriginProtection` rejects cross-origin state-changing requests.
- Strict CSP (`script-src 'self'`, `frame-ancestors 'none'`), `nosniff`,
  `Referrer-Policy: no-referrer`.
- The web UI is embedded in the binary and served from memory, so there is no
  filesystem path to traverse.
- All request bodies are size-limited and decoded with unknown fields rejected;
  reports are range-checked.
- Node tokens are 256-bit random values stored only as hashes and shown once.
- The public API never returns IP addresses, tokens or private notes; hidden
  nodes are only visible to a signed-in admin.
- Outgoing notifications refuse private, loopback, link-local and CGNAT
  addresses after DNS resolution, which blocks SSRF and DNS rebinding.
- The server runs as an unprivileged `fenghuo` user with a hardened unit, or as
  a non-root distroless container.

**Minimal supply chain.** The agent depends only on the Go standard library. The
server adds `modernc.org/sqlite` (pure Go) and `golang.org/x/crypto`. Frontend
dependencies are only needed at build time; pnpm is configured to install only
versions published at least a day ago and to run no install scripts.

## What an attacker gets

| If this is compromised | They can | They cannot |
| --- | --- | --- |
| The admin account or the whole server | See and fake monitoring data, delete nodes, send notifications | Run anything on agents, read files on agents, change agent configuration |
| A node token | Submit fake metrics for that one node | Read anything, affect other nodes |
| The network path (without TLS) | Read metrics and the token | Control the agent; this is why the agent refuses plain `http://` |
| An agent host | Read that host's token | Reach the admin API or other nodes |

The worst case for a full server compromise is a misleading dashboard. The way
to recover is to reinstall the server and rotate node tokens.

## Hardening checklist

- [ ] Change the generated admin password and turn on two-factor authentication.
- [ ] Keep `-admin-listen` on loopback or a private network.
- [ ] Serve the public listener only over HTTPS.
- [ ] Set `-trusted-proxies` to exactly your proxies.
- [ ] Turn off the public status page if you don't need it.
- [ ] Keep the server updated; Go's standard library is compiled into the binary,
      so security fixes arrive through new releases.
- [ ] Rotate a node's token if its host was compromised or decommissioned.

## Verifying releases

Install scripts verify every archive against `checksums.txt` from the same release.
To check by hand:

```bash
sha256sum -c checksums.txt --ignore-missing
```

Each release archive and container image also carries a GitHub build provenance
attestation, which proves it was built by this repository's release workflow:

```bash
gh attestation verify fenghuo-agent_linux_amd64.tar.gz --repo mby/fenghuo
gh attestation verify oci://ghcr.io/mby/fenghuo:latest --repo mby/fenghuo
```

## Reporting a vulnerability

See [SECURITY.md](../SECURITY.md).
