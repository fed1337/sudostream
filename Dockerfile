ARG DISTROLESS_TAG=nonroot
# Distroless nonroot defaults to 65532:65532. Override at build time if host media
# ownership must match (e.g. docker build --build-arg APP_UID=$(id -u) --build-arg APP_GID=$(id -g)).
ARG APP_UID=65532
ARG APP_GID=65532

FROM node:26-trixie-slim AS frontend

RUN npm install -g pnpm@latest

WORKDIR /src/web

COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN --mount=type=cache,target=/root/.local/share/pnpm/store \
    pnpm install --frozen-lockfile

COPY web/ ./
RUN pnpm run build

FROM --platform=$BUILDPLATFORM golang:1.26-trixie AS builder

WORKDIR /src

ARG TARGETARCH
ARG BUILD_VERSION=dev
ARG BUILD_REVISION=unknown
ARG IMAGE_REF_NAME=local

COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .
COPY --from=frontend /src/web/dist ./internal/frontend/dist

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eux; \
    SHORT_REV="${BUILD_REVISION}"; \
    SHORT_REV=$(printf '%.7s' "$BUILD_REVISION"); \
    TAGS="embed nomsgpack"; \
    if [ "$TARGETARCH" = "amd64" ]; then TAGS="$TAGS sonic avx"; fi; \
    CGO_ENABLED=0 GOOS=linux GOARCH="$TARGETARCH" go build \
    -tags="$TAGS" \
    -trimpath \
    -ldflags="-s -w \
    -X sudoStream/internal/version.Build=${BUILD_VERSION} \
    -X sudoStream/internal/version.Revision=${SHORT_REV} \
    -X sudoStream/internal/version.Ref=${IMAGE_REF_NAME}" \
    -o /out/sudostream \
    ./cmd/server

FROM --platform=$BUILDPLATFORM debian:trixie-slim AS ffmpeg

