"""Finance V1 profile for reusable mapping, validation, and reporting agents."""

from __future__ import annotations

import json
import re
from types import MappingProxyType
from typing import Any, Sequence

from app.profiles.contracts import (
    DataExtractionProfile,
    DomainAgentProfile,
    ReportProfile,
    ReviewSummaryProfile,
    SchemaMappingProfile,
    ValidationProfile,
)

PROFILE_KEY = "finance_operating_report_profile"
PROFILE_VERSION = "1.0.0"
BUSINESS_APP_CODE = "finance"

SAMPLE_COLUMNS = (
    "month",
    "department",
    "revenue",
    "cost",
    "gross_profit",
    "net_profit",
    "customer_count",
    "order_count",
)

SAMPLE_ROWS = (
    {
        "month": "2026-05",
        "department": "Finance Center",
        "revenue": 1200000,
        "cost": 760000,
        "gross_profit": 440000,
        "net_profit": 310000,
        "customer_count": 860,
        "order_count": 1430,
    },
    {
        "month": "2026-05",
        "department": "East Region",
        "revenue": 680000,
        "cost": 420000,
        "gross_profit": 260000,
        "net_profit": 180000,
        "customer_count": 420,
        "order_count": 760,
    },
    {
        "month": "2026-05",
        "department": "South Region",
        "revenue": 520000,
        "cost": 340000,
        "gross_profit": 180000,
        "net_profit": 130000,
        "customer_count": 310,
        "order_count": 540,
    },
)


def finance_sample_data() -> tuple[list[str], list[dict[str, Any]]]:
    """Return isolated copies of the canonical Finance V1 demo data."""
    return list(SAMPLE_COLUMNS), [dict(row) for row in SAMPLE_ROWS]


CANONICAL_FIELDS = (
    "month",
    "department",
    "revenue",
    "cost",
    "gross_profit",
    "net_profit",
    "customer_count",
    "order_count",
)

ALIASES = MappingProxyType({
    # Chinese -> canonical
    "收入": "revenue",
    "营业收入": "revenue",
    "成本": "cost",
    "营业成本": "cost",
    "净利润": "net_profit",
    "净利": "net_profit",
    "毛利": "gross_profit",
    "毛利润": "gross_profit",
    "客户数": "customer_count",
    "客户数量": "customer_count",
    "订单数": "order_count",
    "订单数量": "order_count",
    "月份": "month",
    "日期": "month",
    "时间": "month",
    "部门": "department",
    "事业部": "department",
    # English -> canonical
    "income": "revenue",
    "sales": "revenue",
    "turnover": "revenue",
    "expense": "cost",
    "expenses": "cost",
    "operating cost": "cost",
    "profit": "net_profit",
    "net income": "net_profit",
    "net": "net_profit",
    "gross income": "gross_profit",
    "gross": "gross_profit",
    "gross margin": "gross_profit",
    "dept": "department",
    "division": "department",
    "cust": "customer_count",
    "customers": "customer_count",
    "orders": "order_count",
    "order volume": "order_count",
})

REQUIRED_FIELDS = ("month", "department", "revenue", "cost", "net_profit")
MONTH_PATTERN = re.compile(r"^\d{4}-\d{2}$")


def build_mapping_prompt(unmapped: Sequence[str]) -> str:
    return f"""Map these column names to the canonical finance schema fields.

Canonical fields: {json.dumps(CANONICAL_FIELDS)}

Unmapped columns: {json.dumps(list(unmapped))}

Return ONLY a JSON object mapping each column to a canonical field (or null if no match).
Example: {{"sales_revenue": "revenue", "op_cost": "cost", "notes": null}}
"""


