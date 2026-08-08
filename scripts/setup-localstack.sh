#!/usr/bin/env bash
# =============================================================================
# setup-localstack.sh
#
# Idempotently provisions the VPC Proof Agent AWS lab inside LocalStack
# using the AWS CLI:
#
#   - VPC            10.0.0.0/16
#   - Subnet         10.0.1.0/24  (map-public-ip-on-launch enabled)
#   - Internet GW                attached to the VPC
#   - Route Table                default route 0.0.0.0/0 -> IGW, associated
#                                with the subnet
#   - Security Group             ingress SSH (22) from SSH_CIDR and
#                                HTTP (API_PORT) from API_CIDR
#
# SAFETY: this script forces dummy credentials and the LocalStack endpoint.
# It will NEVER reach a real AWS account, even if real credentials are
# configured on the machine.
#
# Usage:
#   ./scripts/setup-localstack.sh
# =============================================================================
set -euo pipefail

# --- Configuration -----------------------------------------------------------
LOCALSTACK_ENDPOINT="${LOCALSTACK_ENDPOINT:-http://localhost:4566}"
AWS_REGION="${AWS_REGION:-us-east-1}"
SSH_CIDR="${SSH_CIDR:-203.0.113.0/32}"
API_CIDR="${API_CIDR:-0.0.0.0/0}"
API_PORT="${API_PORT:-8080}"

VPC_CIDR="10.0.0.0/16"
SUBNET_CIDR="10.0.1.0/24"

TAG_KEY="vpc-proof:managed"
TAG_VALUE="true"
RESOURCE_TAGS="ResourceType=tag,Tags=[{Key=Name,Value=vpc-proof-lab},{Key=${TAG_KEY},Value=${TAG_VALUE}}]"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.localstack-resources.env"

# --- Safety guards -------------------------------------------------------------
# Refuse to run unless the endpoint is explicitly local.
case "${LOCALSTACK_ENDPOINT}" in
  http://localhost:*|http://127.0.0.1:*|http://localhost.localstack.cloud:*|https://localhost.localstack.cloud:*) ;;
  *)
    echo "ERROR: refusing to run: endpoint '${LOCALSTACK_ENDPOINT}' is not a local LocalStack endpoint" >&2
    exit 1
    ;;
esac

# Force LocalStack-only credentials and drop any inherited session/profile.
export AWS_ACCESS_KEY_ID="test"
export AWS_SECRET_ACCESS_KEY="test"
export AWS_DEFAULT_REGION="${AWS_REGION}"
export AWS_REGION="${AWS_REGION}"
unset AWS_SESSION_TOKEN AWS_SECURITY_TOKEN AWS_PROFILE AWS_DEFAULT_PROFILE AWS_CONFIG_FILE 2>/dev/null || true

# --- AWS CLI wrapper pinned to LocalStack --------------------------------------
aws() {
  command aws --endpoint-url "${LOCALSTACK_ENDPOINT}" --region "${AWS_REGION}" "$@"
}

# --- Helpers ---------------------------------------------------------------------
log() {
  printf '[setup-localstack] %s\n' "$*"
}

# Wait for LocalStack to become ready.
wait_for_localstack() {
  log "waiting for LocalStack at ${LOCALSTACK_ENDPOINT}..."
  for _ in $(seq 1 30); do
    if curl -sf "${LOCALSTACK_ENDPOINT}/_localstack/health" >/dev/null 2>&1; then
      log "LocalStack is ready."
      return 0
    fi
    sleep 2
  done
  echo "ERROR: LocalStack is not reachable at ${LOCALSTACK_ENDPOINT}. Start it with: lstk start" >&2
  exit 1
}

# Find a managed resource by tag; prints empty when absent.
find_managed() {
  local sub="$1" query="$2"
  local out
  out="$(aws ec2 "${sub}" --filters "Name=tag:${TAG_KEY},Values=${TAG_VALUE}" --query "${query}" --output text 2>/dev/null || true)"
  [[ "${out}" == "None" ]] && out=""
  printf '%s' "${out}"
}

# --- Provisioning -----------------------------------------------------------------
wait_for_localstack

# 1. VPC
vpc_id="$(find_managed describe-vpcs 'Vpcs[0].VpcId')"
if [[ -z "${vpc_id}" ]]; then
  vpc_id="$(aws ec2 create-vpc --cidr-block "${VPC_CIDR}" --tag-specifications "${RESOURCE_TAGS}" --query 'Vpc.VpcId' --output text)"
  log "created VPC ${vpc_id} (${VPC_CIDR})"
else
  log "reusing existing VPC ${vpc_id}"
fi

