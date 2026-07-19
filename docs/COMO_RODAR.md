# Como rodar frontend e backend

Este guia mostra como subir os dois ambientes do projeto em desenvolvimento local.

## Pré-requisitos

- Go instalado no sistema
- Node.js e npm instalados
- Dependências do frontend instaladas com `npm install` dentro da pasta `frontend/`
- Dependências do backend disponíveis via `go run`/`go test` dentro da pasta `backend/`

## Backend

1. Abra um terminal na raiz do repositório.
2. Suba o backend com:

```bash
make dev-backend
```

O backend fica disponível em `http://localhost:8080` por padrão.

Se preferir rodar sem `make`, use:

```bash
cd backend
go run ./cmd/server
```

## Frontend

1. Abra outro terminal na raiz do repositório.
2. Suba o frontend com:

```bash
make dev-frontend
```

O frontend fica disponível em `http://localhost:4200` por padrão.

Se preferir rodar sem `make`, use:

```bash
cd frontend
npm start
```

## Rodando os dois juntos

Para desenvolvimento normal, mantenha dois terminais abertos:

1. Terminal 1: `make dev-backend`
2. Terminal 2: `make dev-frontend`

Depois acesse `http://localhost:4200` no navegador.

## Observações úteis

- O backend usa o token padrão `dev-token` no header `X-Api-Token`.
- Se a API do backend ou o painel não responderem, verifique se o processo ainda está ativo em cada terminal.
- Para parar os serviços, use `Ctrl+C` em cada terminal.