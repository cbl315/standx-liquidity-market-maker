.PHONY: start stop build build-for-linux start-for-linux stop-for-linux stop-all stop-all-for-linux

# 构建并后台运行
start: build
	@echo "Starting standx-liquidity-market-maker..."
	@bash -c 'set -a && source .env && set +a && nohup ./standx-liquidity-market-maker > main.log 2>&1 & echo $$! > main.pid'
	@echo "Started. PID: $$(cat main.pid)"
	@echo "Logs: tail -f main.log"

# 停止运行
stop:
	@echo "Stopping standx-liquidity-market-maker..."
	@if [ -f main.pid ]; then \
		pkill -P $$(cat main.pid) 2>/dev/null; \
		kill $$(cat main.pid) 2>/dev/null && echo "Killed PID: $$(cat main.pid)" || echo "Process already stopped"; \
		rm -f main.pid; \
	else \
		echo "No PID file found, process may not be running"; \
	fi

# 仅构建
build:
	@echo "Building standx-liquidity-market-maker..."
	go build -o standx-liquidity-market-maker ./cmd/main.go

# 交叉编译 Linux 二进制文件
build-for-linux:
	@echo "Building standx-liquidity-market-maker for Linux..."
	GOOS=linux GOARCH=amd64 go build -o standx-liquidity-market-maker-linux ./cmd/main.go
	@echo "Built binary: standx-liquidity-market-maker-linux"

# 启动 Linux 版本
start-for-linux:
	@echo "Starting standx-liquidity-market-maker-linux..."
	@bash -c 'set -a && source .env && set +a && nohup ./standx-liquidity-market-maker-linux > main.log 2>&1 & echo $$! > main-linux.pid'
	@echo "Started. PID: $$(cat main-linux.pid)"
	@echo "Logs: tail -f main.log"

# 停止 Linux 版本
stop-for-linux:
	@echo "Stopping standx-liquidity-market-maker-linux..."
	@if [ -f main-linux.pid ]; then \
		pkill -P $$(cat main-linux.pid) 2>/dev/null; \
		kill $$(cat main-linux.pid) 2>/dev/null && echo "Killed PID: $$(cat main-linux.pid)" || echo "Process already stopped"; \
		rm -f main-linux.pid; \
	else \
		echo "No PID file found, process may not be running"; \
	fi

# 停止所有本地版本进程
stop-all:
	@echo "Stopping all standx-liquidity-market-maker processes..."
	@pids=$$(ps -ef | grep '[s]tandx-liquidity-market-maker' | grep -v 'standx-liquidity-market-maker-linux' | awk '{print $$2}'); \
	if [ -n "$$pids" ]; then \
		echo "Killing PIDs: $$pids"; \
		kill -9 $$pids; \
		rm -f main.pid; \
	else \
		echo "No running processes found"; \
	fi

# 停止所有 Linux 版本进程
stop-all-for-linux:
	@echo "Stopping all standx-liquidity-market-maker-linux processes..."
	@pids=$$(ps -ef | grep '[s]tandx-liquidity-market-maker-linux' | awk '{print $$2}'); \
	if [ -n "$$pids" ]; then \
		echo "Killing PIDs: $$pids"; \
		kill -9 $$pids; \
		rm -f main-linux.pid; \
	else \
		echo "No running processes found"; \
	fi
