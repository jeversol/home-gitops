#!/usr/bin/env bash

set -Eeuo pipefail

PROGRAM="${0##*/}"
PIRAEUS_NAMESPACE="${PIRAEUS_NAMESPACE:-piraeus-datastore}"
PVC_NAMESPACE=""
ASSUME_YES=false

usage() {
  cat <<EOF
Usage: ${PROGRAM} [OPTIONS] PVC_NAME

Trim unused blocks from a mounted LINSTOR-backed PVC and report LINSTOR
replica allocation and storage-pool capacity before and after the operation.

The PVC namespace is discovered automatically when its name is unique across
the cluster. Use --namespace when multiple namespaces contain the same name.

Options:
  -n, --namespace NAMESPACE  Namespace containing the PVC
  -y, --yes                  Skip the confirmation prompt
  -h, --help                 Show this help

Environment:
  PIRAEUS_NAMESPACE          LINSTOR namespace (default: ${PIRAEUS_NAMESPACE})

Examples:
  ${PROGRAM} storage-mimir-compactor-0
  ${PROGRAM} -n o11y storage-mimir-ingester-0
  ${PROGRAM} --yes -n media config-plex-0
EOF
}

die() {
  printf 'Error: %s\n' "$*" >&2
  exit 1
}

info() {
  printf '\n==> %s\n' "$*"
}

require_command() {
  command -v "$1" >/dev/null 2>&1 || die "required command not found: $1"
}

find_linstor_cli() {
  if command -v kubectl-linstor >/dev/null 2>&1; then
    command -v kubectl-linstor
    return
  fi

  local krew_candidate="${KREW_ROOT:-${HOME}/.krew}/bin/kubectl-linstor"
  if [[ -x "${krew_candidate}" ]]; then
    printf '%s\n' "${krew_candidate}"
    return
  fi

  die "kubectl-linstor was not found; install the LINSTOR Krew plugin"
}

format_kib() {
  awk -v kib="$1" 'BEGIN {
    if (kib >= 1048576) printf "%.2f GiB", kib / 1048576;
    else if (kib >= 1024) printf "%.2f MiB", kib / 1024;
    else printf "%.0f KiB", kib;
  }'
}

get_volume_summary() {
  # LINSTOR's raw machine-readable response contains DRBD connection secrets.
  # Reduce it in the pipeline so secrets are never printed or persisted.
  "${LINSTOR_CLI}" --machine-readable volume list -r "${RESOURCE_NAME}" |
    jq -c '[.[][] | {
      node: .node_name,
      in_use: (.state.in_use // false),
      pool: .volumes[0].storage_pool_name,
      provider: .volumes[0].provider_kind,
      allocated_kib: .volumes[0].allocated_size_kib,
      usable_kib: .volumes[0].layer_data_list[] |
        select(.type == "DRBD") | .data.usable_size_kib,
      minor: .volumes[0].layer_data_list[] |
        select(.type == "DRBD") | .data.drbd_volume_definition.minor_number,
      state: .volumes[0].state.disk_state,
      replication: [.volumes[0].state.replication_states[].replication_state]
    }]'
}

get_pool_summary() {
  "${LINSTOR_CLI}" --machine-readable storage-pool list \
    -n "${RESOURCE_NODES[@]}" -s "${RESOURCE_POOLS[@]}" |
    jq -c '[.[][] | {
      node: .node_name,
      pool: .storage_pool_name,
      provider: .provider_kind,
      free_kib: .free_capacity,
      total_kib: .total_capacity
    }]'
}

print_volume_summary() {
  local summary="$1"

  printf '%-16s %-9s %14s %14s %-12s %s\n' \
    'NODE' 'ROLE' 'ALLOCATED' 'USABLE' 'STATE' 'REPLICATION'
  jq -r '.[] | [
      .node,
      (if .in_use then "primary" else "replica" end),
      (.allocated_kib | tostring),
      (.usable_kib | tostring),
      .state,
      (.replication | join(","))
    ] | @tsv' <<<"${summary}" |
    while IFS=$'\t' read -r node role allocated usable state replication; do
      printf '%-16s %-9s %14s %14s %-12s %s\n' \
        "${node}" "${role}" "$(format_kib "${allocated}")" \
        "$(format_kib "${usable}")" "${state}" "${replication}"
    done
}

