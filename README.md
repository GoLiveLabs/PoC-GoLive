# Live Orchestrator — Orquestrador de Câmeras para Live

PoC de um orquestrador de múltiplas câmeras para transmissão ao vivo. Ver
`PLANO_IMPLEMENTACAO.md` para a especificação completa e `DECISIONS.md` para
decisões técnicas tomadas durante a implementação.

## Documentação

- **[docs/COMO_RODAR.md](docs/COMO_RODAR.md)** — guia rápido para subir frontend e backend em desenvolvimento local
- **[COMO_USAR.md](COMO_USAR.md)** — guia passo a passo de utilização (setup, painel, câmeras fake/reais, troubleshooting)
- **[docs/BACKEND.md](docs/BACKEND.md)** — documentação técnica do backend Go (pacotes, contratos, sync loop, testes)
- **[docs/FRONTEND.md](docs/FRONTEND.md)** — documentação técnica do frontend Angular (services, signals, componentes)

## Arquitetura

```
Câmeras → MediaMTX → [Backend Go] → OBS Studio → Plataformas (Twitch/YouTube)
                          ↑↓ REST/WS
                     Painel Angular
```

## Pré-requisitos

- Go 1.22+
- Node.js 18+ e Angular CLI
- Docker + Docker Compose (para o MediaMTX)
- OBS Studio com obs-websocket v5 habilitado (Ferramentas → WebSocket Server Settings)
- ffmpeg (para simular câmeras em dev)
- `make` (opcional — os comandos do `Makefile` também podem ser rodados manualmente,
  copiando o comando correspondente de cada alvo; veja `Makefile` na raiz)

## Setup passo a passo

1. Suba o MediaMTX:
   ```
   make mediamtx-up
   ```
2. Simule câmeras (opcional, requer ffmpeg e um arquivo `sample.mp4` na raiz):
   ```
   make fake-camera NAME=camera1
   make fake-camera NAME=camera2
   ```
3. Abra o OBS Studio com o obs-websocket habilitado (porta padrão `4455`).
4. Suba o backend:
   ```
   make dev-backend
   ```
5. Suba o frontend:
   ```
   make dev-frontend
   ```
6. Acesse `http://localhost:4200`.

## Variáveis de ambiente (backend)

| Variável | Default | Descrição |
|---|---|---|
| `HTTP_ADDR` | `:8080` | endereço do servidor HTTP |
| `OBS_ADDR` | `localhost:4455` | endereço do obs-websocket |
| `OBS_PASSWORD` | `` | senha do obs-websocket |
| `MEDIAMTX_API_URL` | `http://localhost:9997` | URL da API do MediaMTX |
| `MEDIA_SOURCE_BASE_URL` | `rtmp://localhost:1935` | URL base usada para montar o `sourceUrl` de cada câmera |
| `API_TOKEN` | `dev-token` | token exigido no header `X-Api-Token` |
| `SYNC_INTERVAL` | `3s` | intervalo do loop de sincronização |
| `PROGRAM_SCENE` | `Program` | nome da cena de programa no OBS |
| `LOG_LEVEL` | `info` | nível de log (`debug`, `info`, `warn`, `error`) |

Ver `.env.example`.

## Comandos úteis (Makefile)

- `make mediamtx-up` / `make mediamtx-down` — sobe/derruba o MediaMTX via docker-compose.
- `make fake-camera NAME=camera1` — envia um vídeo em loop como câmera fake via RTMP.
- `make dev-backend` — roda o backend Go.
- `make dev-frontend` — roda o frontend Angular.
- `make test` — roda os testes do backend.
- `make check` — `go vet` + testes do backend + lint/testes do frontend.

## Roteiro de teste manual — API REST (curl)

Com o backend rodando (`make dev-backend`) e o token padrão `dev-token`:

```bash
curl http://localhost:8080/api/v1/health

curl -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/cameras

curl -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/status

curl -X POST -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/sync

curl -X POST -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/cameras/camera1/live
```

## Roteiro de teste manual — WebSocket

Com [`websocat`](https://github.com/vi/websocat) instalado:

```bash
websocat "ws://localhost:8080/api/v1/ws" -H "X-Api-Token: dev-token"
```

Deve receber imediatamente um evento `cameras.updated` e um `system.status` (snapshot
inicial), e depois eventos conforme câmeras aparecem/somem ou a câmera ao vivo muda.

## Checklist manual — Wrapper OBS (FASE 2)

1. Abrir o OBS Studio com o obs-websocket habilitado.
2. Rodar o backend → verificar nos logs que a cena `Program` foi criada (ou já existia).
3. Simular uma câmera fake (`make fake-camera NAME=camera1`) → verificar que um input
   `cam_camera1` aparece na cena `Program`.
4. Chamar `POST /api/v1/cameras/camera1/live` → verificar que a fonte fica visível.
5. Fechar o OBS → verificar nos logs que o backend detecta a desconexão e tenta
   reconectar com backoff exponencial (1s, 2s, 4s...).
6. Reabrir o OBS → verificar nos logs que o backend reconecta sozinho.

## Limitações conhecidas / próximos passos

- Sem autenticação de usuários (apenas token estático via header).
- Sem persistência (estado em memória, perdido ao reiniciar o backend).
- Sem preview de vídeo no painel (espaço reservado nos cards para fase futura via HLS).
- Sem suporte a múltiplas cenas/layouts — apenas uma cena de programa fixa.
- Deploy apenas local (docker-compose cobre somente o MediaMTX).
