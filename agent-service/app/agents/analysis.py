"""AnalysisAgent: performs financial analysis on collected data.

Specializes in:
- Trend analysis
- Variance calculation
- Key metric computation
- Anomaly detection
"""

from __future__ import annotations

import logging
from typing import Any

from app.agents.base import BaseAgent

logger = logging.getLogger(__name__)


class AnalysisAgent(BaseAgent):
    agent_id = "analysis_agent"
    domain = "finance"
    reusable_scope = "finance"

    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        collected = state.get("sub_results", {}).get("data_collection", {})
        data = collected.get("data", [])

        analysis = self._analyze(data)
        result = {
            "task": "analysis",
            "status": "completed",
            "analysis": analysis,
        }
        return {"sub_results": {**state.get("sub_results", {}), "analysis": result}}

    def _analyze(self, data: list[dict[str, Any]]) -> dict[str, Any]:
        if not data:
            return {"findings": [], "metrics": {}, "note": "No data to analyze"}

        # Compute basic metrics
        numeric_fields = self._find_numeric_fields(data)
        metrics = {}
        for field in numeric_fields[:5]:  # Limit to top 5 numeric fields
            values = [row.get(field, 0) for row in data if isinstance(row.get(field), (int, float))]
            if values:
                metrics[field] = {
                    "sum": sum(values),
                    "avg": sum(values) / len(values),
                    "min": min(values),
                    "max": max(values),
                    "count": len(values),
                }

        # Detect anomalies
        anomalies = self._detect_anomalies(metrics)

        return {
            "metrics": metrics,
            "anomalies": anomalies,
            "record_count": len(data),
            "findings": self._generate_findings(metrics, anomalies),
        }

    def _find_numeric_fields(self, data: list[dict[str, Any]]) -> list[str]:
        if not data:
            return []
        fields = set()
        for row in data:
            for k, v in row.items():
                if isinstance(v, (int, float)):
                    fields.add(k)
        return list(fields)

    def _detect_anomalies(self, metrics: dict[str, Any]) -> list[str]:
        anomalies = []
        for field, stats in metrics.items():
            if stats.get("count", 0) > 0:
                avg = stats.get("avg", 0)
                if avg != 0:
                    # Flag values > 2x average
                    if stats.get("max", 0) > avg * 2:
                        anomalies.append(f"{field}: max ({stats['max']}) exceeds 2x average ({avg:.2f})")
        return anomalies

    def _generate_findings(self, metrics: dict, anomalies: list) -> list[str]:
        findings = []
        if anomalies:
            findings.append(f"Detected {len(anomalies)} anomalies requiring review")
        if metrics:
            top_metric = list(metrics.keys())[0]
            findings.append(f"Analyzed {len(metrics)} metric fields with {top_metric} as primary indicator")
        return findings