print_pool_summary() {
  local summary="$1"

  printf '%-16s %-10s %14s %14s %9s\n' \
    'NODE' 'POOL' 'FREE' 'TOTAL' 'FREE %'
  jq -r '.[] | [
      .node,
      .pool,
      (.free_kib | tostring),
      (.total_kib | tostring),
      ((100 * .free_kib / .total_kib) | tostring)
    ] | @tsv' <<<"${summary}" |
    while IFS=$'\t' read -r node pool free total percent; do
      printf '%-16s %-10s %14s %14s %8.1f%%\n' \
        "${node}" "${pool}" "$(format_kib "${free}")" \
        "$(format_kib "${total}")" "${percent}"
    done
}

while [[ $# -gt 0 ]]; do
  case "$1" in
    -n|--namespace)
      [[ $# -ge 2 ]] || die "$1 requires a namespace"
      PVC_NAMESPACE="$2"
      shift 2
      ;;
    -y|--yes)
      ASSUME_YES=true
      shift
      ;;
    -h|--help)
      usage
      exit 0
      ;;
    --)
      shift
      break
      ;;
    -*)
      die "unknown option: $1"
      ;;
    *)
      break
      ;;
  esac
done

[[ $# -eq 1 ]] || {
  usage >&2
  exit 2
}

PVC_NAME="$1"

require_command kubectl
require_command jq
require_command awk
LINSTOR_CLI="$(find_linstor_cli)"

if [[ -z "${PVC_NAMESPACE}" ]]; then
  PVC_MATCHES="$(kubectl get pvc --all-namespaces -o json |
    jq -c --arg name "${PVC_NAME}" '[.items[] | select(.metadata.name == $name) | .metadata.namespace]')"
  PVC_MATCH_COUNT="$(jq 'length' <<<"${PVC_MATCHES}")"

  case "${PVC_MATCH_COUNT}" in
    0) die "PVC ${PVC_NAME} was not found in any namespace" ;;
    1) PVC_NAMESPACE="$(jq -r '.[0]' <<<"${PVC_MATCHES}")" ;;
    *) die "PVC ${PVC_NAME} exists in multiple namespaces; specify --namespace" ;;
  esac
fi

PVC_JSON="$(kubectl -n "${PVC_NAMESPACE}" get pvc "${PVC_NAME}" -o json)"
PVC_STATUS="$(jq -r '.status.phase // ""' <<<"${PVC_JSON}")"
[[ "${PVC_STATUS}" == Bound ]] || die "PVC is not Bound (status: ${PVC_STATUS:-unknown})"

RESOURCE_NAME="$(jq -r '.spec.volumeName // ""' <<<"${PVC_JSON}")"
[[ -n "${RESOURCE_NAME}" ]] || die "PVC has no bound PersistentVolume"

VOLUME_MODE="$(jq -r '.spec.volumeMode // "Filesystem"' <<<"${PVC_JSON}")"
[[ "${VOLUME_MODE}" == Filesystem ]] || die "only Filesystem PVCs can be trimmed"

if jq -e '.spec.accessModes | index("ReadWriteOncePod")' <<<"${PVC_JSON}" >/dev/null; then
  die "ReadWriteOncePod PVCs cannot be mounted by the online helper pod"
fi

PV_JSON="$(kubectl get pv "${RESOURCE_NAME}" -o json)"
CSI_DRIVER="$(jq -r '.spec.csi.driver // ""' <<<"${PV_JSON}")"
[[ "${CSI_DRIVER}" == linstor.csi.linbit.com ]] || \
  die "PV is not managed by LINSTOR CSI (driver: ${CSI_DRIVER:-unknown})"

