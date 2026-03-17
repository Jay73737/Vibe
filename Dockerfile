FROM golang:1.21-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /vibe ./cmd/vibe/

FROM alpine:latest
RUN apk --no-cache add ca-certificates
COPY --from=build /vibe /usr/local/bin/vibe
EXPOSE 7433
VOLUME ["/repo"]
WORKDIR /repo
ENTRYPOINT ["vibe"]
CMD ["serve", "--port", "7433"]
