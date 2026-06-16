"""Base agent class. Each agent in the registry must implement this interface."""

from abc import ABC, abstractmethod
from typing import Any


class BaseAgent(ABC):
    agent_id: str
    domain: str

    @abstractmethod
    async def run(self, state: dict[str, Any]) -> dict[str, Any]:
        ...
