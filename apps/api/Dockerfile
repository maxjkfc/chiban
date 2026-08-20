FROM golang:1.26-alpine AS build
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .
# nodynamic is load-bearing, not a tuning knob. The HEIC decoder ships a purego
# path that dlopens a system libheif, and purego reaches dlopen through
# //go:cgo_import_dynamic — which gives the binary an ELF interpreter even with
# CGO_ENABLED=0. distroless/static has no loader, so such a binary dies at exec
# with "no such file or directory", a message that names neither the cause nor
# the package. The tag drops that path entirely; the decoder is the embedded
# WASM one, which is what internal/media forces at runtime anyway.
RUN CGO_ENABLED=0 go build -tags nodynamic -o /out/api ./cmd/api
RUN CGO_ENABLED=0 go build -tags nodynamic -o /out/migrate ./cmd/migrate

# `go build` succeeds either way, so nothing above can fail on this. Without
# this check the next dependency that pulls in cgo_import_dynamic is found by a
# person watching a deployment 502 for two minutes.
RUN for bin in /out/api /out/migrate; do \
		if grep -qa 'ld-linux\|ld-musl' "$bin"; then \
			echo "$bin is dynamically linked; distroless/static cannot exec it" >&2; \
			exit 1; \
		fi; \
	done

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/api /api
COPY --from=build /out/migrate /migrate
USER nonroot:nonroot
EXPOSE 8080
ENTRYPOINT ["/api"]
