FROM golang:1.26-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /bin/proofhub-mcp .

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /bin/proofhub-mcp /bin/proofhub-mcp
EXPOSE 8080
ENTRYPOINT ["/bin/proofhub-mcp"]
