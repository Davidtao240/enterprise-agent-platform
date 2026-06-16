# Sample Finance Data

## Purpose

This document defines sample CSV data and validation rules for the V1 finance operating report workflow.

## Canonical CSV Fields

| Field | Type | Required | Notes |
|---|---|---:|---|
| month | string | yes | Format `YYYY-MM` |
| department | string | yes | Department name |
| revenue | number | yes | Operating revenue |
| cost | number | yes | Operating cost |
| gross_profit | number | no | Revenue minus cost |
| net_profit | number | yes | Net profit |
| customer_count | integer | no | Number of customers |
| order_count | integer | no | Number of orders |

## Valid Sample CSV

```csv
month,department,revenue,cost,gross_profit,net_profit,customer_count,order_count
2026-05,Finance Center,1200000,760000,440000,310000,860,1430
2026-05,East Region,680000,420000,260000,180000,420,760
2026-05,South Region,520000,340000,180000,130000,310,540
```

Expected validation result:

```json
{
  "valid": true,
  "errors": [],
  "warnings": []
}
```

## Missing Required Field Sample

```csv
month,department,cost,net_profit,customer_count,order_count
2026-05,Finance Center,760000,310000,860,1430
```

Expected validation result:

```json
{
  "valid": false,
  "errors": [
    {
      "code": "MISSING_REQUIRED_FIELD",
      "field": "revenue",
      "message": "Missing required field revenue."
    }
  ],
  "warnings": []
}
```

## Invalid Numeric Sample

```csv
month,department,revenue,cost,gross_profit,net_profit,customer_count,order_count
2026-05,Finance Center,abc,760000,440000,310000,860,1430
```

Expected validation result:

```json
{
  "valid": false,
  "errors": [
    {
      "code": "INVALID_NUMBER",
      "field": "revenue",
      "message": "Revenue must be a number."
    }
  ],
  "warnings": []
}
```

## Outlier Sample

```csv
month,department,revenue,cost,gross_profit,net_profit,customer_count,order_count
2026-05,Finance Center,1200000,2000000,-800000,-950000,860,1430
```

Expected validation result:

```json
{
  "valid": true,
  "errors": [],
  "warnings": [
    {
      "code": "NEGATIVE_PROFIT",
      "field": "net_profit",
      "message": "Net profit is negative.",
      "level": "high"
    },
    {
      "code": "COST_EXCEEDS_REVENUE",
      "field": "cost",
      "message": "Cost is higher than revenue.",
      "level": "high"
    }
  ]
}
```

## Field Alias Examples

SchemaMappingAgent should support simple aliases:

| Alias | Canonical Field |
|---|---|
| 收入 | revenue |
| 营业收入 | revenue |
| 成本 | cost |
| 营业成本 | cost |
| 净利润 | net_profit |
| 客户数 | customer_count |
| 订单数 | order_count |

## Validation Rules

- `month` must match `YYYY-MM`.
- `department` cannot be empty.
- `revenue`, `cost`, and `net_profit` must be numeric.
- `revenue` should be greater than or equal to 0.
- `cost` should be greater than or equal to 0.
- `customer_count` and `order_count` must be integers when present.
- If `gross_profit` is present, it should approximately equal `revenue - cost`.
- Negative `net_profit` should produce a high-level warning.
- `cost > revenue` should produce a high-level warning.

## Expected Analysis Output

For the valid sample, the report should include:

```json
{
  "summary": "Finance Center revenue is 1,200,000 and net profit is 310,000.",
  "key_metrics": {
    "revenue": 1200000,
    "cost": 760000,
    "gross_profit": 440000,
    "net_profit": 310000,
    "gross_margin": 0.3667,
    "net_margin": 0.2583
  },
  "warnings": [],
  "recommendations": [
    "Review cost structure and compare against previous month after historical data is available."
  ]
}
```
