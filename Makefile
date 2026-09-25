# Data Insights Makefile

# 颜色定义
GREEN := \033[0;32m
YELLOW := \033[0;33m
BLUE := \033[0;34m
NC := \033[0m # No Color

.PHONY: help install dev dev-frontend dev-backend build build-frontend build-backend api-gen serve docker-build docker-up docker-down docker-logs clean

# 默认目标
help:
	@echo "$(BLUE)Data Insights 开发命令$(NC)"
	@echo ""
	@echo "$(GREEN)安装依赖:$NC"
	@echo "  make install          安装前后端依赖 (pnpm)"
	@echo "  make install-frontend 安装前端依赖"
	@echo "  make install-backend  安装后端依赖"
	@echo ""
	@echo "$(GREEN)开发运行:$NC"
	@echo "  make dev              启动前后端开发服务器"
	@echo "  make dev-frontend     启动前端开发服务器"
	@echo "  make dev-backend      启动后端开发服务器"
	@echo ""
	@echo "$(GREEN)构建:$NC"
	@echo "  make build            构建前后端"
	@echo "  make build-frontend   构建前端"
	@echo "  make build-backend    构建后端"
	@echo ""
	@echo "$(GREEN)API 契约:$NC"
	@echo "  make api-gen          从 api/openapi.yaml 生成双端类型 (Go + TS)"
	@echo ""
	@echo "$(GREEN)Docker:$NC"
	@echo "  make docker-build     构建单镜像（前端+后端编译进同一个镜像）"
	@echo "  make docker-up        启动 Docker 容器"
	@echo "  make docker-down      停止 Docker 容器"
	@echo "  make docker-logs      查看 Docker 日志"
	@echo ""
	@echo "$(GREEN)本地验证单进程形态:$NC"
	@echo "  make serve            构建前端并让后端一并托管，单端口 23352（不用 Docker）"
	@echo ""
	@echo "$(GREEN)清理:$NC"
	@echo "  make clean            清理构建产物"

# 安装所有依赖
install: install-frontend install-backend

# 安装前端依赖
install-frontend:
	@echo "$(YELLOW)安装前端依赖...$(NC)"
	cd frontend && pnpm install

# 安装后端依赖
install-backend:
	@echo "$(YELLOW)安装后端依赖...$(NC)"
	cd backend && go mod download

# air 启动后端热重载（dev 与 dev-backend 共用）。
#
# ⚠️ 产物路径必须与 `build-backend`（`backend/bin/server`）**一致**：历史上这里写的是
# `./server`，于是项目里并存两个后端二进制（`backend/server` 与 `backend/bin/server`）。
# 一旦有人手工起了 bin/server，air 重建出的 ./server 就会因端口被占起不来，端口上跑的
# 永远是旧产物 —— 症状是"改了代码不生效"，且只往 tmp/build-errors.log 刷 exit status 1，
# 极难察觉（浏览器验收 round1 / round2 都因此白跑过）。
# 统一用 `bin/`：它已被 .gitignore 覆盖，且 `make clean` 会清理。
AIR_CMD = air --build.cmd "mkdir -p bin && go build -o bin/server ./cmd" --build.entrypoint "./bin/server"

# 开发模式运行：前后端并行拉起，任一侧退出即整体退出
#
# 先挡一道端口占用检查：air 重建后若绑不上 23352 不会报错退出，而是静默让别人的旧进程
# 继续服务，排查成本极高。这里直接快速失败并指出该 kill 谁。
dev:
	@for port in 23351 23352; do \
		pid=$$(lsof -nP -iTCP:$${port} -sTCP:LISTEN -t 2>/dev/null | head -1); \
		if [ -n "$${pid}" ]; then \
			echo "$(YELLOW)⚠️  端口 $${port} 已被占用（pid=$${pid}），air/pnpm 重建后无法绑定，会静默继续跑旧产物。$(NC)"; \
			echo "$(YELLOW)    先结束它再重试： kill $${pid}$(NC)"; \
			exit 1; \
		fi; \
	done
	@echo "$(GREEN)启动前后端开发服务器，前端: http://localhost:23351，后端: http://localhost:23352$(NC)"
	@bash -c '\
	 (cd backend && $(AIR_CMD)) < /dev/null & be=$$!; \
	 (cd frontend && pnpm dev) < /dev/null & fe=$$!; \
	 cleanup() { kill $$be $$fe 2>/dev/null; wait $$be $$fe 2>/dev/null; exit; }; \
	 trap cleanup INT TERM EXIT; \
	 wait'

# 前端开发服务器
dev-frontend:
	@echo "$(YELLOW)启动前端开发服务器...$(NC)"
	cd frontend && pnpm dev

# 后端开发服务器
dev-backend:
	@echo "$(YELLOW)启动后端开发服务器...$(NC)"
	cd backend && $(AIR_CMD)
# 	cd backend && go run cmd/main.go

# 构建
build: build-frontend build-backend

# 构建前端
build-frontend:
	@echo "$(YELLOW)构建前端...$(NC)"
	cd frontend && pnpm build

# 构建后端
build-backend:
	@echo "$(YELLOW)构建后端...$(NC)"
	cd backend && go build -o bin/server ./cmd

# 从 api/openapi.yaml 生成双端契约类型（backend/internal/idls 与 frontend/src/idls）
api-gen:
	@echo "$(YELLOW)生成后端契约类型 (oapi-codegen)...$(NC)"
	cd backend && go run github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen -config ../api/gen/backend.cfg.yaml ../api/openapi.yaml
	@echo "$(YELLOW)生成前端契约类型 (openapi-typescript)...$(NC)"
	cd frontend && pnpm api:gen

# Docker：单镜像（前端静态产物 + Go 二进制同进程），构建上下文是仓库根目录
docker-build:
	@echo "$(YELLOW)构建 Data Insights 单镜像...$(NC)"
	docker build -t data-insights .

docker-up:
	@echo "$(YELLOW)启动 Docker 容器...$(NC)"
	docker compose up -d --build

docker-down:
	@echo "$(YELLOW)停止 Docker 容器...$(NC)"
	docker compose down

docker-logs:
	docker compose logs -f

# 不用 Docker 也能验证「单进程同时提供 API 与页面」：先构建前端产物，
# 再让后端托管 frontend/dist。端口与镜像一致（23352）。
serve:
	@echo "$(YELLOW)构建前端产物...$(NC)"
	cd frontend && pnpm build
	@echo "$(GREEN)后端托管 frontend/dist，单端口 http://localhost:23352$(NC)"
	cd backend && STATIC_DIR=../frontend/dist go run ./cmd

# 清理
clean:
	@echo "$(YELLOW)清理构建产物...$(NC)"
	rm -rf frontend/dist
	rm -rf frontend/node_modules
	rm -rf backend/bin