def validate_rows(rows: list[dict[str, Any]]) -> tuple[list[dict[str, Any]], list[dict[str, Any]]]:
    """Apply the Finance V1 deterministic validation rules."""
    columns = list(rows[0].keys()) if rows else []
    errors: list[dict[str, Any]] = []
    warnings: list[dict[str, Any]] = []

    missing = [field for field in REQUIRED_FIELDS if field not in columns]
    for field in missing:
        errors.append({
            "code": "MISSING_REQUIRED_FIELD",
            "field": field,
            "message": f"缺少必填字段“{field}”。",
        })

    if not rows:
        errors.append({
            "code": "EMPTY_DATA",
            "field": None,
            "message": "没有可供校验的数据行。",
        })
        return errors, warnings

    for index, row in enumerate(rows):
        month = row.get("month")
        if month is not None and not MONTH_PATTERN.match(str(month)):
            errors.append({
                "code": "INVALID_MONTH_FORMAT",
                "field": "month",
                "message": f"第 {index + 1} 行：期间“{month}”不符合 YYYY-MM 格式。",
                "row": index,
            })

        department = row.get("department")
        if department is None or str(department).strip() == "":
            warnings.append({
                "code": "EMPTY_DEPARTMENT",
                "field": "department",
                "message": f"第 {index + 1} 行：部门为空。",
                "level": "low",
                "row": index,
            })

        for field in ("revenue", "cost", "net_profit", "gross_profit"):
            value = row.get(field)
            if value is None:
                continue
            try:
                number = float(value)
                if field in ("revenue", "cost") and number < 0:
                    warnings.append({
                        "code": "NEGATIVE_VALUE",
                        "field": field,
                        "message": f"第 {index + 1} 行：字段 {field} 为负数（{number}）。",
                        "level": "medium",
                        "row": index,
                    })
            except (ValueError, TypeError):
                issue = {
                    "code": "INVALID_NUMBER",
                    "field": field,
                    "message": f"第 {index + 1} 行：字段 {field} 的值“{value}”不是有效数字。",
                    "row": index,
                }
                if field in REQUIRED_FIELDS:
                    errors.append(issue)
                else:
                    warnings.append({**issue, "level": "low"})

        for field in ("customer_count", "order_count"):
            value = row.get(field)
            if value is None:
                continue
            try:
                integer_value = int(float(str(value)))
                if integer_value < 0:
                    warnings.append({
                        "code": "NEGATIVE_COUNT",
                        "field": field,
                        "message": f"第 {index + 1} 行：字段 {field} 为负数（{integer_value}）。",
                        "level": "low",
                        "row": index,
                    })
            except (ValueError, TypeError):
                pass

        revenue = row.get("revenue")
        cost = row.get("cost")
        gross_profit = row.get("gross_profit")
        if revenue is not None and cost is not None and gross_profit is not None:
            try:
                revenue_number = float(revenue)
                cost_number = float(cost)
                gross_profit_number = float(gross_profit)
                expected = revenue_number - cost_number
                denominator = max(abs(revenue_number), 1.0)
                if abs(gross_profit_number - expected) / denominator > 0.05:
                    warnings.append({
                        "code": "GROSS_PROFIT_MISMATCH",
                        "field": "gross_profit",
                        "message": (
                            f"第 {index + 1} 行：毛利润 {gross_profit_number} "
                            f"与营业收入减成本的结果 {expected} 不一致。"
                        ),
                        "level": "medium",
                        "row": index,
                    })
            except (ValueError, TypeError):
                pass

        try:
            revenue_number = float(row.get("revenue", 0))
            cost_number = float(row.get("cost", 0))
            net_profit_number = float(row.get("net_profit", 0))
            if net_profit_number < 0:
                warnings.append({
                    "code": "NEGATIVE_PROFIT",
                    "field": "net_profit",
                    "message": f"第 {index + 1} 行：净利润为负数（{net_profit_number}）。",
                    "level": "high",
                    "row": index,
                })
            if cost_number > revenue_number and revenue_number > 0:
                warnings.append({
                    "code": "COST_EXCEEDS_REVENUE",
                    "field": "cost",
                    "message": (
                        f"第 {index + 1} 行：成本（{cost_number}）"
                        f"高于营业收入（{revenue_number}）。"
                    ),
                    "level": "high",
                    "row": index,
                })
        except (ValueError, TypeError):
            pass

    return errors, warnings


