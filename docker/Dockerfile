# Ondaire MASTER image — REUSES the prebuilt binary from the CI `build` stage
# (already version-stamped via -X main.version and with the SPA embedded) on top
# of the upstream go-librespot image, so Spotify Connect comes for free. Multi-arch
# (amd64 + arm64) follows go-librespot's published platforms.
#
# The build context must contain bin/ with the per-arch binaries
# (ondaire-linux-amd64, ondaire-linux-arm64) produced by the build job. See
# .gitlab-ci.yml (the docker job `needs:` the build job's artifacts) and
# ../QUICKSTART.md.
#
#   docker run --rm --network host \
#     -v /srv/music:/media:ro -v ondaire-data:/data \
#     registry.example/ondaire:latest --name living-room
#
# --network host is required in practice: mDNS discovery, the gossip/stream/
# source ports, and Spotify Connect's zeroconf all need real LAN reachability,
# and ports bind-or-increment when taken — so there is nothing to publish/EXPOSE.

# Select the prebuilt binary for the target arch on the BUILD platform (native,
# no emulation): the binary is pure-Go/static (CGO off) so it runs as-is on alpine.
FROM --platform=$BUILDPLATFORM alpine:3.21 AS pick
ARG TARGETARCH
COPY bin/ /bin-in/
RUN cp "/bin-in/ondaire-linux-${TARGETARCH}" /ondaire && chmod +x /ondaire

# Runtime = upstream go-librespot (alpine; go-librespot on PATH + audio libs) plus
# libopus (master stream-encode) + certs (TLS media sources) + the ondaire binary.
FROM ghcr.io/devgianlu/go-librespot:v0.7.3
RUN apk add --no-cache opus ca-certificates
COPY --from=pick /ondaire /usr/local/bin/ondaire

# Master-only. Media on /media (mount read-only), mutable state on /data. Ports use
# the built-in defaults (bind-or-increment); pass -e ONDAIRE_*_PORT to pin one.
ENV ONDAIRE_ROLE=master \
    ONDAIRE_MEDIA_DIR=/media \
    ONDAIRE_DATA_DIR=/data
VOLUME ["/media", "/data"]

# Override the base entrypoint — ondaire launches go-librespot itself. Clear the
# inherited CMD so stray default args never reach ondaire.
ENTRYPOINT ["ondaire"]
CMD []
