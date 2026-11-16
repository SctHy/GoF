# syntax=docker/dockerfile:1

FROM node:26.12.0-alpine3.24 AS builder

WORKDIR /src

COPY package*.json ./
RUN npm ci

COPY . .
RUN npm run build

FROM nginx:1.30.6-alpine3.24

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /src/dist /usr/share/nginx/html

EXPOSE 80
