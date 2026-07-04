# Marketing-site image: build the static site, serve it with nginx.
# Build context is ./site:
#   docker build -t ondaire-site -f site/Dockerfile site
#   docker run --rm -p 8080:80 ondaire-site   →  http://localhost:8080
#
# Multi-arch friendly: the generator runs once on the BUILD platform (its output
# is byte-identical across arches) and only the nginx layer is per-arch — no
# emulation needed (the final stage has no RUN).

FROM --platform=$BUILDPLATFORM node:24-alpine AS build
ARG VER=
ENV ONDAIRE_VERSION=$VER
WORKDIR /site
COPY . .
# The CI job stages bin/ondaire-linux-* into src/assets/downloads/ before this
# build (see .gitlab-ci.yml docker-site); build.mjs hashes them into the download
# page and they ship under /site/dist for direct download.
RUN node build.mjs            # → /site/dist (fully static)

FROM nginx:1.27-alpine
COPY --from=build /site/dist /usr/share/nginx/html
# nginx:alpine already serves /usr/share/nginx/html on :80 with a sane default.
EXPOSE 80
