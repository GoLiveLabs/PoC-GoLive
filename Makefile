.PHONY: dev-backend dev-frontend mediamtx-up mediamtx-down fake-camera test check

dev-backend:
	cd backend && go run ./cmd/server

dev-frontend:
	cd frontend && npm start

mediamtx-up:
	docker compose up -d mediamtx postgres

mediamtx-down:
	docker compose down

# Usage: make fake-camera NAME=camera1
fake-camera:
	ffmpeg -re -stream_loop -1 -i sample.mp4 -c copy -f flv rtmp://localhost:1935/$(NAME)

test:
	cd backend && go test ./...

check:
	cd backend && go vet ./... && go test ./...
	cd frontend && npx ng lint || true
	cd frontend && npx ng test --watch=false
