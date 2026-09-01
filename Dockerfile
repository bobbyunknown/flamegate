# Multi-stage Dockerfile for FlameGate
# Stage 1: Build Frontend with Bun
FROM oven/bun:1-alpine AS frontend-builder
WORKDIR /app/frontend

COPY frontend/package.json frontend/bun.lock* ./
RUN bun install

COPY frontend/ ./
RUN bun run build

# Stage 2: Build Go Backend with Embedded Frontend Assets
FROM golang:alpine AS backend-builder
WORKDIR /app

RUN apk add --no-cache git ca-certificates tzdata
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Copy built frontend dist into static embed folder
RUN rm -rf /app/internal/infrastructure/http/static/dist && \
    mkdir -p /app/internal/infrastructure/http/static/dist
COPY --from=frontend-builder /app/frontend/dist/ /app/internal/infrastructure/http/static/dist/
RUN touch /app/internal/infrastructure/http/static/dist/.gitkeep

ARG VERSION=0.0.1
ARG COMMIT=unknown
ARG BUILDTIME=""

RUN CGO_ENABLED=0 go build -trimpath \
    -ldflags "-X 'main.Version=${VERSION}' -X 'main.Commit=${COMMIT}' -X 'main.BuildTime=${BUILDTIME}' -w -s -buildid=" \
    -o /app/bin/flamegate ./cmd/flamegate

# Stage 3: Minimal Production Runtime
FROM alpine:3.21 AS runner
WORKDIR /app

RUN apk add --no-cache ca-certificates tzdata curl && \
    mkdir -p /root/.flamegate/exts /tmp/flamegate

COPY --from=backend-builder /app/bin/flamegate /usr/local/bin/flamegate

EXPOSE 20180 20181
VOLUME ["/root/.flamegate", "/tmp/flamegate"]

ENTRYPOINT ["/usr/local/bin/flamegate"]