def build_fallback_report(
    mapped_data: dict[str, Any],
    metrics: dict[str, Any],
    warnings: list[dict[str, Any]],
) -> dict[str, Any]:
    rows = mapped_data.get("mapped_rows", [])
    periods = sorted({str(row.get("month", "")) for row in rows if row.get("month")})
    departments = sorted({
        str(row.get("department", ""))
        for row in rows
        if row.get("department")
    })

    period_text = "、".join(periods) if periods else "未提供"
    department_text = "、".join(departments) if departments else "未提供"

    return {
        "title": f"{period_text} 财务运营报告",
        "period": period_text,
        "department": department_text,
        "sections": [
            {
                "key": "executive_summary",
                "title": "管理层摘要",
                "content": (
                    f"本报告覆盖 {period_text} 的 {metrics.get('row_count', 0)} 个部门。"
                    f"营业收入合计为 {metrics.get('revenue', 0):,.0f}，"
                    f"净利润合计为 {metrics.get('net_profit', 0):,.0f}。"
                ),
            },
            {
                "key": "revenue_analysis",
                "title": "收入分析",
                "content": f"营业收入合计为 {metrics.get('revenue', 0):,.0f}。",
            },
            {
                "key": "cost_analysis",
                "title": "成本分析",
                "content": f"成本合计为 {metrics.get('cost', 0):,.0f}。",
            },
            {
                "key": "profit_analysis",
                "title": "利润分析",
                "content": (
                    f"毛利润为 {metrics.get('gross_profit', 0):,.0f}"
                    f"（毛利率 {metrics.get('gross_margin', 0):.1%}），"
                    f"净利润为 {metrics.get('net_profit', 0):,.0f}"
                    f"（净利率 {metrics.get('net_margin', 0):.1%}）。"
                ),
            },
        ],
        "warnings": warnings,
        "recommendations": ["复核成本变化趋势及利润率压力。"],
        "review": {
            "status": "pending",
            "reviewer": None,
            "comment": None,
            "reviewed_at": None,
        },
    }


def build_report_prompt(
    metrics: dict[str, Any],
    narrative: dict[str, Any],
    mapped_data: dict[str, Any],
    warnings: list[dict[str, Any]],
) -> str:
    rows = mapped_data.get("mapped_rows", [])
    periods = sorted({str(row.get("month", "")) for row in rows if row.get("month")})
    departments = sorted({
        str(row.get("department", ""))
        for row in rows
        if row.get("department")
    })
    return f"""请生成一份结构化财务运营报告，并以 JSON 返回。

期间：{'、'.join(periods) if periods else '未提供'}
部门：{'、'.join(departments) if departments else '未提供'}
关键指标：{json.dumps(metrics, ensure_ascii=False)}
分析结果：{json.dumps(narrative, ensure_ascii=False)}
风险提示：{json.dumps(warnings, default=str, ensure_ascii=False)}

返回的 JSON 必须包含：
- title：报告标题
- period：报告期间
- department：部门名称
- sections：{{key, title, content}} 对象数组，必须包含 executive_summary、revenue_analysis、cost_analysis、profit_analysis
- warnings：风险提示对象数组
- recommendations：2-4 条可执行建议
- review：{{status: "pending", reviewer: null, comment: null, reviewed_at: null}}

title、section.title、section.content、warnings.message 和 recommendations 等所有面向业务人员的字段必须使用专业中文。每个 content 使用 2-5 句话。只返回合法 JSON。
"""


