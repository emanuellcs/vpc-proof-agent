# Deployment

The VPC Proof Agent is designed to run on an Amazon EC2 instance (Amazon Linux 2) as a systemd service.

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

Runtime configuration is loaded from `/etc/vpc-proof/vpc-proof.env`
(referenced by `EnvironmentFile=-/etc/vpc-proof/vpc-proof.env`).
The `-` prefix means a missing file is not an error; see `.env.example` at the
repository root for the documented variables. Sensitive values (API token)
must live only in this environment file, which should be `chmod 600`.

> The unit runs the `server` subcommand. The `server` Cobra command is part of
> the CLI surface introduced in a later commit; until then the unit is a
> deployment scaffold.
