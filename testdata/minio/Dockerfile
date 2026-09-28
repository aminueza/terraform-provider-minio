ARG GO_IMAGE=golang:1.26.8
FROM ${GO_IMAGE} AS build
ARG MINIO_VERSION
ARG MC_VERSION
ENV CGO_ENABLED=0 GOFLAGS=-mod=mod
RUN go install -ldflags "-s -w -X github.com/minio/minio/cmd.Version=${MINIO_VERSION#RELEASE.} -X github.com/minio/minio/cmd.ReleaseTag=${MINIO_VERSION}" "github.com/minio/minio@${MINIO_VERSION}"
RUN go install -ldflags "-s -w -X github.com/minio/mc/cmd.Version=${MC_VERSION#RELEASE.} -X github.com/minio/mc/cmd.ReleaseTag=${MC_VERSION}" "github.com/minio/mc@${MC_VERSION}"

FROM alpine:3.22
RUN apk add --no-cache ca-certificates
COPY --from=build /go/bin/minio /go/bin/mc /usr/bin/
EXPOSE 9000 9001
ENTRYPOINT ["minio"]
