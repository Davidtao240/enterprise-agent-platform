FROM python:3.12-slim

WORKDIR /app

# 复制预下载的 Python 包和 requirements
COPY pip-packages/ ./pip-packages/
COPY requirements.txt .

# 从本地缓存安装（离线模式），如果失败则回退到网络
RUN pip install --no-index --find-links=./pip-packages/ -r requirements.txt || \
    pip install -r requirements.txt

COPY . .

EXPOSE 8000
CMD ["uvicorn", "app.main:app", "--host", "0.0.0.0", "--port", "8000"]
