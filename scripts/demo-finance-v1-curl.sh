#!/usr/bin/env bash
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080/api/v1}"
CSV_PATH="${CSV_PATH:-/tmp/finance-v1-demo.csv}"
PYTHON_BIN="${PYTHON_BIN:-}"

if [ -z "$PYTHON_BIN" ]; then
  if command -v python3 >/dev/null 2>&1; then
    PYTHON_BIN=python3
  elif command -v python >/dev/null 2>&1; then
    PYTHON_BIN=python
  else
    echo "python3 or python is required to parse JSON responses" >&2
    exit 1
  fi
fi

json_get() {
  "$PYTHON_BIN" - "$1" "$2" <<'PY'
import json
import sys

data = json.loads(sys.argv[1])
for part in sys.argv[2].split("."):
    data = data[part]
print(data)
PY
}

api_request() {
  local response_file status
  response_file="$(mktemp)"
  status=$(curl -sS -w "%{http_code}" -o "$response_file" "$@")
  if [ "$status" -lt 200 ] || [ "$status" -ge 300 ]; then
    echo "API request failed with HTTP $status:" >&2
    cat "$response_file" >&2
    rm -f "$response_file"
    exit 1
  fi
  cat "$response_file"
  rm -f "$response_file"
}

if [ ! -f "$CSV_PATH" ]; then
  cat > "$CSV_PATH" <<'CSV'
month,department,revenue,cost,gross_profit,net_profit,customer_count,order_count
2026-05,Finance Center,1200000,760000,440000,310000,860,1430
2026-05,East Region,680000,420000,260000,180000,420,760
CSV
fi

echo "1. Login finance user"
LOGIN_JSON=$(api_request -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"finance_user","password":"password"}')
TOKEN=$(json_get "$LOGIN_JSON" "data.access_token")

echo "2. Upload CSV"
UPLOAD_JSON=$(api_request -X POST "$BASE_URL/files" \
  -H "Authorization: Bearer $TOKEN" \
  -F business_app_code=finance \
  -F file_role=source \
  -F "file=@$CSV_PATH")
FILE_ID=$(json_get "$UPLOAD_JSON" "data.file_id")
echo "file_id=$FILE_ID"

echo "3. Create workflow"
CREATE_JSON=$(api_request -X POST "$BASE_URL/workflow-instances" \
  -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"business_app_code\":\"finance\",\"workflow_template_key\":\"finance_operating_report\",\"title\":\"Demo Finance Operating Report\",\"input\":{\"month\":\"2026-05\",\"department\":\"Finance Center\",\"file_id\":\"$FILE_ID\"}}")
WORKFLOW_ID=$(json_get "$CREATE_JSON" "data.id")
echo "workflow_id=$WORKFLOW_ID"

echo "4. Start workflow"
api_request -X POST "$BASE_URL/workflow-instances/$WORKFLOW_ID/start" \
  -H "Authorization: Bearer $TOKEN" > /dev/null

echo "5. Login reviewer"
REVIEW_LOGIN_JSON=$(api_request -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"finance_manager","password":"password"}')
REVIEW_TOKEN=$(json_get "$REVIEW_LOGIN_JSON" "data.access_token")

echo "6. Wait for approval task"
APPROVAL_ID=""
for _ in $(seq 1 30); do
  sleep 2
  WORKFLOW_JSON=$(api_request "$BASE_URL/workflow-instances/$WORKFLOW_ID" \
    -H "Authorization: Bearer $TOKEN")
  STATUS=$(json_get "$WORKFLOW_JSON" "data.status")
  if [ "$STATUS" = "failed" ]; then
    echo "workflow failed before approval task was created:" >&2
    echo "$WORKFLOW_JSON" >&2
    exit 1
  fi
  APPROVALS_JSON=$(api_request "$BASE_URL/approval-tasks?workflow_instance_id=$WORKFLOW_ID&status=pending" \
    -H "Authorization: Bearer $REVIEW_TOKEN")
  APPROVAL_ID=$("$PYTHON_BIN" - "$APPROVALS_JSON" <<'PY'
import json
import sys

data = json.loads(sys.argv[1]).get("data", [])
print(data[0]["id"] if data else "")
PY
)
  [ -n "$APPROVAL_ID" ] && break
done
[ -n "$APPROVAL_ID" ] || { echo "approval task not created in time" >&2; exit 1; }
echo "approval_id=$APPROVAL_ID"

echo "7. Approve as reviewer"
api_request -X POST "$BASE_URL/approval-tasks/$APPROVAL_ID/approve" \
  -H "Authorization: Bearer $REVIEW_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"comment":"Approved by curl demo."}' > /dev/null

echo "8. Wait for archived workflow"
for _ in $(seq 1 20); do
  sleep 1
  WORKFLOW_JSON=$(api_request "$BASE_URL/workflow-instances/$WORKFLOW_ID" \
    -H "Authorization: Bearer $TOKEN")
  STATUS=$(json_get "$WORKFLOW_JSON" "data.status")
  [ "$STATUS" = "archived" ] && break
done
echo "workflow_status=$STATUS"
[ "$STATUS" = "archived" ] || exit 1

echo "9. Recent audit logs"
OPS_LOGIN_JSON=$(api_request -X POST "$BASE_URL/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"username":"ops_viewer","password":"password"}')
OPS_TOKEN=$(json_get "$OPS_LOGIN_JSON" "data.access_token")
api_request "$BASE_URL/audit-logs?business_app_code=finance&page_size=10" \
  -H "Authorization: Bearer $OPS_TOKEN"
