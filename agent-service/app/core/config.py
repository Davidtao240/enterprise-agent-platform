from pydantic_settings import BaseSettings


class Settings(BaseSettings):
    model_config = {
        "env_file": ("../../.env", "../.env", ".env"),
        "env_file_encoding": "utf-8",
        "extra": "ignore",
    }

    # LLM
    llm_provider: str = "qwen"
    llm_model: str = "qwen-plus"
    llm_api_key: str = ""
    llm_base_url: str = "https://dashscope.aliyuncs.com/compatible-mode/v1"

    # MinIO
    minio_endpoint: str = "localhost:9000"
    minio_access_key: str = "minioadmin"
    minio_secret_key: str = "minioadmin"
    minio_bucket: str = "platform-files"
    file_service_url: str | None = None

    # Qdrant
    qdrant_host: str = "localhost"
    qdrant_port: int = 6333


settings = Settings()
