FROM node:18-slim

WORKDIR /app

# Install dependencies first (better layer caching)
COPY package*.json ./
RUN npm clean-install --production

# Copy source code
COPY src/ ./src/

# Runtime configuration
ENV NODE_ENV=production
EXPOSE 8006

CMD ["npm", "start"]
