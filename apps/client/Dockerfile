FROM oven/bun:1 AS build
WORKDIR /app
COPY package.json bun.lock* ./
RUN bun install --frozen-lockfile 2>/dev/null || bun install
COPY . .
RUN bun run build

FROM oven/bun:1-slim
WORKDIR /app
COPY --from=build /app/build build/
COPY --from=build /app/package.json .
EXPOSE 3000
ENV NODE_ENV=production
CMD ["bun", "build/index.js"]