FILESYSTEM="$(jq -r '.spec.csi.fsType // ""' <<<"${PV_JSON}")"
[[ -n "${FILESYSTEM}" ]] || die "PV does not declare a filesystem type"

ATTACHMENTS="$(kubectl get volumeattachment -o json |
  jq -c --arg pv "${RESOURCE_NAME}" --arg driver "${CSI_DRIVER}" \
    '[.items[] | select(
      .spec.source.persistentVolumeName == $pv and
      .spec.attacher == $driver and
      .status.attached == true
    ) | .spec.nodeName]')"
ATTACHMENT_COUNT="$(jq 'length' <<<"${ATTACHMENTS}")"
[[ "${ATTACHMENT_COUNT}" -eq 1 ]] || \
  die "expected exactly one attached node, found ${ATTACHMENT_COUNT}; the PVC must be mounted"
ATTACHED_NODE="$(jq -r '.[0]' <<<"${ATTACHMENTS}")"

VOLUME_BEFORE="$(get_volume_summary)"
[[ "$(jq 'length' <<<"${VOLUME_BEFORE}")" -gt 0 ]] || \
  die "LINSTOR returned no diskful replicas for ${RESOURCE_NAME}"

if ! jq -e 'all(.[]; .provider == "LVM_THIN")' <<<"${VOLUME_BEFORE}" >/dev/null; then
  die "all LINSTOR replicas must use the LVM_THIN provider"
fi

if ! jq -e 'all(.[]; .state == "UpToDate" and all(.replication[]; . == "Established"))' \
  <<<"${VOLUME_BEFORE}" >/dev/null; then
  die "one or more LINSTOR replicas are not UpToDate/Established"
fi

PRIMARY_COUNT="$(jq '[.[] | select(.in_use)] | length' <<<"${VOLUME_BEFORE}")"
[[ "${PRIMARY_COUNT}" -eq 1 ]] || \
  die "expected exactly one in-use LINSTOR replica, found ${PRIMARY_COUNT}"
PRIMARY_NODE="$(jq -r '.[] | select(.in_use) | .node' <<<"${VOLUME_BEFORE}")"
[[ "${PRIMARY_NODE}" == "${ATTACHED_NODE}" ]] || \
  die "LINSTOR primary (${PRIMARY_NODE}) does not match VolumeAttachment (${ATTACHED_NODE})"

MINOR_COUNT="$(jq '[.[].minor] | unique | length' <<<"${VOLUME_BEFORE}")"
[[ "${MINOR_COUNT}" -eq 1 ]] || die "LINSTOR replicas report inconsistent DRBD minors"
DRBD_MINOR="$(jq -r '.[0].minor' <<<"${VOLUME_BEFORE}")"

RESOURCE_NODES=()
while IFS= read -r node; do
  RESOURCE_NODES+=("${node}")
done < <(jq -r '.[].node' <<<"${VOLUME_BEFORE}")

RESOURCE_POOLS=()
while IFS= read -r pool; do
  RESOURCE_POOLS+=("${pool}")
done < <(jq -r '[.[].pool] | unique[]' <<<"${VOLUME_BEFORE}")

POOL_BEFORE="$(get_pool_summary)"

SATELLITE="$(kubectl -n "${PIRAEUS_NAMESPACE}" get pods \
  -l app.kubernetes.io/component=linstor-satellite -o json |
  jq -r --arg node "${PRIMARY_NODE}" '
    [.items[] | select(.spec.nodeName == $node and .status.phase == "Running") | .metadata.name]
    | if length == 1 then .[0] else empty end')"
[[ -n "${SATELLITE}" ]] || \
  die "could not identify one running LINSTOR satellite on ${PRIMARY_NODE}"

