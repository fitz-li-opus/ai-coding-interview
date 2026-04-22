.PHONY: help up down restart ps logs logs-gateway test test-race smoke k6

help:
	@echo "Available commands:"
	@echo "  make up           - Build and start all services"
	@echo "  make down         - Stop all services"
	@echo "  make restart      - Restart all services"
	@echo "  make ps           - Show service status"
	@echo "  make logs         - Show all compose logs"
	@echo "  make logs-gateway - Show gateway logs"
	@echo "  make test         - Run go test ./..."
	@echo "  make test-race    - Run go test -race ./..."
	@echo "  make smoke        - Run curl-based connectivity checks"
	@echo "  make k6           - Run k6 load test in Docker"

up:
	docker compose up --build -d

down:
	docker compose down

restart:
	docker compose down
	docker compose up --build -d

ps:
	docker compose ps

logs:
	docker compose logs -f

logs-gateway:
	docker compose logs -f gateway

test:
	cd gateway && go test ./...

test-race:
	cd gateway && go test -race ./...

smoke:
	curl -i http://localhost:8080/healthz
	@printf '\n---\n'
	curl -i http://localhost:8080/readyz
	@printf '\n---\n'
	curl http://localhost:8080/api/users/
	@printf '\n---\n'
	curl http://localhost:8080/api/orders/
	@printf '\n---\n'
	curl http://localhost:8080/api/products/
	@printf '\n'

k6:
	docker compose --profile tools run --rm \
		-e K6_VUS=$(if $(K6_VUS),$(K6_VUS),20) \
		-e K6_DURATION=$(if $(K6_DURATION),$(K6_DURATION),30s) \
		k6
