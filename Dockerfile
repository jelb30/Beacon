# Build the manager binary.
FROM golang:1.25 AS builder
ARG TARGETOS
ARG TARGETARCH

WORKDIR /workspace
# Copy module files first for Docker layer caching.
COPY go.mod go.mod
COPY go.sum go.sum
# Download dependencies before copying source.
RUN go mod download

# Copy source files.
COPY . .

# Build for the target OS and architecture.
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} go build -a -o manager cmd/main.go

# Use a small non-root runtime image.
FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /workspace/manager .
USER 65532:65532

ENTRYPOINT ["/manager"]
