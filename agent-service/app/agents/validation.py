"""ValidationAgent: validate mapped finance data for correctness.

Pure rule-based — no LLM. Checks required fields, numeric types, month format,
gross profit reconciliation, and outlier detection.
"""

import logging
import re
from typing import Any

from app.agents.base import BaseAgent

logger = logging.getLogger(__name__)

REQUIRED_FIELDS = ["month", "department", "revenue", "cost", "net_profit"]
MONTH_PATTERN = re.compile(r"^\d{4}-\d{2}$")


class ValidationAgent(BaseAgent):
    agent_id = "validation_agent"
    domain = "shared"

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        mapped_data = state.get("mapped_data") or {}
        mapped_rows = mapped_data.get("mapped_rows", [])
        columns = list(mapped_rows[0].keys()) if mapped_rows else []

        errors: list[dict[str, Any]] = []
        warnings: list[dict[str, Any]] = []

        # 1. Check required fields
        missing = [f for f in REQUIRED_FIELDS if f not in columns]
        for field in missing:
            errors.append({
                "code": "MISSING_REQUIRED_FIELD",
                "field": field,
                "message": f"Required field '{field}' is missing.",
            })

        if not mapped_rows:
            errors.append({
                "code": "EMPTY_DATA",
                "field": None,
                "message": "No data rows to validate.",
            })
            state["validation_result"] = {"valid": False, "errors": errors, "warnings": warnings}
            return state

        # 2. Per-row validation
        for i, row in enumerate(mapped_rows):
            # Month format
            month = row.get("month")
            if month is not None and not MONTH_PATTERN.match(str(month)):
                errors.append({
                    "code": "INVALID_MONTH_FORMAT",
                    "field": "month",
                    "message": f"Row {i}: '{month}' does not match YYYY-MM format.",
                    "row": i,
                })

            # Department non-empty
            dept = row.get("department")
            if dept is None or str(dept).strip() == "":
                warnings.append({
                    "code": "EMPTY_DEPARTMENT",
                    "field": "department",
                    "message": f"Row {i}: department is empty.",
                    "level": "low",
                    "row": i,
                })

            # Numeric validation
            for field in ["revenue", "cost", "net_profit", "gross_profit"]:
                val = row.get(field)
                if val is None:
                    continue
                try:
                    num = float(val)
                    if field in ("revenue", "cost") and num < 0:
                        warnings.append({
                            "code": "NEGATIVE_VALUE",
                            "field": field,
                            "message": f"Row {i}: {field} is negative ({num}).",
                            "level": "medium",
                            "row": i,
                        })
                except (ValueError, TypeError):
                    if field in REQUIRED_FIELDS:
                        errors.append({
                            "code": "INVALID_NUMBER",
                            "field": field,
                            "message": f"Row {i}: {field}='{val}' is not a valid number.",
                            "row": i,
                        })
                    else:
                        warnings.append({
                            "code": "INVALID_NUMBER",
                            "field": field,
                            "message": f"Row {i}: {field}='{val}' is not a valid number.",
                            "level": "low",
                            "row": i,
                        })

            # Integer check for count fields
            for field in ["customer_count", "order_count"]:
                val = row.get(field)
                if val is not None:
                    try:
                        int_val = int(float(str(val)))
                        if int_val < 0:
                            warnings.append({
                                "code": "NEGATIVE_COUNT",
                                "field": field,
                                "message": f"Row {i}: {field}={int_val} is negative.",
                                "level": "low",
                                "row": i,
                            })
                    except (ValueError, TypeError):
                        pass

            # 3. Gross profit reconciliation (5% tolerance)
            revenue = row.get("revenue")
            cost = row.get("cost")
            gross_profit = row.get("gross_profit")
            if revenue is not None and cost is not None and gross_profit is not None:
                try:
                    r, c, gp = float(revenue), float(cost), float(gross_profit)
                    expected = r - c
                    denom = max(abs(r), 1.0)
                    if abs(gp - expected) / denom > 0.05:
                        warnings.append({
                            "code": "GROSS_PROFIT_MISMATCH",
                            "field": "gross_profit",
                            "message": f"Row {i}: gross_profit={gp} doesn't match revenue-cost={expected}.",
                            "level": "medium",
                            "row": i,
                        })
                except (ValueError, TypeError):
                    pass

            # 4. Outlier detection
            try:
                r, c = float(row.get("revenue", 0)), float(row.get("cost", 0))
                np_val = float(row.get("net_profit", 0))
                if np_val < 0:
                    warnings.append({
                        "code": "NEGATIVE_PROFIT",
                        "field": "net_profit",
                        "message": f"Row {i}: net_profit is negative ({np_val}).",
                        "level": "high",
                        "row": i,
                    })
                if c > r and r > 0:
                    warnings.append({
                        "code": "COST_EXCEEDS_REVENUE",
                        "field": "cost",
                        "message": f"Row {i}: cost ({c}) exceeds revenue ({r}).",
                        "level": "high",
                        "row": i,
                    })
            except (ValueError, TypeError):
                pass

        # Carry over data-loading warnings
        load_warnings = state.get("_load_warnings", [])
        warnings = load_warnings + warnings

        valid = len(errors) == 0
        state["validation_result"] = {
            "valid": valid,
            "errors": errors,
            "warnings": warnings,
        }

        logger.info("ValidationAgent: valid=%s, %d errors, %d warnings", valid, len(errors), len(warnings))
        return state
