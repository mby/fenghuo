# Binary installation and upgrades

[中文](installation.zh-CN.md)

Fenghuo installs directly from binaries. The server embeds its frontend; target machines do not need
Go, Node.js, pnpm or Docker. Linux with systemd is recommended. Without systemd, run the programs
in the foreground or use your own process supervisor.

## Choose an archive

Download a published version from [GitHub Releases](https://github.com/mby/fenghuo/releases).
Installers default to the latest stable Release. To install a published prerelease such as
`v1.2.3-rc.1`, specify its exact tag with `--version` or in the download URL; prereleases are not
the default. If there is no published Release yet, use the GitHub Actions test build and manual
installation below.

| `uname -m` | Archive architecture |
| --- | --- |
| `x86_64` | `amd64` |
| `aarch64` | `arm64` |
| `armv7l` / `armv8l` | `armv7` (32-bit ARMv7) |

Each release contains six archives, `fenghuo-server_linux_<arch>.tar.gz` and
`fenghuo-agent_linux_<arch>.tar.gz`, plus `checksums.txt`. Archive names do not contain the version;
the download URL selects it. The system needs CA root certificates. Installers require Bash,
curl or wget, tar, sha256sum, coreutils and systemd.

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
from a trusted computer. On each target, change to the directory containing those files, set `ARCH`
for that machine (for example, `ARCH=amd64`), and run `sha256sum --check --ignore-missing checksums.txt`
again before extracting. Checksums detect damaged files. To verify build provenance on an online computer, use `gh attestation verify <archive> --repo mby/fenghuo`
and `gh attestation verify checksums.txt --repo mby/fenghuo` before transferring the files.
The public Release workflow attests both the archives and `checksums.txt`, including for
prereleases; test-build artifacts do not have these attestations.

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

Upgrade in this order: **stop and back up Server → upgrade Server → verify login and nodes →
back up and upgrade each Agent**. Database migrations run at startup. The commands below assume
the default systemd paths shown above; include any custom data paths and configuration separately.
Backups can contain tokens and sessions: keep them root-only and copy another copy to a trusted
location. Test restoring backups on an isolated machine before relying on them.

### Stop and back up Server

On the Server host, stop the service and back up its data, configuration, binary, unit and optional
unit drop-ins. The service remains stopped after the backup; if the backup fails, resolve the problem
before proceeding.

```bash
sudo bash <<'EOF'
set -euo pipefail
umask 077
install -d -m 0700 /var/backups/fenghuo
backup_dir=$(mktemp -d /var/backups/fenghuo/server.XXXXXX)
systemctl stop fenghuo-server
paths=(var/lib/fenghuo etc/fenghuo usr/local/bin/fenghuo-server
       etc/systemd/system/fenghuo-server.service)
if [ -d /etc/systemd/system/fenghuo-server.service.d ]; then
  paths+=(etc/systemd/system/fenghuo-server.service.d)
fi
tar -czf "$backup_dir/backup.tar.gz" -C / "${paths[@]}"
tar -tzf "$backup_dir/backup.tar.gz" >/dev/null
printf 'Backup: %s/backup.tar.gz\n' "$backup_dir"
EOF
```

### Upgrade Server online

With network access, run the reviewed installer and replace `vX.Y.Z` with the desired published tag.
It verifies the download, preserves the existing configuration and data, replaces the binary and unit,
and starts the service.

```bash
sudo bash install-server.sh --version vX.Y.Z
```

### Upgrade Server from downloaded files

For an offline Release or an Actions test build, copy the appropriate archive and its matching
`checksums.txt` to `~/fenghuo-download` on the Server host. After the stop and backup above, change
`ARCH` below to that host's architecture. The command checks the exact archive listed in the manifest
before replacing the stopped binary and unit. It does not download anything or modify `server.env`.
If you customized the unit itself, reconcile your changes with the packaged unit before replacing it;
the example preserves systemd drop-ins.

```bash
bash <<'EOF'
set -euo pipefail
ARCH=amd64
cd "$HOME/fenghuo-download"
archive="fenghuo-server_linux_${ARCH}.tar.gz"
test -f "$archive"
awk -v file="$archive" '$2 == file {print}' checksums.txt | sha256sum --check -
if sudo systemctl is-active --quiet fenghuo-server; then
  echo 'Stop and back up fenghuo-server first.' >&2
  exit 1
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
tar -xzf "$archive" -C "$work" fenghuo-server fenghuo-server.service
sudo install -m 0755 -o root -g root "$work/fenghuo-server" /usr/local/bin/fenghuo-server
sudo install -m 0644 -o root -g root "$work/fenghuo-server.service" /etc/systemd/system/fenghuo-server.service
sudo systemctl daemon-reload
sudo systemctl start fenghuo-server
sudo systemctl is-active --quiet fenghuo-server
EOF
```

Check `sudo systemctl status fenghuo-server --no-pager` and the dashboard. Verify admin login and node
reports before upgrading Agents. If startup fails, preserve the logs and investigate; upgrades do not
automatically roll back.

### Back up and upgrade each Agent

On each Agent host, stop it and save its configuration, binary, unit and optional drop-ins. The
configuration backup includes local probe overrides. The service stays stopped until upgraded.

```bash
sudo bash <<'EOF'
set -euo pipefail
umask 077
install -d -m 0700 /var/backups/fenghuo
backup_dir=$(mktemp -d /var/backups/fenghuo/agent.XXXXXX)
systemctl stop fenghuo-agent
paths=(etc/fenghuo usr/local/bin/fenghuo-agent
       etc/systemd/system/fenghuo-agent.service)
if [ -d /etc/systemd/system/fenghuo-agent.service.d ]; then
  paths+=(etc/systemd/system/fenghuo-agent.service.d)
fi
tar -czf "$backup_dir/backup.tar.gz" -C / "${paths[@]}"
tar -tzf "$backup_dir/backup.tar.gz" >/dev/null
printf 'Backup: %s/backup.tar.gz\n' "$backup_dir"
EOF
```

With network access, use the reviewed installer; `--update` preserves local configuration:

```bash
sudo bash install-agent.sh --update --version vX.Y.Z
```

For an offline Release or downloaded test build, copy the matching Agent archive and `checksums.txt`
to `~/fenghuo-download` on that Agent host. Set `ARCH` for the host and run the following after the
backup. It verifies the selected archive before replacing the stopped binary and unit; `agent.env`
and probe overrides are preserved. Reconcile any custom changes to the unit itself first.

```bash
bash <<'EOF'
set -euo pipefail
ARCH=amd64
cd "$HOME/fenghuo-download"
archive="fenghuo-agent_linux_${ARCH}.tar.gz"
test -f "$archive"
awk -v file="$archive" '$2 == file {print}' checksums.txt | sha256sum --check -
if sudo systemctl is-active --quiet fenghuo-agent; then
  echo 'Stop and back up fenghuo-agent first.' >&2
  exit 1
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT
tar -xzf "$archive" -C "$work" fenghuo-agent fenghuo-agent.service
sudo install -m 0755 -o root -g root "$work/fenghuo-agent" /usr/local/bin/fenghuo-agent
sudo install -m 0644 -o root -g root "$work/fenghuo-agent.service" /etc/systemd/system/fenghuo-agent.service
sudo systemctl daemon-reload
sudo systemctl start fenghuo-agent
sudo systemctl is-active --quiet fenghuo-agent
EOF
```

Check `sudo systemctl status fenghuo-agent --no-pager` and confirm that the node resumes reporting.

### Server rollback

Do not downgrade only the binary while keeping a migrated database. First stop and preserve the
current database, configuration, binary and unit in a separate backup. Restore the **database,
configuration, old binary and unit from the same pre-upgrade backup**, preserving ownership and
permissions; avoid merging the old database with newer files. Then run `systemctl daemon-reload`,
start the service and verify login and nodes. Restore Agent backups separately on their hosts if needed.

## Docker backup, upgrade and rollback

These steps apply to the supplied `deploy/docker-compose.yml` on a Linux host with a local, rootful Docker
daemon. Its `fenghuo` service uses the `fenghuo-data` named volume at `/data`; Compose normally adds
the project name to the actual volume name. Run the commands from the directory containing that
Compose file, using the same project and Compose options as the existing deployment. Stop any other
writers to the volume as well.

The server image is distroless. Prepare the separate `ubuntu:24.04` helper image on the Docker host
before stopping, for example with `docker pull ubuntu:24.04` while online or by transferring it with
`docker save`/`docker load`. The commands use temporary helper containers with networking disabled;
they do not require a shell or tar in the Fenghuo image.

### Stop and back up the Docker service

The command identifies the existing container, actual local volume and immutable image ID, then stops
the service. It saves the volume, resolved Compose configuration, image reference, image ID, binary
version and image itself. The output names a new root-only backup directory; save that path and copy
the backup to a trusted location. The service stays stopped. A missing helper image, non-named volume,
or non-local volume driver stops the procedure; adapt the backup for other storage drivers.

```bash
sudo bash <<'EOF'
set -euo pipefail
umask 077
helper=ubuntu:24.04
docker image inspect "$helper" >/dev/null
container=$(docker compose ps --all --quiet fenghuo)
test -n "$container"
project=$(docker inspect --format '{{index .Config.Labels "com.docker.compose.project"}}' "$container")
volume=$(docker inspect --format '{{range .Mounts}}{{if eq .Destination "/data"}}{{.Name}}{{end}}{{end}}' "$container")
image_ref=$(docker inspect --format '{{.Config.Image}}' "$container")
image_id=$(docker inspect --format '{{.Image}}' "$container")
test -n "$project" && test -n "$volume" && test -n "$image_id"
test "$(docker volume inspect --format '{{.Driver}}' "$volume")" = local
docker compose stop fenghuo
install -d -m 0700 /var/backups/fenghuo
backup_dir=$(mktemp -d /var/backups/fenghuo/docker.XXXXXX)
printf '%s\n' "$project" > "$backup_dir/project.txt"
printf '%s\n' "$volume" > "$backup_dir/volume.txt"
printf '%s\n' "$image_ref" > "$backup_dir/image-ref.txt"
printf '%s\n' "$image_id" > "$backup_dir/image-id.txt"
docker compose config > "$backup_dir/compose.yaml"
docker image save -o "$backup_dir/image.tar" "$image_id"
docker run --rm --pull=never --network none "$image_id" version > "$backup_dir/version.txt"
docker run --rm --pull=never --network none --user 0:0 \
  --mount "type=volume,src=$volume,dst=/data,readonly" \
  --mount "type=bind,src=$backup_dir,dst=/backup" \
  "$helper" tar --numeric-owner -C /data -czf /backup/data.tar.gz .
docker run --rm --pull=never --network none \
  --mount "type=bind,src=$backup_dir,dst=/backup,readonly" \
  "$helper" tar -tzf /backup/data.tar.gz >/dev/null
(
  cd "$backup_dir"
  sha256sum data.tar.gz image.tar compose.yaml image-id.txt image-ref.txt \
    version.txt project.txt volume.txt > SHA256SUMS
  sha256sum -c SHA256SUMS
)
printf 'Backup: %s\n' "$backup_dir"
EOF
```

### Upgrade the Docker service

After the backup, use the intended published `ghcr.io/mby/fenghuo` image tag in the Compose file.
With network access, pull it and start the service:

```bash
docker compose pull fenghuo
docker compose up -d --no-deps --pull never fenghuo
docker compose ps fenghuo
docker compose logs --tail 100 fenghuo
```

For an offline upgrade, transfer and `docker load` the intended image first, ensure its tag matches
the Compose file, skip `docker compose pull` and use the remaining commands. Check the logs, login and
node reports. The pre-upgrade volume backup and image belong together if a database migration occurs.

### Roll back the Docker service

**Before restoring, preserve the current state:** repeat the Docker backup procedure above into a
new directory, even if the upgraded container has stopped. It stops the service and saves the current
volume, configuration and image. Keep that new backup separately; for the command below, set
`backup_dir` to the **earlier pre-upgrade** backup.

The command verifies that backup, restores it to a new named volume with the helper container,
loads the matching old image, and starts the saved Compose configuration with an override. It leaves
the original volume intact and avoids mixing restored files with newer database files.

```bash
sudo bash <<'EOF'
set -euo pipefail
backup_dir=/var/backups/fenghuo/docker.REPLACE
helper=ubuntu:24.04
docker image inspect "$helper" >/dev/null
(
  cd "$backup_dir"
  sha256sum -c SHA256SUMS
)
project=$(cat "$backup_dir/project.txt")
old_image=$(cat "$backup_dir/image-id.txt")
[[ "$project" =~ ^[a-z0-9][a-z0-9_-]*$ ]]
[[ "$old_image" =~ ^sha256:[0-9a-f]{64}$ ]]
if [ -n "$(docker ps --quiet --filter "label=com.docker.compose.project=$project" \
    --filter "label=com.docker.compose.service=fenghuo")" ]; then
  echo 'Stop and back up the current Docker service first.' >&2
  exit 1
fi
restore_volume="fenghuo-restore-$(date +%s)-$$"
if docker volume inspect "$restore_volume" >/dev/null 2>&1; then
  echo 'Restore volume already exists.' >&2
  exit 1
fi
docker volume create "$restore_volume" >/dev/null
docker run --rm --pull=never --network none --user 0:0 \
  --mount "type=volume,src=$restore_volume,dst=/data" \
  --mount "type=bind,src=$backup_dir,dst=/backup,readonly" \
  "$helper" tar --numeric-owner --same-permissions -C /data -xzf /backup/data.tar.gz
docker image load -i "$backup_dir/image.tar"
rollback_tag="ghcr.io/mby/fenghuo:rollback-${old_image#sha256:}"
docker image tag "$old_image" "$rollback_tag"
cat > "$backup_dir/rollback.override.yaml" <<YAML
services:
  fenghuo:
    image: $rollback_tag
volumes:
  fenghuo-data:
    external: true
    name: $restore_volume
YAML
docker compose -p "$project" -f "$backup_dir/compose.yaml" \
  -f "$backup_dir/rollback.override.yaml" up -d --no-deps --pull never fenghuo
docker compose -p "$project" -f "$backup_dir/compose.yaml" \
  -f "$backup_dir/rollback.override.yaml" ps fenghuo
printf 'Restored volume: %s\nProject: %s\nCompose files: %s/compose.yaml and %s/rollback.override.yaml\n' \
  "$restore_volume" "$project" "$backup_dir" "$backup_dir"
EOF
```

After verifying the restored login and nodes, continue using the printed Compose files and project
name, or update your normal Compose configuration to use the restored volume and old image. Running
the unmodified newer Compose file could switch back to the newer volume or image. Keep both backups
and volumes until recovery is verified.

## Uninstall

```bash
sudo bash install-agent.sh --uninstall
sudo bash install-server.sh --uninstall
```

Agent removal deletes the program, unit, `agent.env` and the optional local probe override. Server removal keeps `/var/lib/fenghuo` and
`server.env`. Offline/manual installations can use a reviewed script to uninstall without downloading,
or manually stop/disable the unit and remove the program/unit. Delete data and backups only when no longer needed.

## Generate archives with GitHub Actions

These workflows run in the public `mby/fenghuo` repository:

1. **Test builds:** pushing to `main` triggers **Build binaries**. Once the default branch contains
   the workflow, Actions → **Build binaries** → **Run workflow** can also select a branch.
   The workflow runs checks, builds a GoReleaser snapshot, verifies all six archives and smoke-tests
   the amd64 binaries. Sign in to GitHub and download `fenghuo-linux-<full commit SHA>` from the
   run's Artifacts within 14 days. Extract the outer ZIP to obtain the six tar.gz files and
   `checksums.txt`. These are test artifacts, not a Release; verify and install them using the
   manual steps above, skipping the Release download commands.
2. **Published releases:** pushing a tag matching `v*` triggers **Release**. For example, after
   checking CI, tag committed code with `git tag v1.2.3` and push it with
   `git push origin v1.2.3`, choosing the actual version. The workflow runs `make check` and
   ShellCheck, builds and verifies all six archives, checks the amd64 binary versions and starts
   the server to check its embedded UI and separate listeners. It attests the archives and
   `checksums.txt` before publishing the Release. Any tag containing a hyphen is marked as a
   prerelease and is not the default stable installer entry point.
3. **Container image:** after the binary job succeeds, Release builds and publishes the Docker
   image. Binary installation does not require waiting for or using that image. The test-build
   workflow does not publish images.

To download a test artifact with GitHub CLI, sign in and replace `RUN_ID` and `COMMIT_SHA`
with the run ID and full commit SHA:

```bash
gh run download RUN_ID --repo mby/fenghuo \
  --name fenghuo-linux-COMMIT_SHA --dir fenghuo-download
```

### Install a downloaded test build

Extract the downloaded Actions Artifact ZIP on a trusted computer, then copy the required archive
and its matching `checksums.txt` to the target. On the target, change to that directory, set `ARCH`
for the machine and verify the copied files. No Release download or installer script is used here.

```bash
cd /path/to/fenghuo-download
ARCH=amd64
sha256sum --check --ignore-missing checksums.txt
```

Stop if verification fails. For a new installation, continue with the relevant manual Server or Agent
steps above. For an existing service, first stop and back it up, then use the offline upgrade steps
above; those commands check the exact archive again before replacement. Test builds do not create a
Release, deploy anything or update agents. Prefer stable Releases for production.

If only the image job fails, choose **Re-run failed jobs**. A full rerun retains an existing
published Release only after its checksum file matches the new build and all six downloaded
archives pass verification. A mismatch or an existing draft stops publication instead of
overwriting published assets.
