#!/usr/bin/env bash
# =============================================================================
# teardown-localstack.sh
#
# Removes every resource tagged `vpc-proof:managed=true` from LocalStack and
# deletes the generated scripts/.localstack-resources.env file. Deleting in
# reverse provisioning order to satisfy AWS dependency rules.
#
# SAFETY: forces dummy credentials and the LocalStack endpoint, exactly like
# setup-localstack.sh. It can never delete resources from a real AWS account.
#
# Usage:
#   ./scripts/teardown-localstack.sh
# =============================================================================
set -euo pipefail

# --- Configuration -----------------------------------------------------------
LOCALSTACK_ENDPOINT="${LOCALSTACK_ENDPOINT:-http://localhost:4566}"
AWS_REGION="${AWS_REGION:-us-east-1}"

TAG_KEY="vpc-proof:managed"
TAG_VALUE="true"

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ENV_FILE="${SCRIPT_DIR}/.localstack-resources.env"

# --- Safety guards -------------------------------------------------------------
case "${LOCALSTACK_ENDPOINT}" in
  http://localhost:*|http://127.0.0.1:*|http://localhost.localstack.cloud:*|https://localhost.localstack.cloud:*) ;;
  *)
    echo "ERROR: refusing to run: endpoint '${LOCALSTACK_ENDPOINT}' is not a local LocalStack endpoint" >&2
    exit 1
    ;;
esac

export AWS_ACCESS_KEY_ID="test"
export AWS_SECRET_ACCESS_KEY="test"
export AWS_DEFAULT_REGION="${AWS_REGION}"
export AWS_REGION="${AWS_REGION}"
unset AWS_SESSION_TOKEN AWS_SECURITY_TOKEN AWS_PROFILE AWS_DEFAULT_PROFILE AWS_CONFIG_FILE 2>/dev/null || true

aws() {
  command aws --endpoint-url "${LOCALSTACK_ENDPOINT}" --region "${AWS_REGION}" "$@"
}

# --- Helpers ---------------------------------------------------------------------
log() {
  printf '[teardown-localstack] %s\n' "$*"
}

find_managed() {
  local sub="$1" query="$2"
  local out
  out="$(aws ec2 "${sub}" --filters "Name=tag:${TAG_KEY},Values=${TAG_VALUE}" --query "${query}" --output text 2>/dev/null || true)"
  [[ "${out}" == "None" ]] && out=""
  printf '%s' "${out}"
}

# --- Teardown (reverse dependency order) -------------------------------------------
log "turning down LocalStack lab (endpoint ${LOCALSTACK_ENDPOINT})"

# 1. Security Group
sg_id="$(find_managed describe-security-groups 'SecurityGroups[0].GroupId')"
if [[ -n "${sg_id}" ]]; then
  aws ec2 delete-security-group --group-id "${sg_id}" >/dev/null 2>&1 || true
  log "deleted security group ${sg_id}"
fi

# 2. Route Table (disassociate first, then delete)
rtb_id="$(find_managed describe-route-tables 'RouteTables[0].RouteTableId')"
if [[ -n "${rtb_id}" ]]; then
  assoc_ids="$(aws ec2 describe-route-tables --route-table-ids "${rtb_id}" --query 'RouteTables[0].Associations[].RouteTableAssociationId' --output text 2>/dev/null || true)"
  for assoc in ${assoc_ids}; do
    [[ "${assoc}" == "None" ]] && continue
    aws ec2 disassociate-route-table --association-id "${assoc}" >/dev/null 2>&1 || true
  done
  aws ec2 delete-route-table --route-table-id "${rtb_id}" >/dev/null 2>&1 || true
  log "deleted route table ${rtb_id}"
fi

# 3. Internet Gateway (detach first, then delete)
igw_id="$(find_managed describe-internet-gateways 'InternetGateways[0].InternetGatewayId')"
if [[ -n "${igw_id}" ]]; then
  vpc_id="$(aws ec2 describe-internet-gateways --internet-gateway-ids "${igw_id}" --query 'InternetGateways[0].Attachments[0].VpcId' --output text 2>/dev/null || true)"
  if [[ -n "${vpc_id}" && "${vpc_id}" != "None" ]]; then
    aws ec2 detach-internet-gateway --internet-gateway-id "${igw_id}" --vpc-id "${vpc_id}" >/dev/null 2>&1 || true
  fi
  aws ec2 delete-internet-gateway --internet-gateway-id "${igw_id}" >/dev/null 2>&1 || true
  log "deleted internet gateway ${igw_id}"
fi

# 4. Subnet
subnet_id="$(find_managed describe-subnets 'Subnets[0].SubnetId')"
if [[ -n "${subnet_id}" ]]; then
  aws ec2 delete-subnet --subnet-id "${subnet_id}" >/dev/null 2>&1 || true
  log "deleted subnet ${subnet_id}"
fi

# 5. VPC
vpc_id="$(find_managed describe-vpcs 'Vpcs[0].VpcId')"
if [[ -n "${vpc_id}" ]]; then
  aws ec2 delete-vpc --vpc-id "${vpc_id}" >/dev/null 2>&1 || true
  log "deleted vpc ${vpc_id}"
fi

# 6. Cleanup generated env file
if [[ -f "${ENV_FILE}" ]]; then
  rm -f "${ENV_FILE}"
  log "removed ${ENV_FILE}"
fi

echo
echo "=== LocalStack lab torn down ==="