# 2. Subnet (public IP mapping enabled)
# NOTE: LocalStack does not accept tag-specifications on create-subnet,
# so the subnet is created first and tagged afterwards via create-tags.
subnet_id="$(find_managed describe-subnets 'Subnets[0].SubnetId')"
if [[ -z "${subnet_id}" ]]; then
  subnet_id="$(aws ec2 create-subnet --vpc-id "${vpc_id}" --cidr-block "${SUBNET_CIDR}" --query 'Subnet.SubnetId' --output text)"
  aws ec2 modify-subnet-attribute --subnet-id "${subnet_id}" --map-public-ip-on-launch
  aws ec2 create-tags --resources "${subnet_id}" --tags "Key=Name,Value=vpc-proof-lab" "Key=${TAG_KEY},Value=${TAG_VALUE}"
  log "created subnet ${subnet_id} (${SUBNET_CIDR}, auto-assign public IP)"
else
  log "reusing existing subnet ${subnet_id}"
fi

# 3. Internet Gateway + attachment
igw_id="$(find_managed describe-internet-gateways 'InternetGateways[0].InternetGatewayId')"
if [[ -z "${igw_id}" ]]; then
  igw_id="$(aws ec2 create-internet-gateway --tag-specifications "${RESOURCE_TAGS}" --query 'InternetGateway.InternetGatewayId' --output text)"
  aws ec2 attach-internet-gateway --internet-gateway-id "${igw_id}" --vpc-id "${vpc_id}"
  log "created and attached Internet Gateway ${igw_id}"
else
  log "reusing existing Internet Gateway ${igw_id}"
fi

# 4. Route table with default route + subnet association
rtb_id="$(find_managed describe-route-tables 'RouteTables[0].RouteTableId')"
if [[ -z "${rtb_id}" ]]; then
  rtb_id="$(aws ec2 create-route-table --vpc-id "${vpc_id}" --tag-specifications "${RESOURCE_TAGS}" --query 'RouteTable.RouteTableId' --output text)"
  aws ec2 create-route --route-table-id "${rtb_id}" --destination-cidr-block 0.0.0.0/0 --gateway-id "${igw_id}" >/dev/null
  aws ec2 associate-route-table --route-table-id "${rtb_id}" --subnet-id "${subnet_id}" >/dev/null
  log "created route table ${rtb_id} (0.0.0.0/0 -> ${igw_id}) and associated it with ${subnet_id}"
else
  log "reusing existing route table ${rtb_id}"
fi

# 5. Security Group + ingress rules
# NOTE: LocalStack ignores tag-specifications on create-security-group, so the
# SG is created first and tagged afterwards via create-tags (as for the subnet).
sg_id="$(find_managed describe-security-groups 'SecurityGroups[0].GroupId')"
if [[ -z "${sg_id}" ]]; then
  sg_id="$(aws ec2 create-security-group --group-name vpc-proof-sg --description "VPC Proof Agent security group" --vpc-id "${vpc_id}" --query 'GroupId' --output text)"
  aws ec2 create-tags --resources "${sg_id}" --tags "Key=Name,Value=vpc-proof-lab" "Key=${TAG_KEY},Value=${TAG_VALUE}"
  aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port 22 --cidr "${SSH_CIDR}" >/dev/null
  aws ec2 authorize-security-group-ingress --group-id "${sg_id}" --protocol tcp --port "${API_PORT}" --cidr "${API_CIDR}" >/dev/null
  log "created security group ${sg_id} (SSH/22 from ${SSH_CIDR}, HTTP/${API_PORT} from ${API_CIDR})"
else
  log "reusing existing security group ${sg_id}"
fi

# --- Output -------------------------------------------------------------------------
cat > "${ENV_FILE}" <<EOF
VPC_ID=${vpc_id}
SUBNET_ID=${subnet_id}
IGW_ID=${igw_id}
ROUTE_TABLE_ID=${rtb_id}
SECURITY_GROUP_ID=${sg_id}
EOF

echo
echo "=== LocalStack lab provisioned ==="
printf '  VPC            : %s  (%s)\n' "${vpc_id}" "${VPC_CIDR}"
printf '  Subnet         : %s  (%s, public IP on launch)\n' "${subnet_id}" "${SUBNET_CIDR}"
printf '  Internet GW    : %s  (attached to %s)\n' "${igw_id}" "${vpc_id}"
printf '  Route Table    : %s  (0.0.0.0/0 -> %s)\n' "${rtb_id}" "${igw_id}"
printf '  Security Group : %s  (SSH/22 from %s, HTTP/%s from %s)\n' "${sg_id}" "${SSH_CIDR}" "${API_PORT}" "${API_CIDR}"
echo
echo "Resource IDs written to ${ENV_FILE}"
