# VPC Proof Agent: Evidence Report

| Field | Value |
| --- | --- |
| Overall status | warn |
| Generated at | 2026-08-08 12:00:00 UTC |
| Agent | vpc-proof 1.2.3 (commit abc1234, built 2026-08-08T12:00:00Z) |
| Developer | Emanuel Lázaro (emanuellcs) |
| Go runtime | go1.26.5 |
| Platform | linux/amd64 |
| Integrity hash | `6448c244bce299b34b6321e9c2bc93f7effe29d7c10d66395e15a8002d3af0c0` |

## 1. Instance

| Field | Value |
| --- | --- |
| Instance ID | i-0123456789abcdef0 |
| Availability Zone | us-east-1a |
| Private IP | 10.0.1.42 |
| Public IP | 203.0.113.7 |
| MAC Address | 0a:1b:2c:3d:4e:5f |
| VPC CIDR | 10.0.0.0/16 |
| Subnet CIDR | 10.0.1.0/24 |

## 2. Network Summary

| Field | Value |
| --- | --- |
| Default Gateway | 10.0.2.2 |
| Default Interface | eth0 |
| Primary IP | 10.0.1.42 |
| DNS Resolved Addresses | 203.0.113.10, 203.0.113.11 |

## 3. Summary

| Metric | Count |
| --- | --- |
| Total | 7 |
| Passed | 5 |
| Failed | 1 |
| Warned | 1 |
| Skipped | 0 |

## 4. Probe Results

| ID | Status | Duration | Message |
| --- | --- | --- | --- |
| metadata | pass | 10ms | IMDSv2 metadata accessible |
| vpc_ownership | pass | 20ms | private IP 10.0.1.42 is inside VPC 10.0.0.0/16 |
| subnet_ownership | fail | 30ms | private IP 10.0.1.42 is not inside subnet 10.0.2.0/24 |
| default_route | pass | 40ms | default route via eth0 (gateway 10.0.2.2) |
| dns | pass | 50ms | resolved amazon.com |
| internet_https | pass | 60ms | outbound HTTPS to https://checkip.amazonaws.com succeeded |
| public_ip_consistency | warn | 70ms | instance has no public IP assigned |

## 5. Diagnostics

- **critical**: Verify the EC2 instance was launched in the correct Subnet (10.0.1.0/24).
- **warning**: Ensure the Subnet has 'Auto-assign public IP' enabled and the instance has a public IP associated.

