.PHONY: start stop build

# 构建并后台运行
start: build
	@echo "Starting standx-liquidity-market-maker..."
	nohup ./standx-liquidity-market-maker > main.log 2>&1 &
	@echo "Started. PID: $$(ps -ef | grep '[s]tandx-liquidity-market-maker' | awk '{print $$2}')"
	@echo "Logs: tail -f main.log"

# 停止运行
stop:
	@echo "Stopping standx-liquidity-market-maker..."
	@pid=$$(ps -ef | grep '[s]tandx-liquidity-market-maker' | awk '{print $$2}'); \
	if [ -n "$$pid" ]; then \
		kill -9 $$pid; \
		echo "Killed PID: $$pid"; \
	else \
		echo "No running process found"; \
	fi

# 仅构建
build:
	@echo "Building standx-liquidity-market-maker..."
	go build -o standx-liquidity-market-maker ./cmd/main.go