DISCARD_CONFIG="$(kubectl -n "${PIRAEUS_NAMESPACE}" exec "${SATELLITE}" \
  -c linstor-satellite -- sh -c \
  'drbdsetup show "$1" --show-defaults | grep -E "discard-zeroes-if-aligned|^[[:space:]]+discard-granularity"' \
  _ "${RESOURCE_NAME}")"
DISCARD_ZEROES="$(awk '$1 == "discard-zeroes-if-aligned" {gsub(";", "", $2); print $2; exit}' \
  <<<"${DISCARD_CONFIG}")"
[[ "${DISCARD_ZEROES}" == yes ]] || \
  die "DRBD discard-zeroes-if-aligned is not enabled (value: ${DISCARD_ZEROES:-unknown})"

DISCARD_GRANULARITY="$(kubectl -n "${PIRAEUS_NAMESPACE}" exec "${SATELLITE}" \
  -c linstor-satellite -- cat "/sys/block/drbd${DRBD_MINOR}/queue/discard_granularity")"
DISCARD_MAX_BYTES="$(kubectl -n "${PIRAEUS_NAMESPACE}" exec "${SATELLITE}" \
  -c linstor-satellite -- cat "/sys/block/drbd${DRBD_MINOR}/queue/discard_max_bytes")"
[[ "${DISCARD_GRANULARITY}" =~ ^[0-9]+$ && "${DISCARD_GRANULARITY}" -gt 0 ]] || \
  die "DRBD does not advertise a usable discard granularity"
[[ "${DISCARD_MAX_BYTES}" =~ ^[0-9]+$ && "${DISCARD_MAX_BYTES}" -gt 0 ]] || \
  die "DRBD does not advertise discard support"

CSI_NODE_POD="$(kubectl -n "${PIRAEUS_NAMESPACE}" get pods \
  -l app.kubernetes.io/component=linstor-csi-node -o json |
  jq -r --arg node "${PRIMARY_NODE}" '
    [.items[] | select(.spec.nodeName == $node and .status.phase == "Running") | .metadata.name]
    | if length == 1 then .[0] else empty end')"
[[ -n "${CSI_NODE_POD}" ]] || \
  die "could not identify one running LINSTOR CSI node pod on ${PRIMARY_NODE}"

CSI_NODE_JSON="$(kubectl -n "${PIRAEUS_NAMESPACE}" get pod "${CSI_NODE_POD}" -o json)"
if ! jq -e '
  .spec.containers[]
  | select(.name == "linstor-csi")
  | .securityContext.privileged == true
    and (.securityContext.capabilities.add | index("SYS_ADMIN") != null)
    and any(.volumeMounts[];
      .mountPath == "/var/lib/kubelet" and
      (.mountPropagation == "Bidirectional" or .mountPropagation == "HostToContainer")
    )' <<<"${CSI_NODE_JSON}" >/dev/null; then
  die "LINSTOR CSI node container lacks the required privilege or kubelet mount propagation"
fi

MOUNT_LINES="$(kubectl -n "${PIRAEUS_NAMESPACE}" exec "${CSI_NODE_POD}" \
  -c linstor-csi -- findmnt -rn -S "/dev/drbd${DRBD_MINOR}" \
  -o TARGET,SOURCE,FSTYPE,SIZE,USED,AVAIL)"
MOUNT_INFO="$(awk -v resource="${RESOURCE_NAME}" '
  index($1, "/" resource "/mount") > 0 {print; exit}
' <<<"${MOUNT_LINES}")"
[[ -n "${MOUNT_INFO}" ]] || \
  die "CSI node pod cannot find the kubelet mount for ${RESOURCE_NAME}"

MOUNT_TARGET="$(awk '{print $1}' <<<"${MOUNT_INFO}")"
MOUNT_FILESYSTEM="$(awk '{print $3}' <<<"${MOUNT_INFO}")"
[[ "${MOUNT_FILESYSTEM}" == "${FILESYSTEM}" ]] || \
  die "mounted filesystem is ${MOUNT_FILESYSTEM}, expected ${FILESYSTEM}"

