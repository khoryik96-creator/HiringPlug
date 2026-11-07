# The web app: built once, served by nginx, which also forwards /v1 to the API.
FROM node:22-slim AS build
RUN corepack enable
WORKDIR /web
COPY web/package.json web/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY web/ ./
# vite.config.ts reads the security headers (for `vite preview`).
COPY deploy/security-headers.conf /deploy/security-headers.conf
RUN pnpm run build

FROM nginx:1.29-alpine
COPY deploy/nginx.conf /etc/nginx/conf.d/default.conf
COPY deploy/security-headers.conf /etc/nginx/security-headers.conf
COPY --from=build /web/dist /usr/share/nginx/html
