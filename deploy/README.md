# Deployment

The VPC Proof Agent runs on an Amazon EC2 instance (Amazon Linux 2023) as a hardened systemd service.

## Layout

```
deploy/
├── README.md                       # this file
└── systemd/vpc-proof.service       # systemd unit for the API server
```

## Install

```bash
# 1. Build and copy the binary
make build
sudo cp bin/vpc-proof /usr/local/bin/vpc-proof

# 2. Create the service user
sudo useradd --system --home-dir /var/lib/vpc-proof --create-home vpc-proof

# 3. Create the configuration directory
sudo mkdir -p /etc/vpc-proof

# 4. Install the unit
sudo cp deploy/systemd/vpc-proof.service /etc/systemd/system/
sudo systemctl daemon-reload

# 5. Enable and start
sudo systemctl enable --now vpc-proof

# 6. Inspect
sudo systemctl status vpc-proof
journalctl -u vpc-proof -f
```

## Configuration

Runtime configuration is loaded from `/etc/vpc-proof/vpc-proof.env`, referenced by `EnvironmentFile=-/etc/vpc-proof/vpc-proof.env`.

The `-` prefix means a missing file is not an error; see `.env.example` at the repository root for the documented variables.

Sensitive values (the API token) must live only in this environment file, which should be `chmod 600`.

## Service behavior

The unit runs `vpc-proof serve` as an unprivileged `vpc-proof` user and applies comprehensive systemd hardening: read-only system paths (`ProtectSystem=full`), no new privileges, private `/tmp`, restricted namespaces and capabilities, and clock, hostname, and kernel protections.

The agent's state directory is `/var/lib/vpc-proof` (`StateDirectory=vpc-proof`), so report and history outputs should be written there.

The unit deliberately omits `ProcSubset=pid` because the agent's `default_route` and `system_resources` probes must read host-wide `/proc` files (`/proc/net/route`, `/proc/uptime`, `/proc/loadavg`, `/proc/meminfo`), which that directive hides.

## Testing the systemd unit

Validate the unit file without starting it:

```bash
systemd-analyze verify /etc/systemd/system/vpc-proof.service
```

To exercise the full daemon lifecycle locally, boot a systemd-enabled Amazon Linux 2023 container and install the unit as described above:

```bash
docker run -d --name vpc-proof-sysd --privileged --cgroupns=host \
  -v /sys/fs/cgroup:/sys/fs/cgroup:rw --tmpfs /run --tmpfs /tmp \
  amazonlinux:2023 /usr/sbin/init
```

Notes for the containerized environment:

- Amazon Linux 2023 is the supported platform: Amazon Linux 2 has been discontinued by Amazon and is not used (among other reasons, AL2 ships systemd 219, which does not initialize on cgroup-v2 hosts).
- The base Docker images are minimal, so install systemd first (`dnf install -y systemd`).
- Mask `systemd-networkd-wait-online.service` inside the container; it never completes in Docker and blocks the unit's `network-online.target` ordering.
- Verify the lifecycle end to end: enable/start, `journalctl`, restart-on-failure, graceful stop, and the hardening values exposed by `systemctl show -p <directive>`.
