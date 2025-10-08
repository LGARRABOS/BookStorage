FROM python:3.11-slim

ENV PYTHONDONTWRITEBYTECODE=1 \
    PYTHONUNBUFFERED=1

ARG APP_DIR=/srv/bookstorage
WORKDIR ${APP_DIR}

COPY requirements.txt ./
RUN pip install --no-cache-dir -r requirements.txt

COPY . ${APP_DIR}
RUN install -m 755 docker-entrypoint.sh /usr/local/bin/bookstorage-entrypoint

ENV BOOKSTORAGE_ENV=production \
    BOOKSTORAGE_SECRET_KEY=change-me \
    BOOKSTORAGE_HOST=0.0.0.0 \
    BOOKSTORAGE_PORT=5000 \
    BOOKSTORAGE_DATA_DIR=/data \
    BOOKSTORAGE_DATABASE=database.db \
    BOOKSTORAGE_UPLOAD_DIR=/data/images \
    BOOKSTORAGE_UPLOAD_URL_PATH=images \
    BOOKSTORAGE_AVATAR_DIR=/data/avatars \
    BOOKSTORAGE_AVATAR_URL_PATH=avatars \
    BOOKSTORAGE_APP_DIR=${APP_DIR}

RUN mkdir -p /data/images /data/avatars

VOLUME ["/data"]

EXPOSE 5000

ENTRYPOINT ["bookstorage-entrypoint"]
CMD ["python", "app.py"]
