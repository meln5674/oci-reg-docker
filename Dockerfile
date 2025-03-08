ARG PROXY_CACHE
ARG BUILD_IMAGE=docker.io/library/golang:1.23
FROM ${PROXY_CACHE}${BUILD_IMAGE} AS build

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY main.go ./
COPY pkg/proxy ./pkg/proxy
RUN go build -a -tags netgo,osusergo -ldflags '-w -linkmode external -extldflags "-static"' -o registry

FROM scratch
EXPOSE 8080
COPY --from=build /src/registry /registry
ENTRYPOINT ["/registry"]
