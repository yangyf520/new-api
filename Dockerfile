FROM harbor.sensetime.com/infra/oven/bun:v1 AS builder

WORKDIR /build/web
COPY web/package.json web/bun.lock ./
COPY web/default/package.json ./default/package.json
COPY web/classic/package.json ./classic/package.json
RUN bun install --registry=https://registry.npmmirror.com --frozen-lockfile
COPY ./web/default ./default
COPY ./VERSION /build/VERSION
RUN cd default && DISABLE_ESLINT_PLUGIN='true' VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build

FROM harbor.sensetime.com/infra/oven/bun:v1 AS builder-classic

WORKDIR /build/web
COPY web/package.json web/bun.lock ./
COPY web/default/package.json ./default/package.json
COPY web/classic/package.json ./classic/package.json
RUN bun install --registry=https://registry.npmmirror.com --frozen-lockfile
COPY ./web/classic ./classic
COPY ./VERSION /build/VERSION
RUN cd classic && VITE_REACT_APP_VERSION=$(cat /build/VERSION) bun run build

FROM harbor.sensetime.com/infra/golang:v1 AS builder2
ENV GO111MODULE=on CGO_ENABLED=0

# 1. 声明接收外部传进来的参数
ARG GOPROXY
ARG GONOPROXY

# 2. 将参数设置为环境变量，供后续的 go 命令使用
ENV GOPROXY=$GOPROXY
ENV GONOPROXY=$GONOPROXY

ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64}
ENV GOEXPERIMENT=greenteagc

WORKDIR /build

ADD go.mod go.sum ./
RUN go mod download

COPY . .
COPY --from=builder /build/web/default/dist ./web/default/dist
COPY --from=builder-classic /build/web/classic/dist ./web/classic/dist
RUN go build -ldflags "-s -w -X 'github.com/QuantumNous/new-api/common.Version=$(cat VERSION)'" -o new-api

FROM harbor.sensetime.com/infra/debian:bookworm-slimv1

RUN sed -i 's|http://deb.debian.org|http://mirrors.aliyun.com|g' \
    /etc/apt/sources.list.d/debian.sources  && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates tzdata libasan8 wget \
    && rm -rf /var/lib/apt/lists/* \
    && update-ca-certificates

COPY --from=builder2 /build/new-api /
COPY LICENSE NOTICE THIRD-PARTY-LICENSES.md /licenses/
EXPOSE 3000
WORKDIR /data
ENTRYPOINT ["/new-api"]