FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY *.go ./
# timetzdata embebe la base de zonas horarias: la imagen final no trae /usr/share/zoneinfo y los
# recibos usan la hora local (TZ).
RUN CGO_ENABLED=0 go build -trimpath -tags timetzdata -ldflags="-s -w" -o /out/tenis-ahora-api .

FROM gcr.io/distroless/static-debian12
WORKDIR /app
COPY --from=build /out/tenis-ahora-api /app/tenis-ahora-api
COPY seed/ /app/seed/
ENV TZ=America/Argentina/Buenos_Aires
# Mismo uid que el chown del deploy sobre el volumen de datos.
USER 1000:1000
EXPOSE 5090
ENTRYPOINT ["/app/tenis-ahora-api"]
