# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM node:25-alpine AS web
RUN npm install -g pnpm@11.17.0
WORKDIR /src/web
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
RUN mkdir -p /src/internal/server/webui/dist && pnpm build

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/server/webui/dist ./internal/server/webui/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -tags timetzdata \
      -ldflags "-s -w -X github.com/mby/fenghuo/internal/version.Version=${VERSION}" \
      -o /out/fenghuo-server ./cmd/fenghuo-server \
 && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/fenghuo-server /usr/local/bin/fenghuo-server
COPY --from=build --chown=65532:65532 /out/data /data
ENV FENGHUO_DATA=/data \
    FENGHUO_LISTEN=:8080 \
    FENGHUO_ADMIN_LISTEN=:8081
VOLUME /data
EXPOSE 8080 8081
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/fenghuo-server"]
CMD ["serve"]
