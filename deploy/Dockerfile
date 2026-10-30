# Build context is the repository root.

FROM node:24-slim AS frontend
RUN npm install --global pnpm@12
WORKDIR /src/frontend
COPY frontend/package.json frontend/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY frontend/ ./
RUN pnpm build

FROM golang:1.27-trixie AS backend
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        libvips-dev libheif-plugin-libde265 libimage-exiftool-perl \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./

# Runs the tests against libvips from the same Debian release as production.
FROM backend AS test
CMD ["sh", "-c", "go vet ./... && go test ./..."]

# Regenerates the fixtures of internal/media.
FROM backend AS fixtures
RUN apt-get update \
    && apt-get install --yes --no-install-recommends libvips-tools libheif-plugin-x265 \
    && rm -rf /var/lib/apt/lists/*
WORKDIR /src/backend/internal/media/testdata
CMD ["sh", "generate.sh"]

FROM backend AS build
COPY --from=frontend /src/frontend/dist/ internal/web/dist/
RUN mkdir /out && go build -trimpath -ldflags='-s -w' -o /out/ ./cmd/...

FROM debian:trixie-slim
RUN apt-get update \
    && apt-get install --yes --no-install-recommends \
        libvips42t64 libheif-plugin-libde265 libimage-exiftool-perl \
    && rm -rf /var/lib/apt/lists/*
COPY --from=build /out/ /usr/local/bin/
USER 1000:1000
EXPOSE 8080
CMD ["fuchsia-photos", "serve"]
