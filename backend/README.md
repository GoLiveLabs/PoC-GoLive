# golive-backend

Backend Go do Live Orchestrator — orquestrador central de câmeras para
transmissões ao vivo. Mantém sincronizado o estado das câmeras conhecidas
(via MediaMTX) com inputs no OBS Studio, expõe uma API REST/WebSocket para o
painel web e permite ao operador selecionar qual câmera está "ao vivo".

> Este repositório é **independente**. Ele contém apenas o código e o histórico
> do backend. Para executar a stack completa (backend + frontend + MediaMTX +
> OBS), consulte o repositório de infraestrutura
> [`GoLiveLabs/PoC-GoLive`](https://github.com/GoLiveLabs/PoC-GoLive) e use
> o `docker-compose.yml` publicado lá como matriz de compatibilidade entre as
> imagens `ghcr.io/golivelabs/golive-backend` e `ghcr.io/golivelabs/golive-frontend`.
> Não há nenhum subdiretório `backend/` aqui, e não é necessário clonar nenhum
> outro repositório para construir, testar ou executar apenas este serviço.

## Pré-requisitos

- Go 1.26+
- Docker (opcional, apenas para construir/rodar a imagem publicada)
- MediaMTX acessível (para conexão com câmeras reais em tempo de execução)
- OBS Studio com `obs-websocket` v5 habilitado (Ferramentas → WebSocket Server
  Settings)

## Executando localmente

A partir da raiz deste repositório:

```bash
go run ./cmd/server
```

O servidor ficará disponível em `http://localhost:8080` por padrão. A
autenticação da API usa o token padrão `dev-token` no header `X-Api-Token`
(ver `internal/config/config.go` para todas as variáveis de ambiente aceitas).

## Testes

```bash
go vet ./...
go test ./...
```

A suíte de testes é a mesma que rodava dentro do monorepo original e deve
passar de forma idêntica neste repositório isolado.

## Build e publicação

A imagem Docker é publicada automaticamente pelo workflow
[`.github/workflows/release.yml`](.github/workflows/release.yml) quando uma
tag `vMAJOR.MINOR.PATCH` é empurrada para este repositório. Para construir a
imagem localmente (sem depender do pipeline):

```bash
docker build -t ghcr.io/golivelabs/golive-backend:dev .
```

A imagem produzida expõe a porta `8080` e usa `ENTRYPOINT ["/app/server"]`.

### Tags e releases

- As releases seguem [SemVer](https://semver.org/) a partir de `v0.1.0`.
- O pipeline de release é acionado apenas por tags que casam com
  `^v[0-9]+\.[0-9]+\.[0-9]+$`. Tags malformadas (ex.: `v0.1`) não disparam
  build/publicação.
- O workflow de CI
  [`.github/workflows/ci.yml`](.github/workflows/ci.yml) roda `go vet`/`go test`
  em pushes e PRs contra `main`, sem build/publicação.

## Documentação interna

- [`docs/ARQUITETURA.md`](docs/ARQUITETURA.md) — pacotes, princípios de design e responsabilidades
- [`docs/COMUNICACAO.md`](docs/COMUNICACAO.md) — fluxos de comunicação entre backend, MediaMTX, OBS e painel
- [`docs/SERVICOS.md`](docs/SERVICOS.md) — descrição por serviço com interfaces públicas

## Estrutura

```
cmd/server/          ponto de entrada (main.go)
internal/            pacotes internos (config, httpapi, orchestrator, ...)
docs/                documentação do backend
.github/workflows/   ci.yml e release.yml
Dockerfile           build multi-estágio (golang:1.26-alpine -> alpine:3.20)
go.mod / go.sum      módulo Go: live-orchestrator/backend
```