ARG TARGETARCH
ARG JELLYFIN_FFMPEG_VERSION=8.1.2-4

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl xz-utils \
    && rm -rf /var/lib/apt/lists/*

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN set -eux; \
    case "$TARGETARCH" in \
    amd64) variant=linux64 ;; \
    arm64) variant=linuxarm64 ;; \
    *) echo "unsupported TARGETARCH: $TARGETARCH" >&2; exit 1 ;; \
    esac; \
    url="https://github.com/jellyfin/jellyfin-ffmpeg/releases/download/v${JELLYFIN_FFMPEG_VERSION}/jellyfin-ffmpeg_${JELLYFIN_FFMPEG_VERSION}_portable_${variant}-gpl.tar.xz"; \
    curl -fsSL -o /tmp/ffmpeg.tar.xz "$url"; \
    mkdir -p /opt/ffmpeg; \
    tar -xJf /tmp/ffmpeg.tar.xz -C /opt/ffmpeg; \
    chmod +x /opt/ffmpeg/ffmpeg /opt/ffmpeg/ffprobe; \
    rm /tmp/ffmpeg.tar.xz

# amd64: Intel/AMD VA + OneVPL + Vulkan/OpenCL loaders.
# arm64: portable jellyfin-ffmpeg embeds RKMPP/RGA; install OpenCL ICD loader for Mali tonemap when /dev/mali0 is passed through.
FROM debian:trixie-slim AS hw-prep

ARG TARGETARCH
ARG APP_UID=65532
ARG APP_GID=65532

SHELL ["/bin/bash", "-o", "pipefail", "-c"]

RUN set -eux; \
    mkdir -p /opt/hw-install/usr; \
    if [ "$TARGETARCH" = "arm64" ]; then \
    apt-get update \
    && apt-get install -y --no-install-recommends ocl-icd-libopencl1 \
    && rm -rf /var/lib/apt/lists/*; \
    LIBARCH=aarch64-linux-gnu; \
    LIB="/usr/lib/${LIBARCH}"; \
    mkdir -p "/opt/hw-install/usr/lib/${LIBARCH}"; \
    cp -a "${LIB}/libOpenCL.so"* "/opt/hw-install/usr/lib/${LIBARCH}/" 2>/dev/null || true; \
    chown -R "${APP_UID}:${APP_GID}" /opt/hw-install; \
    exit 0; \
    fi; \
    if [ "$TARGETARCH" != "amd64" ]; then \
    chown -R "${APP_UID}:${APP_GID}" /opt/hw-install; \
    exit 0; \
    fi; \
    apt-get update \
    && apt-get install -y --no-install-recommends \
    intel-media-va-driver \
    mesa-va-drivers \
    mesa-vulkan-drivers \
    libva2 \
    libva-drm2 \
    libdrm2 \
    ocl-icd-libopencl1 \
    libvpl2 \
    && rm -rf /var/lib/apt/lists/*; \
    LIBARCH=x86_64-linux-gnu; \
    LIB="/usr/lib/${LIBARCH}"; \
    mkdir -p /opt/hw/lib /opt/hw/dri; \
    cp -a "${LIB}/dri/." /opt/hw/dri/; \
    for pattern in libva.so libva-drm.so libdrm.so libigdgmm.so libvulkan.so libOpenCL.so libvpl.so; do \
    cp -a "${LIB}/${pattern}"* /opt/hw/lib/ 2>/dev/null || true; \
    done; \
    mkdir -p "/opt/hw-install/usr/lib/${LIBARCH}/dri"; \
    cp -a /opt/hw/dri/. "/opt/hw-install/usr/lib/${LIBARCH}/dri/"; \
    cp -a /opt/hw/lib/. "/opt/hw-install/usr/lib/${LIBARCH}/"; \
    chown -R "${APP_UID}:${APP_GID}" /opt/hw-install

FROM debian:trixie-slim AS runtime-prep

ARG APP_UID=65532
ARG APP_GID=65532

RUN mkdir -p /var/lib/sudostream/cache/thumbs /var/lib/sudostream/cache/hls /var/lib/sudostream/logs \
    && chown -R "${APP_UID}:${APP_GID}" /var/lib/sudostream

FROM gcr.io/distroless/cc-debian13:${DISTROLESS_TAG}

ARG BUILD_VERSION=dev
ARG BUILD_REVISION=unknown
ARG BUILD_CREATED=unknown
ARG IMAGE_REF_NAME=local
ARG JELLYFIN_FFMPEG_VERSION=8.1.2-4
ARG TARGETARCH
ARG DISTROLESS_TAG=nonroot
ARG APP_UID=65532
ARG APP_GID=65532

LABEL org.opencontainers.image.title="sudoStream" \
    org.opencontainers.image.description="Self-hosted media server using jellyfin-ffmpeg" \
    org.opencontainers.image.source="https://github.com/fed1337/sudostream" \
    org.opencontainers.image.url="https://github.com/fed1337/sudostream" \
    org.opencontainers.image.documentation="https://github.com/fed1337/sudostream/blob/master/README.md" \
    org.opencontainers.image.licenses="MIT" \
    org.opencontainers.image.vendor="fed1337" \
    org.opencontainers.image.version="${BUILD_VERSION}" \
    org.opencontainers.image.revision="${BUILD_REVISION}" \
    org.opencontainers.image.created="${BUILD_CREATED}" \
    org.opencontainers.image.ref.name="${IMAGE_REF_NAME}" \
    org.opencontainers.image.base.name="gcr.io/distroless/cc-debian13:${DISTROLESS_TAG}"

WORKDIR /

ENV GIN_MODE=release

COPY --from=ffmpeg --chown=${APP_UID}:${APP_GID} /opt/ffmpeg/ffmpeg /opt/ffmpeg/ffprobe /usr/local/bin/
COPY --from=builder --chown=${APP_UID}:${APP_GID} /out/sudostream /usr/local/bin/sudostream
COPY --from=runtime-prep --chown=${APP_UID}:${APP_GID} /var/lib/sudostream /var/lib/sudostream
COPY --from=hw-prep --chown=${APP_UID}:${APP_GID} /opt/hw-install/usr /usr

USER ${APP_UID}:${APP_GID}

EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/sudostream"]
