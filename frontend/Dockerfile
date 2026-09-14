# --- Build stage ---
# Installs deps and builds the static bundle inside a container with
# Node already in it, so you don't need Node installed on your own machine.
FROM node:20-alpine AS build
WORKDIR /app
COPY package.json ./
RUN npm install
COPY . .
# VITE_API_URL is baked into the static bundle at build time. docker-compose
# passes it in automatically; override it here if you deploy standalone.
ARG VITE_API_URL=http://localhost:8080
ENV VITE_API_URL=$VITE_API_URL
RUN npm run build

# --- Run stage ---
# Serves the built static files with a tiny web server.
FROM nginx:alpine
COPY --from=build /app/dist /usr/share/nginx/html
EXPOSE 80
CMD ["nginx", "-g", "daemon off;"]
