param(
  [string]$BaseUrl = "http://localhost:8080/api/v1",
  [string]$Username = "finance_user",
  [string]$Password = "password",
  [string]$ReviewerUsername = "finance_manager",
  [string]$ReviewerPassword = "password",
  [string]$OpsUsername = "ops_viewer",
  [string]$OpsPassword = "password",
  [string]$CsvPath = "docs/04_V1_FINANCE/sample_operating_data.csv"
)

$ErrorActionPreference = "Stop"

if (!(Test-Path $CsvPath)) {
  $tmp = New-TemporaryFile
  @"
month,department,revenue,cost,gross_profit,net_profit,customer_count,order_count
2026-05,Finance Center,1200000,760000,440000,310000,860,1430
2026-05,East Region,680000,420000,260000,180000,420,760
"@ | Set-Content -Encoding UTF8 $tmp
  $CsvPath = $tmp
}

function Login($user, $pass) {
  $body = @{ username = $user; password = $pass } | ConvertTo-Json
  $resp = Invoke-RestMethod -Method Post -Uri "$BaseUrl/auth/login" -ContentType "application/json" -Body $body
  return $resp.data.access_token
}

$token = Login $Username $Password
$headers = @{ Authorization = "Bearer $token" }
$reviewerToken = Login $ReviewerUsername $ReviewerPassword
$reviewHeaders = @{ Authorization = "Bearer $reviewerToken" }

Write-Host "1. Upload CSV"
$upload = Invoke-RestMethod -Method Post -Uri "$BaseUrl/files" -Headers $headers -Form @{
  business_app_code = "finance"
  file_role = "source"
  file = Get-Item $CsvPath
}
$fileId = $upload.data.file_id
Write-Host "file_id=$fileId"

Write-Host "2. Create workflow instance"
$createBody = @{
  business_app_code = "finance"
  workflow_template_key = "finance_operating_report"
  title = "Demo Finance Operating Report"
  input = @{ month = "2026-05"; department = "Finance Center"; file_id = $fileId }
} | ConvertTo-Json -Depth 8
$workflow = Invoke-RestMethod -Method Post -Uri "$BaseUrl/workflow-instances" -Headers $headers -ContentType "application/json" -Body $createBody
$workflowId = $workflow.data.id
Write-Host "workflow_id=$workflowId"

Write-Host "3. Start workflow"
Invoke-RestMethod -Method Post -Uri "$BaseUrl/workflow-instances/$workflowId/start" -Headers $headers | Out-Null

Write-Host "4. Wait for human_review approval task"
$approval = $null
for ($i = 0; $i -lt 30; $i++) {
  Start-Sleep -Seconds 2
  $approvals = Invoke-RestMethod -Method Get -Uri "$BaseUrl/approval-tasks?workflow_instance_id=$workflowId&status=pending" -Headers $reviewHeaders
  if ($approvals.data.Count -gt 0) {
    $approval = $approvals.data[0]
    break
  }
}
if ($null -eq $approval) { throw "Approval task was not created in time." }
Write-Host "approval_id=$($approval.id)"

Write-Host "5. Approve as reviewer"
$approveBody = @{ comment = "Approved by demo script." } | ConvertTo-Json
Invoke-RestMethod -Method Post -Uri "$BaseUrl/approval-tasks/$($approval.id)/approve" -Headers $reviewHeaders -ContentType "application/json" -Body $approveBody | Out-Null

Write-Host "6. Wait for workflow archived"
for ($i = 0; $i -lt 20; $i++) {
  Start-Sleep -Seconds 1
  $current = Invoke-RestMethod -Method Get -Uri "$BaseUrl/workflow-instances/$workflowId" -Headers $headers
  if ($current.data.status -eq "archived") {
    Write-Host "workflow archived"
    break
  }
}

Write-Host "7. Audit logs"
$opsToken = Login $OpsUsername $OpsPassword
$opsHeaders = @{ Authorization = "Bearer $opsToken" }
Invoke-RestMethod -Method Get -Uri "$BaseUrl/audit-logs?business_app_code=finance&page_size=10" -Headers $opsHeaders
