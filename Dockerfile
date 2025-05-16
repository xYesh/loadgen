# Stage 1: Build the Go application
FROM jfrog.fkinternal.com/flow-step-custom/golang:1.21.5-swag-git-debian11.8 as builder
ENV GOPROXY="https://jfrog.fkinternal.com/artifactory/api/go/go_virtual"
# Set the working directory inside the container
WORKDIR /app

RUN go clean -modcache
# Copy the Go modules manifests
COPY go.mod go.sum ./

# Download the Go modules
RUN go mod download

# Copy the source code
COPY . .

RUN ls -la

# Build the Go application
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -buildvcs=false -a -installsuffix cgo -o /app/loadgen .

# Stage 2: Create a smaller image for running the application
FROM jfrog.fkinternal.com/flow-step-custom/golang:1.21.5-swag-git-debian11.8

# Set the working directory inside the container
WORKDIR /app
COPY ./config.yaml /app/
# Copy the compiled Go application from the builder stage
COPY --from=builder /app/loadgen /app/loadgen

USER 1000
# Set the entry point to run the application
ENTRYPOINT ["/app/loadgen","--config=config.yaml"]
