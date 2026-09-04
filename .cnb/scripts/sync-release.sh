#!/usr/bin/env bash
# - Copyright (c) 2026 HaydenGuo
# - Project: file-share-manager
# - Gitee: https://gitee.com/ghl1024/file-share-manager
# - GitHub: https://github.com/ghl1024/file-share-manager
# - CNB: https://cnb.cool/ghl1024/file-share-manager
# - GitCode: https://gitcode.com/haydenguo/file-share-manager
# - Author: https://hayden.pub

set -Eeuo pipefail

readonly TAG="${CNB_BRANCH:?CNB_BRANCH is required}"
readonly GITEE_API="https://gitee.com/api/v5/repos/ghl1024/file-share-manager"
readonly GITCODE_API="https://api.gitcode.com/api/v5/repos/haydenguo/file-share-manager"
work_dir="$(mktemp -d /tmp/fileshare-release.XXXXXX)"
readonly WORK_DIR="${work_dir}"
readonly BODY_FILE="${WORK_DIR}/release-body.md"
readonly GITEE_RELEASE_FILE="${WORK_DIR}/gitee-release.json"
readonly GITEE_ASSETS_FILE="${WORK_DIR}/gitee-assets.json"
readonly GITCODE_RELEASE_FILE="${WORK_DIR}/gitcode-release.json"

cleanup() {
  rm -rf "${WORK_DIR}"
}
trap cleanup EXIT

require_command() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "Missing required command: $1" >&2
    exit 1
  }
}

require_http_success() {
  local status="$1"
  local action="$2"
  local body_file="$3"

  if [[ ! "${status}" =~ ^2[0-9][0-9]$ ]]; then
    echo "${action} failed with HTTP ${status}:" >&2
    sed -n '1,120p' "${body_file}" >&2
    exit 1
  fi
}

for command_name in curl jq; do
  require_command "${command_name}"
done

for token_name in GITEE_TOKEN GITCODE_TOKEN; do
  if [[ -z "${!token_name:-}" ]]; then
    echo "${token_name} is not available from the CNB secret import." >&2
    exit 1
  fi
done

echo "Starting File Share Manager Release synchronization for ${TAG}."

