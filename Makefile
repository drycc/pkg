# Short name: Short name, following [a-zA-Z_], used all over the place.
# Some uses for short name:
# - Container image name
# - Kubernetes service, rc, pod, secret, volume names
REPO_PATH := github.com/drycc/pkg
DEV_ENV_IMAGE := ${DEV_REGISTRY}/drycc/go-dev
DEV_ENV_WORK_DIR := /opt/drycc/go/src/${REPO_PATH}

# Enable vendor/ directory support.
export GO15VENDOREXPERIMENT=1

# SemVer with build information is defined in the SemVer 2 spec, but Container
# doesn't allow +, so we use -.
VERSION := 0.0.1-$(shell date "+%Y%m%d%H%M%S")

# Common flags passed into Go's linker.
LDFLAGS := "-s -X main.version=${VERSION}"

PKG_DIRS := ./...
DEV_ENV_CMD := podman run --rm -v ${CURDIR}:${DEV_ENV_WORK_DIR} -w ${DEV_ENV_WORK_DIR} ${DEV_ENV_IMAGE}

bootstrap:
	${DEV_ENV_CMD} go mod vendor

all: build test

build:
	${DEV_ENV_CMD} go build ${PKG_DIRS}

test: build test-style
	${DEV_ENV_CMD} go test -race -cover -coverprofile=coverage.txt -covermode=atomic ${PKG_DIRS}

test-style: bootstrap
	${DEV_ENV_CMD} lint

.PHONY: all build test
