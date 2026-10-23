# Binary installation and upgrades

[中文](installation.zh-CN.md)

Fenghuo installs directly from binaries. The server embeds its frontend; target machines do not need
Go, Node.js, pnpm or Docker. Linux with systemd is recommended. Without systemd, run the programs
in the foreground or use your own process supervisor.

## Choose an archive

Download a published version from [GitHub Releases](https://github.com/mby/fenghuo/releases).
Installers need published archives. If the repository has only source and no first release yet,
build or publish it using the workflow below.

| `uname -m` | Archive architecture |
| --- | --- |
| `x86_64` | `amd64` |
| `aarch64` | `arm64` |
| `armv7l` / `armv8l` | `armv7` (32-bit ARMv7) |

Each release contains six archives, `fenghuo-server_linux_<arch>.tar.gz` and
`fenghuo-agent_linux_<arch>.tar.gz`, plus `checksums.txt`. Archive names do not contain the version;
the download URL selects it. The system needs CA root certificates. Installers require Bash,
curl or wget, tar, sha256sum, coreutils and systemd.

## Deploying from a private repository

The installers download scripts and Release archives over anonymous HTTPS. They do not
read GitHub login credentials or access tokens. The public `curl` commands below cannot
install from a private repository; downloading the script first is insufficient because
its archive download remains anonymous. Generated installation and probe-configuration
commands also use public URLs, so retrieve their files yourself for a private repository.

For testing, push the code to `main` to trigger **Build binaries** automatically, including
in private repositories. Alternatively, sign in to GitHub and use Actions → **Build binaries** →
**Run workflow** to select a branch manually. Download `fenghuo-linux-<full commit SHA>` from that
run's Artifacts and extract the outer ZIP. Transfer the Server/Agent `.tar.gz` files for
your target architecture plus `checksums.txt` to Linux using SCP/SFTP. Follow Method 2
below to verify, extract and install the systemd services. Target machines need neither
GitHub credentials nor source code or build tools.

With GitHub CLI installed, download on a computer with repository read access. Replace
`RUN_ID` and `COMMIT_SHA` with the actual run ID and full commit SHA:

```bash
gh auth login
gh run download RUN_ID --repo mby/fenghuo \
  --name fenghuo-linux-COMMIT_SHA --dir fenghuo-download
```

For an existing private Release, an authenticated CLI can download selected archives:

```bash
gh release download vX.Y.Z --repo mby/fenghuo \
  --pattern 'fenghuo-*_linux_amd64.tar.gz' --pattern checksums.txt --dir fenghuo-download
```

Replace `amd64` with the target architecture. Downloads from Releases in a signed-in
browser work too; use the same manual installation steps afterward. Never put a GitHub
token into the agent's `FENGHUO_TOKEN`, which accepts only this system's node token.
Download and transfer the local probe configuration script from source, then run
`sudo bash configure-probes.sh ...` on the target machine when needed.

Formal releases skip GitHub attestations by default in private repositories, retaining
build checks, archive checksums and publication. With GitHub Enterprise Cloud support
for private attestations, set the repository Actions variable
`ENABLE_PRIVATE_ATTESTATIONS=true` to opt in. Public repositories still attest by default.
See [GitHub attestation availability](https://docs.github.com/en/actions/how-tos/secure-your-work/use-artifact-attestations/use-artifact-attestations)
and [artifact download permissions](https://docs.github.com/en/actions/how-tos/manage-workflow-runs/download-workflow-artifacts).

## Option 1: installer scripts (recommended)

Download and inspect the script before running it on the target Linux machine. The default is the
latest stable Release:

```bash
curl -fL --proto '=https' --tlsv1.2 \
  https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-server.sh \
  -o install-server.sh
less install-server.sh
sudo bash install-server.sh
```

The script verifies the archive's SHA-256, creates the non-login `fenghuo` user, and installs and
starts a systemd service. It preserves an existing `/etc/fenghuo/server.env`. After configuration
changes, run `sudo systemctl restart fenghuo-server`.

To pin both the script and binary, replace `vX.Y.Z` with a real published release tag:

```bash
VERSION=vX.Y.Z
curl -fL --proto '=https' --tlsv1.2 \
  "https://raw.githubusercontent.com/mby/fenghuo/$VERSION/deploy/install-server.sh" \
  -o install-server.sh
less install-server.sh
sudo bash install-server.sh --version "$VERSION"
```

Before installing an agent, add a node in the admin UI and obtain its one-time token. Download and
inspect `https://raw.githubusercontent.com/mby/fenghuo/main/deploy/install-agent.sh`, saving it as
`install-agent.sh`. Use hidden input to avoid placing the token in shell history or command arguments:

```bash
sudo -v
read -r -s -p 'Agent token: ' FENGHUO_TOKEN; printf '\n'
export FENGHUO_TOKEN
sudo --preserve-env=FENGHUO_TOKEN bash install-agent.sh \
  --server https://probe.example.com --idle-interval 15s
unset FENGHUO_TOKEN
```

If your sudo policy does not allow that variable, use the manual installation below to edit
`agent.env`. The installer stores the token in root-only `/etc/fenghuo/agent.env`; systemd runs the
agent as a transient unprivileged user. Idle reporting defaults to 15 seconds. The admin 3/15/30/60
second preset only changes generated install commands. Existing agents require a local edit of
`FENGHUO_IDLE_INTERVAL=15s` and a restart; they accept no remote configuration or automatic upgrades.

## Option 2: manual or offline installation

These steps are for a **new installation**. For existing services, use the upgrade section instead
of overwriting a running executable. Replace `VERSION` with a published tag and choose `ARCH` from
the table. Download and verify in a dedicated directory:

```bash
VERSION=vX.Y.Z
ARCH=amd64
mkdir -p fenghuo-download
cd fenghuo-download
BASE="https://github.com/mby/fenghuo/releases/download/$VERSION"
curl -fL --proto '=https' --tlsv1.2 "$BASE/checksums.txt" -o checksums.txt
curl -fL --proto '=https' --tlsv1.2 "$BASE/fenghuo-server_linux_$ARCH.tar.gz" -o "fenghuo-server_linux_$ARCH.tar.gz"
curl -fL --proto '=https' --tlsv1.2 "$BASE/fenghuo-agent_linux_$ARCH.tar.gz" -o "fenghuo-agent_linux_$ARCH.tar.gz"
sha256sum --check --ignore-missing checksums.txt
```

Skip the other archive if you only need one program. Stop on a failed checksum. For an offline
machine, copy archives and `checksums.txt` from the same release through SCP or removable media
from a trusted computer, then verify again on the target. Checksums detect damaged files; to verify
build provenance on an online computer, use `gh attestation verify <archive> --repo mby/fenghuo`.
Provenance verification applies to formal release archives.

### Server with systemd

Continue in the download directory:

```bash
mkdir -p server
tar -xzf "fenghuo-server_linux_$ARCH.tar.gz" -C server
cd server
id fenghuo >/dev/null 2>&1 || sudo useradd --system --home-dir /var/lib/fenghuo \
  --no-create-home --shell /usr/sbin/nologin fenghuo
sudo install -m 0755 -o root -g root fenghuo-server /usr/local/bin/fenghuo-server
sudo install -m 0644 -o root -g root fenghuo-server.service /etc/systemd/system/fenghuo-server.service
sudo install -d -m 0755 -o root -g root /etc/fenghuo
sudo test -e /etc/fenghuo/server.env || sudo install -m 0600 -o root -g root server.env.example /etc/fenghuo/server.env
sudo systemctl daemon-reload
sudo systemctl enable --now fenghuo-server
sudo systemctl status fenghuo-server --no-pager
```

`StateDirectory` creates `/var/lib/fenghuo` with the correct owner. The public listener defaults to
`127.0.0.1:8080`, and admin to `127.0.0.1:8081`. Put Caddy/nginx with HTTPS in front of the public
listener; `Caddyfile.example` is included. Keep the admin port off the public internet.

Point your domain at the server and allow the reverse proxy's ports 80/443. Example Caddy configuration:

```caddy
probe.example.com {
    encode zstd gzip
    reverse_proxy 127.0.0.1:8080
}
```

On your computer, run `ssh -N -L 8081:127.0.0.1:8081 you@your-server` and open
`http://127.0.0.1:8081/login`. The username is `admin`; retrieve the initial password on the server
with `sudo cat /var/lib/fenghuo/initial_admin_password`. Change it after login, set the public URL
to `https://probe.example.com`, and enable two-factor authentication.

### Agent with systemd

Return to the download directory and extract the agent on the monitored machine:

```bash
mkdir -p agent
tar -xzf "fenghuo-agent_linux_$ARCH.tar.gz" -C agent
cd agent
sudo install -m 0755 -o root -g root fenghuo-agent /usr/local/bin/fenghuo-agent
sudo install -m 0644 -o root -g root fenghuo-agent.service /etc/systemd/system/fenghuo-agent.service
sudo install -d -m 0700 -o root -g root /etc/fenghuo
sudo test -e /etc/fenghuo/agent.env || sudo install -m 0600 -o root -g root /dev/null /etc/fenghuo/agent.env
sudoedit /etc/fenghuo/agent.env
```

Enter your real URL and token in the editor, replacing these placeholders:

```ini
FENGHUO_SERVER=https://probe.example.com
FENGHUO_TOKEN=fh_YOUR_REAL_TOKEN
FENGHUO_INTERVAL=3s
FENGHUO_IDLE_INTERVAL=15s
```

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now fenghuo-agent
sudo systemctl status fenghuo-agent --no-pager
```

Agents only need outbound HTTPS to the server. No inbound ports need to be opened.

### Without systemd

After extracting, an ordinary user can run `./fenghuo-server serve -data ./data`. Data and the
initial password live in that directory; both listeners still default to loopback. The server
stops when the foreground process exits. For production, configure your own supervisor and a restricted user.

Your supervisor can also supply the agent's environment. For a temporary foreground run, write
the settings above into your own private local `agent.env`, then run:

```bash
chmod 600 agent.env
set -a
. ./agent.env
set +a
exec ./fenghuo-agent
```

This sources `agent.env` as shell code: only use a file you wrote yourself. Do not run the agent as root.

## Checks and troubleshooting

```bash
fenghuo-server version
fenghuo-agent -version
curl --fail http://127.0.0.1:8080/api/v1/meta
sudo journalctl -u fenghuo-server -n 100 --no-pager
sudo journalctl -u fenghuo-agent -n 100 --no-pager
```

The dashboard should show its UI and added nodes after they report. `Exec format error` usually means
the wrong architecture. For TLS errors check the domain, certificate, CA roots and system time; for
agent 401 responses check the token. Inspect logs for occupied ports or startup failures. If you
customize the data directory, use that actual path in backup and restore commands too.

## Backup, upgrade and rollback

Order: **stop and back up → upgrade Server → verify login and nodes → upgrade Agent**. Database
migrations run at startup. This backup command is for the default systemd installation above. It
captures the database, configuration, previous server and unit. Backups can contain tokens and sessions;
keep them root-only and copy another instance to a trusted location.

```bash
sudo bash <<'EOF'
set -euo pipefail
umask 077
install -d -m 0700 /var/backups/fenghuo
backup_dir=$(mktemp -d /var/backups/fenghuo/snapshot.XXXXXX)
systemctl stop fenghuo-server
trap 'systemctl start fenghuo-server' EXIT
tar -czf "$backup_dir/backup.tar.gz" -C / var/lib/fenghuo etc/fenghuo \
  usr/local/bin/fenghuo-server etc/systemd/system/fenghuo-server.service
printf 'Backup: %s/backup.tar.gz\n' "$backup_dir"
EOF
```

Then run the reviewed installers, replacing `vX.Y.Z` with your target release:

```bash
sudo bash install-server.sh --version vX.Y.Z
sudo bash install-agent.sh --update --version vX.Y.Z
```

Server preserves configuration and data; agent `--update` preserves local configuration. Installers
stage the executable beside its destination and rename it atomically, then restart the service.
They do not automatically roll back: if startup fails, retain logs, stop the service and investigate.

Do not downgrade just the binary while keeping a migrated database. Stop Server, preserve the current
data separately, then restore the **database, configuration, old binary and unit from the same pre-upgrade
backup**, preserving ownership and permissions. Run `systemctl daemon-reload`, start and verify.
Test extraction and database recovery in an isolated directory or test machine first.

## Uninstall

```bash
sudo bash install-agent.sh --uninstall
sudo bash install-server.sh --uninstall
```

Agent removal deletes the program, unit, `agent.env` and the optional local probe override. Server removal keeps `/var/lib/fenghuo` and
`server.env`. Offline/manual installations can use a reviewed script to uninstall without downloading,
or manually stop/disable the unit and remove the program/unit. Delete data and backups only when no longer needed.

## Generate archives with GitHub Actions

After committing and pushing the changes to GitHub:

1. **Test builds:** pushing to `main` automatically triggers **Build binaries**. Once the default
   branch contains the workflow, Actions → **Build binaries** → **Run workflow** can also select
   a branch manually. Download `fenghuo-linux-<full commit SHA>` from the run's
   Artifacts. Retention is 14 days, and downloading requires GitHub login. Extract the outer ZIP to get
   six tar.gz files and checksums. Binary versions contain `SNAPSHOT`; use the manual installation steps for testing.
2. **Formal releases:** check CI, then tag committed code and push the tag, for example `git tag v1.2.3`
   and `git push origin v1.2.3` (choose your actual version). **Release** runs full checks, builds and
   verifies all six archives, starts amd64 binaries to check the embedded UI and separate listeners,
   and generates provenance before creating the Release for public repositories or opted-in private
   repositories. Tags with a hyphen are prereleases and do
   not become the stable installer entry point.
3. The existing Docker image job runs after successful binary publication. Binary installation does
   not require waiting for or using that image. The test-build workflow does not publish images.

Test builds do not create a Release, deploy anything or update agents. Prefer stable Releases for
production. If only the image job fails, choose **Re-run failed jobs**. A full rerun reuses an existing
published Release only if its archives match the current checksums; different contents or an existing
draft stop the workflow instead of overwriting published versions.