shopt -s nullglob
assets=(dist/*.tar.gz)
if [[ -f dist/checksums.txt ]]; then
  assets+=(dist/checksums.txt)
fi
if (( ${#assets[@]} == 0 )); then
  echo 'No release archives found under dist/.' >&2
  exit 1
fi

release_name="${CNB_TAG_RELEASE_TITLE:-File Share Manager ${TAG}}"
release_body="${CNB_TAG_RELEASE_DESC:-}"
if [[ -z "${release_body}" ]]; then
  release_body="${CNB_COMMIT_MESSAGE:-File Share Manager ${TAG}}"
fi
printf '%s\n' "${release_body}" >"${BODY_FILE}"

curl_args=(
  curl
  -sS
  --retry 3
  --retry-delay 5
  --retry-connrefused
  --connect-timeout 15
  --max-time 180
)

normalize_gitcode_url() {
  local url="$1"

  if [[ "${url}" =~ ^https?:// ]]; then
    printf '%s\n' "${url}"
  elif [[ "${url}" == /* ]]; then
    printf 'https://api.gitcode.com%s\n' "${url}"
  else
    printf '%s\n' "${url}"
  fi
}

extract_gitcode_upload_url() {
  local body_file="$1"
  local upload_url

  if ! upload_url="$(jq -er '
    if type == "string" then
      .
    elif type == "object" then
      .url // .upload_url // .uploadUrl // .href //
      .data.url // .data.upload_url // .data.uploadUrl // .data.href //
      (if (.data | type) == "string" then .data else empty end)
    else
      empty
    end
  ' "${body_file}")"; then
    echo 'GitCode upload URL response does not contain a usable URL:' >&2
    sed -n '1,120p' "${body_file}" >&2
    exit 1
  fi

  normalize_gitcode_url "${upload_url}"
}

load_gitcode_upload_headers() {
  local body_file="$1"
  local header

  upload_headers=()
  while IFS= read -r header; do
    [[ -n "${header}" ]] || continue
    upload_headers+=(-H "${header}")
  done < <(jq -r '
    (.headers // .data.headers // {})
    | if type == "object" then to_entries[] else empty end
    | select(.value != null)
    | "\(.key): \(.value | tostring)"
  ' "${body_file}")

  if ! jq -e '
    (.headers // .data.headers // {}) as $headers
    | ($headers | type) == "object"
      and ($headers | has("Content-Type") or has("content-type"))
  ' "${body_file}" >/dev/null; then
    upload_headers+=(-H 'Content-Type: application/octet-stream')
  fi
}

upload_gitcode_asset() {
  local file="$1"
  local filename="$2"
  local upload_url
  local upload_url_status
  local upload_status
  local -a upload_headers

  upload_url_status="$("${curl_args[@]}" \
    -o "${WORK_DIR}/gitcode-upload-url.json" \
    -w '%{http_code}' \
    -H "PRIVATE-TOKEN: ${GITCODE_TOKEN}" \
    --get "${GITCODE_API}/releases/${TAG}/upload_url" \
    --data-urlencode "access_token=${GITCODE_TOKEN}" \
    --data-urlencode "file_name=${filename}")"
  require_http_success "${upload_url_status}" \
    "GitCode upload URL lookup (${filename})" \
    "${WORK_DIR}/gitcode-upload-url.json"

  upload_url="$(extract_gitcode_upload_url "${WORK_DIR}/gitcode-upload-url.json")"
  load_gitcode_upload_headers "${WORK_DIR}/gitcode-upload-url.json"

  upload_status="$("${curl_args[@]}" \
    -o "${WORK_DIR}/gitcode-upload-${filename}.json" \
    -w '%{http_code}' \
    -X PUT "${upload_url}" \
    "${upload_headers[@]}" \
    --upload-file "${file}")"
  require_http_success "${upload_status}" \
    "GitCode asset upload (${filename})" \
    "${WORK_DIR}/gitcode-upload-${filename}.json"
}

gitee_release_id=''
gitee_status="$("${curl_args[@]}" \
  -o "${GITEE_RELEASE_FILE}" \
  -w '%{http_code}' \
  --get "${GITEE_API}/releases/tags/${TAG}" \
  --data-urlencode "access_token=${GITEE_TOKEN}")"

echo "Gitee release lookup returned HTTP ${gitee_status}."
if [[ "${gitee_status}" == 200 ]]; then
  if ! gitee_release_id="$(jq -er 'select(type == "object") | .id // empty' "${GITEE_RELEASE_FILE}")"; then
    echo "Gitee returned no Release object for ${TAG}; creating it."
    gitee_status=404
  fi
fi

if [[ -n "${gitee_release_id}" ]]; then
  echo "Gitee Release ${TAG} already exists; updating it."
  gitee_status="$("${curl_args[@]}" \
    -o "${GITEE_RELEASE_FILE}" \
    -w '%{http_code}' \
    -X PATCH "${GITEE_API}/releases/${gitee_release_id}" \
    --data-urlencode "access_token=${GITEE_TOKEN}" \
    --data-urlencode "tag_name=${TAG}" \
    --data-urlencode "name=${release_name}" \
    --data-urlencode "body@${BODY_FILE}" \
    --data-urlencode 'target_commitish=main')"
  require_http_success "${gitee_status}" 'Gitee release update' "${GITEE_RELEASE_FILE}"
elif [[ "${gitee_status}" == 404 ]]; then
  echo "Creating Gitee Release ${TAG}."
  gitee_status="$("${curl_args[@]}" \
    -o "${GITEE_RELEASE_FILE}" \
    -w '%{http_code}' \
    -X POST "${GITEE_API}/releases" \
    --data-urlencode "access_token=${GITEE_TOKEN}" \
    --data-urlencode "tag_name=${TAG}" \
    --data-urlencode "name=${release_name}" \
    --data-urlencode "body@${BODY_FILE}" \
    --data-urlencode 'target_commitish=main')"
  require_http_success "${gitee_status}" 'Gitee release create' "${GITEE_RELEASE_FILE}"
  if ! gitee_release_id="$(jq -er 'select(type == "object") | .id // empty' "${GITEE_RELEASE_FILE}")"; then
    echo 'Gitee release create response does not contain an id:' >&2
    sed -n '1,120p' "${GITEE_RELEASE_FILE}" >&2
    exit 1
  fi
else
  require_http_success "${gitee_status}" 'Gitee release lookup' "${GITEE_RELEASE_FILE}"
fi

gitee_status="$("${curl_args[@]}" \
  -o "${GITEE_ASSETS_FILE}" \
  -w '%{http_code}' \
  --get "${GITEE_API}/releases/${gitee_release_id}/attach_files" \
  --data-urlencode "access_token=${GITEE_TOKEN}")"
require_http_success "${gitee_status}" 'Gitee release asset lookup' "${GITEE_ASSETS_FILE}"
if ! jq -e 'type == "array"' "${GITEE_ASSETS_FILE}" >/dev/null; then
  echo 'Gitee release asset response is not an array:' >&2
  sed -n '1,120p' "${GITEE_ASSETS_FILE}" >&2
  exit 1
fi

for file in "${assets[@]}"; do
  filename="$(basename "${file}")"
  existing_asset="$(jq -c --arg name "${filename}" '.[] | select((.name // "") == $name)' "${GITEE_ASSETS_FILE}" | sed -n '1p' || true)"
  if [[ -n "${existing_asset}" ]]; then
    asset_id="$(jq -r '.id // empty' <<<"${existing_asset}")"
    if [[ -n "${asset_id}" ]]; then
      delete_status="$("${curl_args[@]}" \
        -o "${WORK_DIR}/gitee-delete-${asset_id}.json" \
        -w '%{http_code}' \
        -X DELETE "${GITEE_API}/releases/${gitee_release_id}/attach_files/${asset_id}" \
        --get \
        --data-urlencode "access_token=${GITEE_TOKEN}")"
      require_http_success "${delete_status}" "Gitee asset delete (${filename})" "${WORK_DIR}/gitee-delete-${asset_id}.json"
    fi
  fi

  echo "Uploading ${filename} to Gitee."
  upload_status="$("${curl_args[@]}" \
    -o "${WORK_DIR}/gitee-upload-${filename}.json" \
    -w '%{http_code}' \
    -X POST "${GITEE_API}/releases/${gitee_release_id}/attach_files" \
    -F "access_token=${GITEE_TOKEN}" \
    -F "file=@${file}")"
  require_http_success "${upload_status}" "Gitee asset upload (${filename})" "${WORK_DIR}/gitee-upload-${filename}.json"
done

gitcode_status="$("${curl_args[@]}" \
  -o "${GITCODE_RELEASE_FILE}" \
  -w '%{http_code}' \
  -H "PRIVATE-TOKEN: ${GITCODE_TOKEN}" \
  "${GITCODE_API}/releases/tags/${TAG}")"

echo "GitCode release lookup returned HTTP ${gitcode_status}."
if [[ "${gitcode_status}" == 200 ]] && ! jq -e --arg tag "${TAG}" '
    type == "object" and .tag_name == $tag
  ' "${GITCODE_RELEASE_FILE}" >/dev/null; then
  echo "GitCode returned no Release object for ${TAG}; creating it."
  gitcode_status=404
fi

if [[ "${gitcode_status}" == 200 ]]; then
  echo "GitCode Release ${TAG} already exists; reusing it."
elif [[ "${gitcode_status}" == 404 ]]; then
  echo "Creating GitCode Release ${TAG}."
  gitcode_payload="$(jq -n \
    --arg tag "${TAG}" \
    --arg name "${release_name}" \
    --arg body "${release_body}" \
    '{tag_name: $tag, name: $name, body: $body, target_commitish: "main"}')"
  gitcode_status="$("${curl_args[@]}" \
    -o "${GITCODE_RELEASE_FILE}" \
    -w '%{http_code}' \
    -X POST "${GITCODE_API}/releases" \
    -H "PRIVATE-TOKEN: ${GITCODE_TOKEN}" \
    -H 'Content-Type: application/json' \
    --data "${gitcode_payload}")"
  require_http_success "${gitcode_status}" 'GitCode release create' "${GITCODE_RELEASE_FILE}"
  if ! jq -e --arg tag "${TAG}" 'type == "object" and .tag_name == $tag' \
    "${GITCODE_RELEASE_FILE}" >/dev/null; then
    echo 'GitCode release create response does not contain the expected tag:' >&2
    sed -n '1,120p' "${GITCODE_RELEASE_FILE}" >&2
    exit 1
  fi
else
  require_http_success "${gitcode_status}" 'GitCode release lookup' "${GITCODE_RELEASE_FILE}"
fi

for file in "${assets[@]}"; do
  filename="$(basename "${file}")"
  if jq -e --arg name "${filename}" '
      [ .assets[]?, .attach_files[]?, .attachFiles[]?, .files[]? ]
      | map(.name // .filename // .file_name // empty)
      | index($name) != null
    ' "${GITCODE_RELEASE_FILE}" >/dev/null; then
    echo "GitCode already contains ${filename}; skipping upload."
    continue
  fi

  echo "Uploading ${filename} to GitCode."
  upload_gitcode_asset "${file}" "${filename}"
done

echo "Release ${TAG} synchronized to Gitee and GitCode."
