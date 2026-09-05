FROM golang:1.26-alpine AS build

WORKDIR /src
COPY go.mod ./
COPY *.go ./
COPY internal/ ./internal/
COPY web/ ./web/
COPY data/dtw.json ./data/dtw.json

RUN CGO_ENABLED=0 go build -buildvcs=false -trimpath -ldflags="-s -w" -o /out/atc-sim .

FROM scratch

LABEL org.opencontainers.image.title="DTW Metro Tower" \
      org.opencontainers.image.description="Independent single-player air traffic control games for Detroit Metropolitan Airport." \
      org.opencontainers.image.source="https://github.com/lab1702/atc-sim" \
      org.opencontainers.image.licenses="MIT AND ODbL-1.0"

COPY --from=build /out/atc-sim /atc-sim
COPY LICENSE /licenses/LICENSE
COPY data/SOURCES.md /licenses/DATA-SOURCES.md
COPY web/vendor/THREE-LICENSE.txt /licenses/THREE-LICENSE.txt

USER 65532:65532
EXPOSE 8080
# The server handles os.Interrupt for shutdown.
STOPSIGNAL SIGINT
ENTRYPOINT ["/atc-sim"]
CMD ["-addr", "0.0.0.0:8080"]
