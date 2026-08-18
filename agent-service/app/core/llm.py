import logging

from langchain_openai import ChatOpenAI

from app.core.config import settings

logger = logging.getLogger(__name__)

PROVIDER_BASE_URLS: dict[str, str] = {
    "qwen": "https://dashscope.aliyuncs.com/compatible-mode/v1",
    "deepseek": "https://api.deepseek.com/v1",
}


def get_llm(temperature: float = 0.1) -> ChatOpenAI:
    if not settings.llm_api_key:
        logger.warning(
            "LLM API key is not configured (LLM_API_KEY is empty). "
            "Agent graph execution will fail at runtime. Set the environment variable before starting the service."
        )
    base_url = settings.llm_base_url or PROVIDER_BASE_URLS.get(settings.llm_provider, "")
    return ChatOpenAI(
        model=settings.llm_model,
        api_key=settings.llm_api_key,
        base_url=base_url,
        temperature=temperature,
    )
