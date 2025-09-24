FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

WORKDIR /app

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY . .

ENV BOOKSTORAGE_ENV=production \
    BOOKSTORAGE_SECRET_KEY=change-me \
    BOOKSTORAGE_HOST=0.0.0.0 \
    BOOKSTORAGE_PORT=5000 \
    BOOKSTORAGE_DATA_DIR=/data \
    BOOKSTORAGE_DATABASE=database.db \
    BOOKSTORAGE_UPLOAD_DIR=/data/images \
    BOOKSTORAGE_UPLOAD_URL_PATH=images \
    BOOKSTORAGE_AVATAR_DIR=/data/avatars \
    BOOKSTORAGE_AVATAR_URL_PATH=avatars

RUN mkdir -p /data/images /data/avatars

VOLUME ["/data"]

EXPOSE 5000

ENTRYPOINT ["/app/docker-entrypoint.sh"]
CMD ["python", "app.py"]
