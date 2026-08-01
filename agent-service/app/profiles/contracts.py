"""Capability-specific contracts for versioned domain profiles.

Shared agents execute these contracts without selecting a business domain.
The explicit graph is responsible for binding the appropriate profile.
"""

from __future__ import annotations

from dataclasses import dataclass
from typing import Any, Callable, Mapping, Sequence

Issue = dict[str, Any]
Record = dict[str, Any]

MappingPromptBuilder = Callable[[Sequence[str]], str]
FallbackDataProvider = Callable[[], tuple[list[str], list[Record]]]
ValidationExecutor = Callable[[list[Record]], tuple[list[Issue], list[Issue]]]
ReportFallbackBuilder = Callable[
    [dict[str, Any], dict[str, Any], list[Issue]],
    dict[str, Any],
]
ReportPromptBuilder = Callable[
    [dict[str, Any], dict[str, Any], dict[str, Any], list[Issue]],
    str,
]
ReviewFallbackBuilder = Callable[
    [dict[str, Any], dict[str, Any], list[Issue]],
    dict[str, Any],
]
ReviewPromptBuilder = Callable[
    [dict[str, Any], dict[str, Any], list[Issue]],
    str,
]


@dataclass(frozen=True)
class DataExtractionProfile:
    profile_key: str
    profile_version: str
    business_app_code: str
    fallback_data: FallbackDataProvider
    fallback_warning: str


@dataclass(frozen=True)
class SchemaMappingProfile:
    profile_key: str
    profile_version: str
    business_app_code: str
    canonical_fields: tuple[str, ...]
    aliases: Mapping[str, str]
    build_prompt: MappingPromptBuilder


@dataclass(frozen=True)
class ValidationProfile:
    profile_key: str
    profile_version: str
    business_app_code: str
    required_fields: tuple[str, ...]
    validate_rows: ValidationExecutor


@dataclass(frozen=True)
class ReportProfile:
    profile_key: str
    profile_version: str
    business_app_code: str
    build_fallback: ReportFallbackBuilder
    build_prompt: ReportPromptBuilder
    fallback_warning: Issue


@dataclass(frozen=True)
class ReviewSummaryProfile:
    profile_key: str
    profile_version: str
    business_app_code: str
    build_fallback: ReviewFallbackBuilder
    build_prompt: ReviewPromptBuilder


@dataclass(frozen=True)
class DomainAgentProfile:
    profile_key: str
    profile_version: str
    business_app_code: str
    data_extraction: DataExtractionProfile
    schema_mapping: SchemaMappingProfile
    validation: ValidationProfile
    report: ReportProfile
    review_summary: ReviewSummaryProfile
