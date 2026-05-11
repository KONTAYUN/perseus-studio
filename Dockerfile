ARG IMAGE_REGISTRY=docker.io

FROM ${IMAGE_REGISTRY}/library/node:20-bookworm AS frontend-build
WORKDIR /src
COPY package*.json ./
RUN npm ci
COPY index.html postcss.config.js tailwind.config.js vite.config.js ./
COPY src ./src
RUN npm run build

FROM ${IMAGE_REGISTRY}/library/golang:1.25-bookworm AS backend-build
ARG APP_VERSION=dev
ARG APP_COMMIT=unknown
ARG APP_BUILT_AT=unknown
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend ./
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags "-X webide-backend/internal/version.Version=${APP_VERSION} -X webide-backend/internal/version.Commit=${APP_COMMIT} -X webide-backend/internal/version.BuiltAt=${APP_BUILT_AT}" \
    -o /out/webide-backend ./cmd/server

FROM ${IMAGE_REGISTRY}/library/debian:bookworm-slim AS runtime
ENV BACKEND_ADDR=:8080 \
    BACKEND_DB_PATH=/data/app.db \
    WORKSPACE_ROOT=/data/workspace \
    UPLOADS_ROOT=/data/uploads \
    SKILLS_ROOT=/data/skills \
    FRONTEND_DIST=/app/dist \
    TASK_WORKER_COUNT=2 \
    USER_DATA_ROOT=/data/users

RUN apt-get update \
    && apt-get install -y --no-install-recommends \
      ca-certificates \
      curl \
      git \
      python3 \
      python3-pip \
      unzip \
      zip \
    && python3 -m pip install --no-cache-dir --break-system-packages \
      requests \
      beautifulsoup4 \
      pyyaml \
      pillow \
      pandas \
      openpyxl \
      python-docx \
      pypdf \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app
COPY --from=backend-build /out/webide-backend /app/webide-backend
COPY --from=frontend-build /src/dist /app/dist

RUN mkdir -p /data/workspace /data/uploads /data/skills /data/users

EXPOSE 8080
CMD ["/app/webide-backend"]
