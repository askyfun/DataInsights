# 前端生产镜像：构建静态产物 + nginx 托管（此前 CMD 跑的是 `npm run dev`，
# 拿 Vite 开发服务器当生产服务，既没有构建产物也没有反向代理）。

# ---- 构建阶段 ----
FROM node:24-alpine AS builder

WORKDIR /app

RUN corepack enable

COPY package.json pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile

COPY . .

# 留空字符串 = 前端只发同源相对路径 /api，由运行阶段的 nginx 反代到 backend:23352。
# 不要把后端地址烧进镜像：生产对外入口只有 23351 一个。
ARG VITE_API_BASE_URL=""
ENV VITE_API_BASE_URL=${VITE_API_BASE_URL}

# pnpm build = tsc + vite build，类型错误会让镜像构建失败
RUN pnpm build

# ---- 运行阶段 ----
FROM nginx:1.27-alpine

COPY nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=builder /app/dist /usr/share/nginx/html

EXPOSE 23351

CMD ["nginx", "-g", "daemon off;"]
