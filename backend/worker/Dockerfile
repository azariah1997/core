FROM golang:1.23-alpine AS build
WORKDIR /src
COPY . .
RUN GOWORK=/src/go.work go build -o /out/app ./backend/worker/cmd/worker
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8091
ENTRYPOINT ["/app"]
