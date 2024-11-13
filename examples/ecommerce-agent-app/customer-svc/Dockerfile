FROM node:18-alpine

WORKDIR /app

COPY package*.json ./
RUN npm cache clean --force && \
    npm install

COPY . .

ENV NODE_ENV=production
ENV PORT=3200

CMD ["npm", "start"]
