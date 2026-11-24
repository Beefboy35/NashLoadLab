.PHONY: dev backend frontend test build docker-up docker-down docker-logs

dev:
	powershell -ExecutionPolicy Bypass -File ./scripts/dev.ps1

backend:
	cd backend && go run ./cmd/server

frontend:
	cd frontend && npm run dev

test:
	cd backend && go test ./...
	cd frontend && npm test

build:
	cd backend && go build ./cmd/server
	cd frontend && npm run build

docker-up:
	docker-compose up --build

docker-down:
	docker-compose down

docker-logs:
	docker-compose logs -f
