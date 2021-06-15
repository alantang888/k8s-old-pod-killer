FROM golang:1.16 AS builder

WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd/
WORKDIR /app/cmd/k8s-old-pod-killer
RUN go build -o /bin/k8s-old-pod-killer .

FROM gcr.io/distroless/base
COPY --from=builder /bin/k8s-old-pod-killer /

CMD ["/k8s-old-pod-killer"]
