FROM oven/bun:1.3.4 AS deps
WORKDIR /app
COPY package.json bun.lock bunfig.toml ./
RUN bun install --frozen-lockfile

FROM deps AS build
COPY tsconfig.json .oxlintrc.json ./
COPY src ./src
RUN bun run build

FROM oven/bun:1.3.4 AS runtime
WORKDIR /app
ENV NODE_ENV=production
COPY --from=build /app/dist ./dist
EXPOSE 8080
CMD ["bun", "dist/index.js"]
