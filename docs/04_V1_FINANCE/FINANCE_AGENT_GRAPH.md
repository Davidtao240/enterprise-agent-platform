# Finance Agent Graph

## Purpose

`finance_operating_report_graph` is the V1 Agent Graph for operating data reporting. It parses uploaded finance data, maps fields, validates metrics, generates analysis, creates a report draft, and prepares a review summary for human approval.

## Graph Identity

```text
business_app_code: finance
workflow_template_key: finance_operating_report
graph_key: finance_operating_report_graph
version: 1.0.0
```

## Graph Input

```json
{
  "trace_id": "trace_001",
  "business_app_code": "finance",
  "workflow_template_key": "finance_operating_report",
  "graph_key": "finance_operating_report_graph",
  "workflow_instance_id": "wf_001",
  "node_instance_id": "node_002",
  "input": {
    "file_id": "file_001",
    "month": "2026-05",
    "department": "Finance Center"
  },
  "context": {
    "user_id": "user_001",
    "department_id": "finance",
    "tenant_id": "default"
  }
}
```

## Graph Output

```json
{
  "run_id": "run_001",
  "graph_key": "finance_operating_report_graph",
  "status": "succeeded",
  "output": {
    "summary": "Revenue increased by 8.2%, while cost increased by 10.5%.",
    "key_metrics": {
      "revenue": 1200000,
      "cost": 760000,
      "gross_profit": 440000,
      "net_profit": 310000,
      "gross_margin": 0.3667,
      "net_margin": 0.2583
    },
    "warnings": [
      {
        "level": "medium",
        "message": "Cost growth is higher than revenue growth."
      }
    ],
    "report": {
      "title": "2026-05 Finance Operating Report",
      "sections": []
    },
    "result_file_id": "file_099",
    "review_suggestions": [
      "Confirm whether marketing expense caused the cost increase."
    ]
  },
  "usage": {
    "model": "qwen-plus",
    "prompt_tokens": 1200,
    "completion_tokens": 600,
    "cost": 0.03
  },
  "error": null
}
```

## Agent Flow

```text
DataExtractAgent
-> SchemaMappingAgent
-> ValidationAgent
-> FinanceAnalysisAgent
-> ReportAgent
-> ReviewSummaryAgent
```

## DataExtractAgent

Responsibility:

- Load uploaded CSV or Excel from file service.
- Extract rows and columns.
- Produce normalized tabular data.

Input:

```json
{
  "file_id": "file_001"
}
```

Output:

```json
{
  "columns": ["month", "department", "revenue", "cost", "net_profit"],
  "rows": [
    {
      "month": "2026-05",
      "department": "Finance Center",
      "revenue": 1200000,
      "cost": 760000,
      "net_profit": 310000
    }
  ]
}
```

Failure cases:

- File not found.
- Unsupported file type.
- Empty file.
- Parse error.

## SchemaMappingAgent

Responsibility:

- Map uploaded column names to canonical finance schema.
- Handle simple aliases such as `收入` to `revenue`.
- Keep mapping confidence and warnings.

Canonical fields:

```text
month
department
revenue
cost
gross_profit
net_profit
customer_count
order_count
```

Output:

```json
{
  "mapped_rows": [],
  "field_mapping": {
    "收入": "revenue",
    "成本": "cost"
  },
  "warnings": []
}
```

## ValidationAgent

Responsibility:

- Validate required fields.
- Validate numeric fields.
- Detect missing values and obvious outliers.
- Check derived metrics when possible.

Required fields:

```text
month
department
revenue
cost
net_profit
```

Output:

```json
{
  "valid": true,
  "errors": [],
  "warnings": [
    {
      "field": "cost",
      "message": "Cost growth appears high.",
      "level": "medium"
    }
  ]
}
```

Validation failure:

```json
{
  "valid": false,
  "errors": [
    {
      "code": "MISSING_REQUIRED_FIELD",
      "field": "revenue",
      "message": "Missing revenue."
    }
  ]
}
```

## FinanceAnalysisAgent

Responsibility:

- Calculate key metrics.
- Generate revenue, cost, profit, and margin analysis.
- Identify risks and anomalies.
- Produce concise business explanation.

Output:

```json
{
  "key_metrics": {
    "revenue": 1200000,
    "cost": 760000,
    "gross_profit": 440000,
    "net_profit": 310000,
    "gross_margin": 0.3667,
    "net_margin": 0.2583
  },
  "analysis": {
    "revenue_summary": "Revenue increased compared with the previous period.",
    "cost_summary": "Cost growth is higher than revenue growth.",
    "profit_summary": "Net profit remains positive.",
    "risk_summary": "Margin pressure should be reviewed."
  },
  "warnings": []
}
```

## ReportAgent

Responsibility:

- Generate structured report JSON.
- Generate preview content.
- Optionally generate report file and return `result_file_id`.

Output:

```json
{
  "report": {
    "title": "2026-05 Finance Operating Report",
    "period": "2026-05",
    "department": "Finance Center",
    "sections": [
      {
        "title": "Executive Summary",
        "content": "Revenue increased by 8.2%..."
      }
    ]
  },
  "result_file_id": "file_099"
}
```

## ReviewSummaryAgent

Responsibility:

- Prepare reviewer-facing summary.
- Highlight warnings.
- Suggest review questions.

Output:

```json
{
  "summary": "Revenue increased by 8.2%, cost increased by 10.5%.",
  "warnings": [
    "Cost growth is higher than revenue growth."
  ],
  "review_suggestions": [
    "Confirm whether cost growth is caused by one-time expense."
  ]
}
```

## Retry Strategy

| Step | Retryable | Notes |
|---|---:|---|
| DataExtractAgent | yes | Retry if file service timeout |
| SchemaMappingAgent | yes | Retry if LLM timeout |
| ValidationAgent | no | Data errors should return validation failure |
| FinanceAnalysisAgent | yes | Retry if LLM timeout |
| ReportAgent | yes | Retry if report generation fails |
| ReviewSummaryAgent | yes | Retry if LLM timeout |

## Human Review Conditions

V1 always requires finance manager review before archive.

Additional warnings should be highlighted when:

- Missing optional fields reduce analysis confidence.
- Cost growth is higher than revenue growth.
- Net profit is negative.
- Margin is lower than configured threshold.
- Outliers are detected.

## Error Codes

| Code | Meaning |
|---|---|
| FILE_NOT_FOUND | Uploaded file does not exist |
| UNSUPPORTED_FILE_TYPE | File type is not supported |
| FILE_PARSE_FAILED | File parse failed |
| SCHEMA_MAPPING_FAILED | Column mapping failed |
| SCHEMA_VALIDATION_FAILED | Required validation failed |
| FINANCE_ANALYSIS_FAILED | Analysis failed |
| REPORT_GENERATION_FAILED | Report generation failed |
| GRAPH_EXECUTION_FAILED | Unknown graph failure |
