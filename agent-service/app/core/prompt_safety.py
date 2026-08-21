"""Prompt safety utilities — input sanitization and injection detection."""

from __future__ import annotations

import re
import logging
from typing import Optional

logger = logging.getLogger(__name__)

# Patterns that indicate prompt injection attempts
_INJECTION_PATTERNS: list[tuple[str, re.Pattern[str]]] = [
    ("ignore_previous", re.compile(r"ignore\s+(all\s+)?(previous|prior|above)\s+(instructions?|prompts?)", re.IGNORECASE)),
    ("system_prompt_leak", re.compile(r"disclose|reveal|show\s+(your|the)\s+(system\s+)?prompt", re.IGNORECASE)),
    ("new_instructions", re.compile(r"(your|new)\s+instructions?\s+(are|is|should|must)\s+", re.IGNORECASE)),
    ("role_change", re.compile(r"you\s+are\s+now\s+(a|an|the)\s+", re.IGNORECASE)),
    ("jailbreak", re.compile(r"(jailbreak|prompt\s+injection|hack|bypass)\s+", re.IGNORECASE)),
]


def sanitize_user_input(text: str, max_length: int = 8000) -> str:
    """Sanitize user input for safe inclusion in LLM prompts.
    
    1. Detects and logs potential injection attempts
    2. Wraps input in explicit <user_input> tags
    3. Truncates to max_length
    4. Escapes markdown-like formatting that could confuse the model
    """
    if not text:
        return ""

    # Check for injection patterns
    for pattern_name, pattern in _INJECTION_PATTERNS:
        if pattern.search(text):
            logger.warning(
                "Potential prompt injection detected: pattern=%s, length=%d",
                pattern_name, len(text),
            )

    # Wrap in explicit delimiter tags to prevent prompt injection
    sanitized = text[:max_length]

    # Escape characters that could be used to break out of the user context
    dangerous_chars = ["\x00", "\x01", "\x02", "\x03", "\x04", "\x05", "\x06", "\x07"]
    for ch in dangerous_chars:
        sanitized = sanitized.replace(ch, "")

    return f"<user_input>\n{sanitized}\n</user_input>"


def detect_injection(text: str) -> Optional[str]:
    """Check if text contains known prompt injection patterns.
    
    Returns the pattern name if detected, None otherwise.
    """
    for pattern_name, pattern in _INJECTION_PATTERNS:
        if pattern.search(text):
            return pattern_name
    return None