# Arquitetura — Backend (Go)

## Visão Geral

O backend é um servidor Go que funciona como orquestrador central. Mantém sincronizado o estado de câmeras conhecidas (via MediaMTX) com inputs no OBS Studio, expõe uma API REST/WebSocket para o painel web, e permite ao operador selecionar qual câmera está "ao vivo" em qualquer momento.

## Princípios de Design

1. **Sincronização periódica** — a cada N segundos, query MediaMTX para descobrir câmeras novas/perdidas
2. **Reatividade** — mudanças de estado propagam imediatamente via WebSocket
3. **Resiliência** — reconexão automática com OBS e MediaMTX com backoff exponencial
4. **Sem persistência** — estado em memória (loss on shutdown)
5. **Token estático** — autenticação simplificada via header `X-Api-Token`

## Pacotes e Responsabilidades

### `config` — Configuração

Carrega variáveis de ambiente e as fornece como struct:

```go
type Config struct {
  HTTPAddr              string        // :8080
  OBSAddr               string        // localhost:4455
  OBSPassword           string
  MediaMTXAPIURL        string        // http://localhost:9997
  MediaSourceBaseURL    string        // rtmp://localhost:1935
  APIToken              string        // dev-token
  SyncInterval          time.Duration // 3s
  ProgramScene          string        // Program
  LogLevel              string        // info
}
```

**Arquivo**: `internal/config/config.go`

---

### `httpapi` — API HTTP + WebSocket

Expõe endpoints REST e gerencia conexões WebSocket.

#### Rotas

| Método | Caminho | Descrição |
|--------|---------|-----------|
| GET | `/api/v1/health` | Health check (sem autenticação) |
| GET | `/api/v1/cameras` | Lista todas as câmeras |
| GET | `/api/v1/status` | Status atual do sistema |
| POST | `/api/v1/sync` | Força sincronização imediata |
| POST | `/api/v1/cameras/{id}/live` | Seleciona câmera como ao vivo |
| WS | `/api/v1/ws` | WebSocket para eventos em tempo real |

#### Middleware

- **Token** — Valida `X-Api-Token` header (exceto `/health`)

#### Arquivos

- `httpapi.go` — Servidor, rotas, handlers REST
- `middleware.go` — Middleware de autenticação
- `ws.go` — Handler e lógica de WebSocket

---

### `mediaserver` — Cliente MediaMTX

Comunica-se com a API HTTP do MediaMTX para descobrir streams ativos.

```go
type Client struct {
  baseURL string
}

func (c *Client) ListActiveStreams(ctx context.Context) ([]StreamInfo, error)
```

Retorna lista de streams no formato:
```go
type StreamInfo struct {
  Name string
}
```

**Arquivo**: `internal/mediaserver/client.go`

---

### `obs` — Cliente OBS Studio

Comunica-se com OBS Studio via WebSocket v5 para gerenciar inputs e cenas.

#### Responsabilidades

