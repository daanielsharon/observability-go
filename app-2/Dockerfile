FROM golang:1.24-alpine AS builder
WORKDIR /app
# Set timezone for builder
RUN apk add --no-cache tzdata
ENV TZ=Asia/Jakarta
COPY . .
RUN go mod tidy && go build -o main main.go

FROM alpine:latest
# Set timezone for runtime
RUN apk add --no-cache tzdata ca-certificates && \
    cp /usr/share/zoneinfo/Asia/Jakarta /etc/localtime && \
    echo "Asia/Jakarta" > /etc/timezone
ENV TZ=Asia/Jakarta
WORKDIR /root/
COPY --from=builder /app/main .
CMD ["./main"]