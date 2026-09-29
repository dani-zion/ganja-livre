# ─── Stage 1: dependencies ────────────────────────────────────────────────────
FROM node:24-alpine AS deps

WORKDIR /app
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci

# ─── Stage 2: dev server (hot-reload via bind mount in compose) ───────────────
FROM deps AS dev

COPY frontend/ .
EXPOSE 5173
CMD ["npm", "run", "dev"]

# ─── Stage 3: build static assets ─────────────────────────────────────────────
FROM deps AS build

COPY frontend/ .
COPY backend/schema.graphql /schema/schema.graphql
ENV CODEGEN_SCHEMA=/schema/schema.graphql
RUN npm run build

# ─── Stage 4: nginx serving static assets + GraphQL reverse proxy ─────────────
FROM nginx:1.27-alpine AS prod

COPY --from=build /app/dist /usr/share/nginx/html
COPY frontend/nginx.conf /etc/nginx/conf.d/default.conf
EXPOSE 80