kubectl -n "${PIRAEUS_NAMESPACE}" exec "${CSI_NODE_POD}" \
  -c linstor-csi -- /usr/sbin/fstrim --version >/dev/null

info "Target"
printf 'PVC:                 %s/%s\n' "${PVC_NAMESPACE}" "${PVC_NAME}"
printf 'PV/LINSTOR resource: %s\n' "${RESOURCE_NAME}"
printf 'Filesystem:          %s\n' "${FILESYSTEM}"
printf 'Primary node:        %s\n' "${PRIMARY_NODE}"
printf 'DRBD device:         /dev/drbd%s\n' "${DRBD_MINOR}"
printf 'Discard granularity: %s bytes\n' "${DISCARD_GRANULARITY}"
printf 'CSI node pod:        %s/%s\n' "${PIRAEUS_NAMESPACE}" "${CSI_NODE_POD}"
printf 'Kubelet mount:       %s\n' "${MOUNT_TARGET}"

info "LINSTOR replicas before trim"
print_volume_summary "${VOLUME_BEFORE}"

info "LINSTOR pools before trim"
print_pool_summary "${POOL_BEFORE}"

if [[ "${ASSUME_YES}" != true ]]; then
  [[ -t 0 ]] || die "confirmation requires a terminal; use --yes for non-interactive execution"
  printf '\nTrim unused blocks from %s/%s? [y/N] ' "${PVC_NAMESPACE}" "${PVC_NAME}"
  read -r answer
  [[ "${answer}" == y || "${answer}" == Y ]] || die "cancelled"
fi

info "Mounted filesystem"
printf '%s\n' "${MOUNT_INFO}"

info "Syncing filesystem and issuing FITRIM"
kubectl -n "${PIRAEUS_NAMESPACE}" exec "${CSI_NODE_POD}" \
  -c linstor-csi -- /bin/sync
TRIM_OUTPUT="$(kubectl -n "${PIRAEUS_NAMESPACE}" exec "${CSI_NODE_POD}" \
  -c linstor-csi -- /usr/sbin/fstrim --verbose \
  --minimum "${DISCARD_GRANULARITY}" "${MOUNT_TARGET}")"
printf '%s\n' "${TRIM_OUTPUT}"

VOLUME_AFTER="$(get_volume_summary)"
POOL_AFTER="$(get_pool_summary)"

info "LINSTOR replicas after trim"
print_volume_summary "${VOLUME_AFTER}"

info "LINSTOR pools after trim"
print_pool_summary "${POOL_AFTER}"

ALLOCATED_BEFORE_KIB="$(jq '[.[].allocated_kib] | add' <<<"${VOLUME_BEFORE}")"
ALLOCATED_AFTER_KIB="$(jq '[.[].allocated_kib] | add' <<<"${VOLUME_AFTER}")"
RECLAIMED_KIB=$((ALLOCATED_BEFORE_KIB - ALLOCATED_AFTER_KIB))

POOL_GAIN_KIB="$(jq -n \
  --argjson before "${POOL_BEFORE}" \
  --argjson after "${POOL_AFTER}" '
  [
    $after[] as $a
    | $before[]
    | select(.node == $a.node and .pool == $a.pool)
    | ($a.free_kib - .free_kib)
  ] | add // 0')"

info "Result"
printf 'Replica allocation before: %s\n' "$(format_kib "${ALLOCATED_BEFORE_KIB}")"
printf 'Replica allocation after:  %s\n' "$(format_kib "${ALLOCATED_AFTER_KIB}")"
printf 'Replica allocation freed:  %s\n' "$(format_kib "${RECLAIMED_KIB}")"
printf 'Observed pool free gain:    %s\n' "$(format_kib "${POOL_GAIN_KIB}")"

if [[ "${RECLAIMED_KIB}" -le 0 ]]; then
  printf 'Note: LINSTOR allocation did not decrease; the volume may already be fully trimmed.\n' >&2
fi
