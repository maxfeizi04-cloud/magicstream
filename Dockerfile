FROM golang:1.26.1-alpine AS builder

RUN apk add --no-cache git ca-cartificates

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

    # 开发调试不要加-ldflags= -s -w
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -ldflags="" -o magicstream ./cmd/magicstream

FROM alpine:3.21

RUN apk add --no-cache ca-certificates ffmpeg tzdata curl

WORKDIR /app

# 从构建阶段复制编译产物
COPY --from=builder /app/magicstream .

COPY configs/ /app/configs/
COPY scripts/ /app/scripts/

RUN mkdir -p /app/data/videos /app/data/live /app/data/uploads

RUN adduser -D -H magicstream && \
    chown -R magicstream:magicstream /app

USER magicstream

EXPOSE 8080 1935

ENTRYPOINT ["./magicstream"]
