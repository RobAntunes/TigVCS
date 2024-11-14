# Development commands
.PHONY: dev build test clean

# Start development environment
dev:
	docker-compose up --build

# Build all services
build:
	docker-compose build

# Run tests
test:
	docker-compose run tig go test ./...

# Clean up containers and volumes
clean:
	docker-compose down -v

# View logs
logs:
	docker-compose logs -f

# Shell into tig container
shell:
	docker-compose exec tig sh

# Database management
db-init:
	docker-compose exec cockroach cockroach sql --insecure -e "CREATE DATABASE IF NOT EXISTS tig;"

# Stop all containers
stop:
	docker-compose down