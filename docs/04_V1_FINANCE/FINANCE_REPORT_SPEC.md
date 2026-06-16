# Finance Report Spec

## Purpose

This document defines the V1 finance report structure produced by `finance_operating_report_graph` and reviewed by a finance manager.

The report is generated as structured JSON first. HTML preview is rendered by the frontend. PDF or DOCX export can be added later.

## Report Identity

```text
report_type: finance_operating_report
schema_version: 1.0.0
business_app_code: finance
```

## Report JSON Structure

```json
{
  "report_type": "finance_operating_report",
  "schema_version": "1.0.0",
  "title": "2026-05 Finance Operating Report",
  "period": "2026-05",
  "department": "Finance Center",
  "generated_at": "2026-05-26T10:00:00Z",
  "summary": "Revenue increased by 8.2%, while cost increased by 10.5%.",
  "key_metrics": {
    "revenue": 1200000,
    "cost": 760000,
    "gross_profit": 440000,
    "net_profit": 310000,
    "gross_margin": 0.3667,
    "net_margin": 0.2583,
    "customer_count": 860,
    "order_count": 1430
  },
  "sections": [
    {
      "key": "revenue_analysis",
      "title": "Revenue Analysis",
      "content": "Revenue increased compared with the previous period."
    },
    {
      "key": "cost_analysis",
      "title": "Cost Analysis",
      "content": "Cost growth is higher than revenue growth."
    },
    {
      "key": "profit_analysis",
      "title": "Profit Analysis",
      "content": "Net profit remains positive but margin pressure should be reviewed."
    }
  ],
  "warnings": [
    {
      "level": "medium",
      "message": "Cost growth is higher than revenue growth.",
      "evidence": {
        "revenue_growth_rate": 0.082,
        "cost_growth_rate": 0.105
      }
    }
  ],
  "recommendations": [
    "Review marketing and operating expenses for one-time cost increases.",
    "Check whether revenue growth is sustainable in the next reporting period."
  ],
  "review": {
    "status": "pending",
    "reviewer": null,
    "comment": null,
    "reviewed_at": null
  }
}
```

## Required Fields

```text
report_type
schema_version
title
period
department
summary
key_metrics
sections
warnings
recommendations
review
```

## Key Metrics

| Field | Type | Required | Notes |
|---|---|---:|---|
| revenue | number | yes | Operating revenue |
| cost | number | yes | Operating cost |
| gross_profit | number | no | Revenue minus cost when available |
| net_profit | number | yes | Net profit |
| gross_margin | number | no | gross_profit / revenue |
| net_margin | number | no | net_profit / revenue |
| customer_count | number | no | Customer count |
| order_count | number | no | Order count |

## Warning Levels

| Level | Meaning |
|---|---|
| low | Informational warning |
| medium | Requires reviewer attention |
| high | Strong risk signal and must be reviewed |

## Frontend Preview

The finance report preview page should display:

- Report title.
- Period and department.
- Executive summary.
- Key metrics.
- Revenue analysis.
- Cost analysis.
- Profit analysis.
- Warnings.
- Recommendations.
- Approval decision panel.

## Approval Result

After approval:

```json
{
  "review": {
    "status": "approved",
    "reviewer": "finance_manager",
    "comment": "Approved.",
    "reviewed_at": "2026-05-26T10:30:00Z"
  }
}
```

After rejection:

```json
{
  "review": {
    "status": "rejected",
    "reviewer": "finance_manager",
    "comment": "Revenue source data needs correction.",
    "reviewed_at": "2026-05-26T10:30:00Z"
  }
}
```

## Archive Rules

- Only approved reports can be archived.
- Archived report JSON must be stored with the workflow output.
- Generated report file metadata must be stored in `files`.
- Approval decision must remain visible in report detail.

## Future Export Formats

V1 requires JSON and frontend HTML preview.

Optional future formats:

```text
PDF
DOCX
HTML file
```