def build_fallback_review_summary(
    key_metrics: dict[str, Any],
    analysis: dict[str, Any],
    warnings: list[dict[str, Any]],
) -> dict[str, Any]:
    del analysis
    revenue = key_metrics.get("revenue", 0)
    net_profit = key_metrics.get("net_profit", 0)
    net_margin = key_metrics.get("net_margin", 0)
    high_warnings = [warning for warning in warnings if warning.get("level") == "high"]
    medium_warnings = [warning for warning in warnings if warning.get("level") == "medium"]

    return {
        "summary": (
            f"营业收入合计为 {revenue:,.0f}，净利润合计为 {net_profit:,.0f}，"
            f"净利率为 {net_margin:.1%}。"
        ),
        "warnings": [
            warning.get("message", "")
            for warning in high_warnings + medium_warnings
        ],
        "review_suggestions": [
            "复核高优先级风险，并核对原始财务报表的数据准确性。",
            "确认报告涉及的审批和佐证材料是否完整。",
            "检查各部门是否存在异常趋势或显著偏差。",
        ],
    }


def build_review_prompt(
    key_metrics: dict[str, Any],
    analysis: dict[str, Any],
    warnings: list[dict[str, Any]],
) -> str:
    high_warnings = [
        warning
        for warning in warnings
        if warning.get("level") in ("high", "medium")
    ]
    return f"""你是一名财务复核专家。请为需要审批该报告的财务经理总结以下内容。

关键指标：{json.dumps(key_metrics, ensure_ascii=False)}
分析结果：{json.dumps(analysis, ensure_ascii=False)}
关键风险：{json.dumps(high_warnings, default=str, ensure_ascii=False)}

返回一个 JSON 对象，包含：
- summary：包含关键数字的 2-4 句中文管理层摘要
- warnings：需要关注的中文风险提示数组
- review_suggestions：2-4 条财务经理应核验的具体中文问题或事项

所有面向审批人的内容必须使用简洁、专业的中文。只返回合法 JSON。
"""


DATA_EXTRACTION_PROFILE = DataExtractionProfile(
    profile_key=PROFILE_KEY,
    profile_version=PROFILE_VERSION,
    business_app_code=BUSINESS_APP_CODE,
    fallback_data=finance_sample_data,
    fallback_warning="未读取到有效上传文件，当前使用内置财务示例数据。",
)

SCHEMA_MAPPING_PROFILE = SchemaMappingProfile(
    profile_key=PROFILE_KEY,
    profile_version=PROFILE_VERSION,
    business_app_code=BUSINESS_APP_CODE,
    canonical_fields=CANONICAL_FIELDS,
    aliases=ALIASES,
    build_prompt=build_mapping_prompt,
)

VALIDATION_PROFILE = ValidationProfile(
    profile_key=PROFILE_KEY,
    profile_version=PROFILE_VERSION,
    business_app_code=BUSINESS_APP_CODE,
    required_fields=REQUIRED_FIELDS,
    validate_rows=validate_rows,
)

REPORT_PROFILE = ReportProfile(
    profile_key=PROFILE_KEY,
    profile_version=PROFILE_VERSION,
    business_app_code=BUSINESS_APP_CODE,
    build_fallback=build_fallback_report,
    build_prompt=build_report_prompt,
    fallback_warning={
        "level": "info",
        "message": "AI 报告生成暂不可用，当前展示基于规则模板生成的报告。",
    },
)

REVIEW_SUMMARY_PROFILE = ReviewSummaryProfile(
    profile_key=PROFILE_KEY,
    profile_version=PROFILE_VERSION,
    business_app_code=BUSINESS_APP_CODE,
    build_fallback=build_fallback_review_summary,
    build_prompt=build_review_prompt,
)

FINANCE_PROFILE = DomainAgentProfile(
    profile_key=PROFILE_KEY,
    profile_version=PROFILE_VERSION,
    business_app_code=BUSINESS_APP_CODE,
    data_extraction=DATA_EXTRACTION_PROFILE,
    schema_mapping=SCHEMA_MAPPING_PROFILE,
    validation=VALIDATION_PROFILE,
    report=REPORT_PROFILE,
    review_summary=REVIEW_SUMMARY_PROFILE,
)
