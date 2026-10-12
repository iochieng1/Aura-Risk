# Backend image: one static binary that serves the API and also runs
# `aurarisk-backend migrate`. Build from aurarisk-backend/:
#   docker build -t aurarisk-backend .
FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/aurarisk-backend .

# distroless/static has CA certificates (for Open-Meteo, Postgres TLS, S3)
# and runs as a non-root user.
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/aurarisk-backend /aurarisk-backend
ENV APP_ENV=production PORT=8080
EXPOSE 8080
ENTRYPOINT ["/aurarisk-backend"]
