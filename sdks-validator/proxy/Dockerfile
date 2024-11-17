FROM node:18-alpine

WORKDIR /app

# Install dependencies first (better layer caching)
COPY package*.json ./
RUN npm clean-install --production

COPY package*.json ./
RUN npm cache clean && npm install

# Copy source code
COPY src/ ./src/

# Runtime configuration
ENV NODE_ENV=production
EXPOSE 8006

CMD ["npm", "start"]
