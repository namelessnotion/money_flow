# Build context is the repo root (see docker-compose.yml).
FROM node:22-slim

WORKDIR /app

COPY client/package.json client/package-lock.json ./
RUN npm install

COPY client/ .

EXPOSE 5173

CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
