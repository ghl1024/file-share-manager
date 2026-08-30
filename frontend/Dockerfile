# ==========================================
# 阶段 1: 构建 Vue 前端项目
# ==========================================
ARG NODE_VERSION=22
ARG VERSION=v0.0.0
ARG GIT_COMMIT=none
ARG BUILD_TIME=unknown
FROM --platform=$BUILDPLATFORM node:${NODE_VERSION}-alpine AS builder
ARG VERSION
ARG GIT_COMMIT
ARG BUILD_TIME
ENV VITE_APP_VERSION=${VERSION}
ENV VITE_GIT_COMMIT=${GIT_COMMIT}
ENV VITE_BUILD_TIME=${BUILD_TIME}

WORKDIR /app

# 利用 Docker 层缓存，先安装依赖
COPY package*.json ./
RUN npm ci

# 拷贝源码并构建生产版本
COPY . .
RUN npm run build:prod

# ==========================================
# 阶段 2: Nginx 提供服务
# ==========================================
FROM nginx:alpine
ARG VERSION
ARG GIT_COMMIT
ARG BUILD_TIME
ENV FILESHARE_VERSION=${VERSION}
ENV FILESHARE_GIT_COMMIT=${GIT_COMMIT}
ENV FILESHARE_BUILD_TIME=${BUILD_TIME}

# 拷贝自定义 Nginx 配置
COPY nginx.conf /etc/nginx/conf.d/default.conf

# 拷贝构建好的前端静态文件
COPY --from=builder /app/dist /usr/share/nginx/html
COPY docker-entrypoint.sh /docker-entrypoint.sh
RUN chmod +x /docker-entrypoint.sh

EXPOSE 80

# 启动 Nginx
CMD ["/docker-entrypoint.sh"]