- Conectar/reconectar com backoff exponencial
- Criar/deletar inputs de câmera (naming: `cam_<camera_id>`)
- Gerenciar cena "Program" (criar se não existir)
- Tornar source visível/invisível (set item's visible state)

#### Métodos Públicos

```go
func (c *Controller) CreateInput(name, sourceUrl string) error
func (c *Controller) DeleteInput(name string) error
func (c *Controller) SetItemVisible(sceneName, sourceName string, visible bool) error
func (c *Controller) IsConnected() bool
func (c *Controller) Close()
```

**Arquivos**: 
- `obs.go` — Controller principal
- `obsmock/obsmock.go` — Mock para testes

---

### `events` — Event Hub (Pub/Sub)

Permite publicar eventos que serão entregues a todos os subscribers (conexões WebSocket).

```go
type Hub struct {
  // ...
}

func (h *Hub) Publish(eventType string, payload interface{})
func (h *Hub) Subscribe() (<-chan Event, func())
```

**Arquivo**: `internal/events/hub.go`

---

### `orchestrator` — Core da Lógica

O coração da aplicação. Sincroniza câmeras e gerencia estado.

#### Modelos (Contratos)

```go
type Camera struct {
  ID               string
  Name             string
  SourceURL        string
  Status           string  // "online" | "offline"
  ObsSourceCreated bool
  IsLive           bool
  LastSeenAt       time.Time
}

type SystemStatus struct {
  ObsConnected         bool
  MediaServerConnected bool
  Streaming            bool
  ActiveSceneName      string
  LiveCameraID         string
}
```

#### Métodos Públicos

```go
func (o *Orchestrator) Cameras() []Camera
func (o *Orchestrator) Status() SystemStatus
func (o *Orchestrator) SetLive(cameraID string) (SystemStatus, error)
func (o *Orchestrator) SyncOnce(ctx context.Context) []Camera
func (o *Orchestrator) Run(ctx context.Context)  // Sync loop
```

#### Fluxo Interno — `Run()`

```go
func (o *Orchestrator) Run(ctx context.Context) {
  ticker := time.NewTicker(o.syncInterval)
  for {
    select {
    case <-ticker.C:
      o.doSync()
    case <-ctx.Done():
      return
    }
  }
}

func (o *Orchestrator) doSync() {
  1. Query MediaMTX: ctx.ListActiveStreams()
  2. Compare com cameras in-memory
  3. Detecta:
     - Streams novos → criar Camera + OBS input
     - Streams desaparecidos → marcar offline, contar tempo
     - Offline por 60s → deletar OBS input
  4. Emitir eventos para subscribers
}
```

#### Fluxo — `SetLive()`

```go
func (o *Orchestrator) SetLive(cameraID string) {
  1. Validar se câmera existe e está online
  2. Iterar todas câmeras:
     - Se era live: SetItemVisible(false)
     - Se é a selecionada: SetItemVisible(true)
  3. Atualizar state.LiveCameraID
  4. Emitir "system.status"
}
```

**Arquivo**: `internal/orchestrator/orchestrator.go`

---

## Modelo de Concorrência

### Goroutines

1. **Main goroutine** — Aguarda signals (SIGINT, SIGTERM)
2. **Orchestrator.Run()** — Sync loop em background
3. **HTTP server** — Aguarda requisições (goroutine por conexão)
4. **WebSocket handlers** — Uma goroutine por conexão ativa

### Sincronização

- `sync.Mutex` em `Server.conns` (para gerenciar WebSocket connections)
- `sync.Mutex` em `Orchestrator.mu` (para proteger camera state)
- `sync.Mutex` em `events.Hub.mu` (para pub/sub)

### Nota sobre Shutdown

```go
// main.go
orchCancel()           // Stop sync loop
apiServer.CloseAllWS() // Close all WebSocket connections
httpServer.Shutdown()  // Graceful shutdown do servidor HTTP
obsCtl.Close()         // Desconectar do OBS
```

---

## Tratamento de Erros e Resiliência

### MediaMTX

- Se API não responder: erro é logado, câmeras permanecem no estado anterior
- Próxima sincronização tenta novamente

### OBS

- Desconexão é detectada na tentativa de write/read
- Retry com exponential backoff: 1s, 2s, 4s, 8s...
- Ao reconectar: re-cria cena e inputs

### WebSocket

- Cliente fecha conexão → é removido do mapa
- Erro ao escrever para cliente → conexão é fechada
- Cliente reconecta → nova conexão WebSocket

---

## Testes

### Estrutura

- `*_test.go` ao lado de cada pacote
- Usa `obsmock.Mock` para mockar OBS
- Usa table-driven tests

### Executar Testes

```bash
go test ./...              # Todos
go test ./internal/...     # Apenas internals
go test -v ./internal/orchestrator  # Com verbose
```

---

## Logging

Usa `log/slog` com níveis: debug, info, warn, error.

**Configuração** via `LOG_LEVEL` env var (default: `info`).

### Exemplos

```go
slog.Info("camera found", "id", "camera1", "status", "online")
slog.Warn("obs connection lost", "attempt", 3)
slog.Error("failed to set live", "error", err)
```

---

## Extensibilidade

### Adicionar Nova Câmera — Fluxo Interno

1. Câmera envia RTMP para MediaMTX
2. Sync loop → `ListActiveStreams()` retorna stream novo
3. Detecta: `if !o.cameras[streamName]`
4. Cria `Camera` struct
5. `obs.CreateInput("cam_" + streamName, rtmpUrl)`
6. Emite `cameras.updated`

### Adicionar Novo Endpoint

1. Adicionar handler em `httpapi.go`
2. Registrar rota em `routes()`
3. Middleware de autenticação se necessário
4. Retornar JSON com status apropriado

### Adicionar Nova Dependência Externa

1. Criar novo pacote em `internal/`
2. Definir interface (se necessário para testes/mock)
3. Integrar no `main.go`
4. Passar via DI para quem precisa
