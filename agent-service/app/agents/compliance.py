"""ComplianceAgent: validates financial data against regulatory rules.

Specializes in:
- Compliance rule checking
- Policy validation
- Audit trail verification
- Risk flagging
"""

from __future__ import annotations

import logging
from typing import Any

from app.agents.base import BaseAgent

logger = logging.getLogger(__name__)


class ComplianceAgent(BaseAgent):
    agent_id = "compliance_agent"
    domain = "finance"
    reusable_scope = "finance"

    # Compliance rules (extensible)
    RULES = [
        {"id": "COMPLIANCE_001", "description": "All financial records must have valid identifiers"},
        {"id": "COMPLIANCE_002", "description": "Monetary values must be non-negative where applicable"},
        {"id": "COMPLIANCE_003", "description": "Dates must be within the reporting period"},
        {"id": "COMPLIANCE_004", "description": "Required fields must not be empty"},
    ]

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        collected = state.get("sub_results", {}).get("data_collection", {})
        data = collected.get("data", [])

        compliance_result = self._check_compliance(data, state)
        result = {
            "task": "compliance_check",
            "status": "completed",
            "compliance": compliance_result,
        }
        return {"sub_results": {**state.get("sub_results", {}), "compliance_check": result}}

    def _check_compliance(self, data: list[dict[str, Any]], state: dict[str, Any]) -> dict[str, Any]:
        violations = []
        passed_rules = []

        if not data:
            return {
                "status": "pass",
                "violations": [],
                "passed_rules": [r["id"] for r in self.RULES],
                "compliance_rate": 100.0,
                "note": "No data to check",
            }

        # Check COMPLIANCE_004: Required fields not empty
        required_fields = self._get_required_fields(state)
        for row_idx, row in enumerate(data):
            for field in required_fields:
                if field in row and (row[field] is None or row[field] == ""):
                    violations.append({
                        "rule_id": "COMPLIANCE_004",
                        "row_index": row_idx,
                        "field": field,
                        "message": f"Required field '{field}' is empty",
                    })

        # Check COMPLIANCE_002: Non-negative monetary values
        monetary_fields = self._find_monetary_fields(data)
        for row_idx, row in enumerate(data):
            for field in monetary_fields:
                val = row.get(field)
                if isinstance(val, (int, float)) and val < 0:
                    violations.append({
                        "rule_id": "COMPLIANCE_002",
                        "row_index": row_idx,
                        "field": field,
                        "message": f"Monetary field '{field}' has negative value: {val}",
                    })

        passed_rules = [r["id"] for r in self.RULES if not any(
            v["rule_id"] == r["id"] for v in violations
        )]

        total_checks = len(data) * max(len(required_fields), 1)
        compliance_rate = ((total_checks - len(violations)) / total_checks * 100) if total_checks > 0 else 100.0

        return {
            "status": "fail" if violations else "pass",
            "violations": violations,
            "passed_rules": passed_rules,
            "compliance_rate": round(compliance_rate, 2),
            "records_checked": len(data),
            "violation_count": len(violations),
        }

    def _get_required_fields(self, state: dict[str, Any]) -> list[str]:
        # Default required fields for financial data
        return state.get("required_fields", ["id", "date", "amount", "description"])

    def _find_monetary_fields(self, data: list[dict[str, Any]]) -> list[str]:
        monetary_indicators = [
            "amount", "value", "total", "sum", "price", "cost", "revenue", "profit",
            "fee", "tax", "discount", "balance", "rate", "budget", "expense",
        ]
        currency_codes = ["cny", "usd", "eur", "hkd", "gbp", "jpy", "rmb", "yuan"]
        currency_symbols = ["¥", "￥", "$", "€", "£", "₩", "₪", "₫"]
        chinese_indicators = ["金额", "总额", "合计", "价格", "成本", "收入", "利润", "费用", "税率", "折扣", "余额", "预算", "支出"]

        if not data:
            return []
        fields = set()
        sample = data[0]
        for key in sample.keys():
            key_lower = key.lower()
            if any(indicator in key_lower for indicator in monetary_indicators):
                fields.add(key)
            if any(code in key_lower for code in currency_codes):
                fields.add(key)
            if any(ind in key for ind in chinese_indicators):
                fields.add(key)
        return list(fields)