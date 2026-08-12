# syntax=docker/dockerfile:1
FROM node:24-alpine AS frontend
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.26.5-alpine AS backend
ARG VERSION=dev
WORKDIR /src
RUN apk add --no-cache ca-certificates
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN rm -rf ./internal/webui/dist
COPY --from=frontend /src/web/dist ./internal/webui/dist
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/planexus ./cmd/planexus

FROM scratch
ARG VERSION=dev
LABEL org.opencontainers.image.title="Planexus" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.source="https://github.com/hkjang/Planexus"
COPY --from=backend /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=backend /out/planexus /planexus
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/planexus"]
