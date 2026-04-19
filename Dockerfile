FROM node:20-alpine

WORKDIR /app

# The backend consumes the routes package via `file:../packages/routes` —
# copy both before npm install so the link resolves.
COPY packages/routes packages/routes
COPY backend/package*.json backend/
RUN cd backend && npm install --omit=dev

COPY backend/ backend/

RUN addgroup -g 1001 -S nodejs && \
    adduser -S nodejs -u 1001 && \
    chown -R nodejs:nodejs /app
USER nodejs

EXPOSE 3000

HEALTHCHECK --interval=30s --timeout=3s --start-period=10s --retries=3 \
  CMD node -e "require('http').get('http://localhost:3000/health', (r) => {process.exit(r.statusCode === 200 ? 0 : 1)})"

CMD ["node", "backend/server.js"]
