FROM node:20-alpine AS frontend-builder
WORKDIR /app/frontend
ENV NPM_CONFIG_REGISTRY=https://registry.npmmirror.com
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.24-alpine AS builder
WORKDIR /app
ENV GOPROXY=https://goproxy.cn,direct
COPY backend/go.mod ./
COPY backend/ .
RUN go mod tidy
RUN CGO_ENABLED=0 GOOS=linux go build -o server .

FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /app/server .
COPY --from=frontend-builder /app/frontend/dist ./static
EXPOSE 8080
CMD ["./server"]
