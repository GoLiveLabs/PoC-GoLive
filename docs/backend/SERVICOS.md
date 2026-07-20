# Serviços do Backend

Descrição detalhada de cada serviço, suas responsabilidades, interfaces e como se integram ao sistema.

---

## 1. Config Service

**Arquivo**: `internal/config/config.go`

### Responsabilidade

Carregar e validar variáveis de ambiente, centralizando todas as configurações que o sistema precisa.

### Interface Pública

```go
type Config struct {
  HTTPAddr             string
  OBSAddr              string
  OBSPassword          string
  MediaMTXAPIURL       string
  MediaSourceBaseURL   string
  APIToken             string
  SyncInterval         time.Duration
  ProgramScene         string
  LogLevel             string
}

func Load() Config
```

### Variáveis de Ambiente

| Variável | Default | Tipo | Descrição |
|----------|---------|------|-----------|
| `HTTP_ADDR` | `:8080` | string | Endereço para ouvir (host:port) |
| `OBS_ADDR` | `localhost:4455` | string | Endereço do OBS Studio (host:port) |
| `OBS_PASSWORD` | `` | string | Senha do obs-websocket v5 |
| `MEDIAMTX_API_URL` | `http://localhost:9997` | string | URL base da API MediaMTX |
| `MEDIA_SOURCE_BASE_URL` | `rtmp://localhost:1935` | string | URL base para montar sourceUrl |
| `API_TOKEN` | `dev-token` | string | Token exigido no header `X-Api-Token` |
| `SYNC_INTERVAL` | `3s` | duration | Intervalo entre sincronizações |
| `PROGRAM_SCENE` | `Program` | string | Nome da cena de programa no OBS |
| `LOG_LEVEL` | `info` | string | Nível de log (debug/info/warn/error) |

### Exemplo de Uso

```go
cfg := config.Load()
// cfg.HTTPAddr = ":8080"
// cfg.OBSAddr = "localhost:4455"
// etc.
```

---

## 2. MediaMTX Client

**Arquivo**: `internal/mediaserver/client.go`

### Responsabilidade

Comunicar-se com a API HTTP do MediaMTX para descobrir streams de vídeo ativos.

### Interface Pública

```go
type Client struct {
  baseURL string
}

func NewClient(baseURL string) *Client

type StreamInfo struct {
  Name string // Nome do stream (ex: "camera1")
}

func (c *Client) ListActiveStreams(ctx context.Context) ([]StreamInfo, error)
```

### Fluxo de Chamada

1. **Configuração** → `NewClient(cfg.MediaMTXAPIURL)`
2. **Query periódica** → Sync loop chama `ListActiveStreams()` a cada intervalo
3. **Retorno** → Lista de `StreamInfo` (apenas o nome é importante para o orquestrador)

### Exemplo HTTP

```bash
GET http://localhost:9997/v3/list
```

Retorna JSON com lista de streams ativos.

### Tratamento de Erro

Se MediaMTX estiver offline:
- `ListActiveStreams()` retorna erro
- Sync loop o loga
- Estado anterior é mantido
- Próxima tentativa ocorre na próxima iteração

### Testes

**Arquivo**: `internal/mediaserver/client_test.go`

- Mock HTTP server para testar parsing
- Testes de erro (timeout, 5xx, etc.)

---

## 3. OBS Controller

**Arquivo**: `internal/obs/obs.go`

### Responsabilidade

Comunicar-se com OBS Studio via WebSocket v5 para:
- Criar/deletar inputs de câmera
- Gerenciar cena "Program"
- Tornar fontes visíveis/invisíveis

### Interface Pública

```go
type Controller interface {
  CreateInput(name, sourceUrl string) error
  DeleteInput(name string) error
  SetItemVisible(sceneName, sourceName string, visible bool) error
  IsConnected() bool
  Close()
}

func New(addr, password string) Controller
```

### Fluxo de Conexão

```
1. New() → Tenta conectar imediatamente
2. Se falhar → Inicia retry loop com exponential backoff
3. Em background → Aguarda mensagens RPC de OBS
4. Ao reconectar → Re-cria cena e inputs
```

### Operações Suportadas

#### CreateInput
```go
obs.CreateInput("cam_camera1", "rtmp://localhost:1935/live/camera1")
```
- Cria input RTMP com o nome especificado
- Adiciona o input à cena "Program"
- Retorna erro se input já existe ou se desconectado

#### DeleteInput
```go
obs.DeleteInput("cam_camera1")
```
- Remove input da cena
- Retorna erro silenciosamente se não existe

#### SetItemVisible
```go
obs.SetItemVisible("Program", "cam_camera1", true)  // Torna visível
obs.SetItemVisible("Program", "cam_camera1", false) // Torna invisível
```
- Controla se o item é renderizado na cena
- Usado para determinar qual câmera está "ao vivo"

#### IsConnected
```go
if obs.IsConnected() {
  // OBS está conectado
}
```
- Verifica estado da conexão
- Não garante que próxima operação funcionará (pode desconectar)

### Reconexão Automática

```
Desconexão detectada
  ↓
Retry loop inicia:
  Wait 1s
  Try connect → falha
  Wait 2s
  Try connect → falha
  Wait 4s
  Try connect → falha
  Wait 8s
  Try connect → sucesso!
  ↓
Re-cria cena "Program" (se não existir)
Re-cria todos os inputs conhecidos
Emite log "obs reconnected"
```

### Mock para Testes

**Arquivo**: `internal/obs/obsmock/obsmock.go`

```go
type Mock struct {
  // Rastreia estado para assertions
  CreatedInputs map[string]string  // nome → sourceUrl
  DeletedInputs  map[string]bool
  VisibleItems   map[string]bool
}

// Implementa interface Controller
```

### Testes

**Arquivo**: `internal/obs/obs_test.go` (não existe ainda, mas pode ser adicionado)

- Usar `obsmock.Mock` para testar orchestrator
- Mock retorna erros controlados para testar retry logic

---

## 4. Events Hub

**Arquivo**: `internal/events/hub.go`

### Responsabilidade

Implementar padrão pub/sub para notificar múltiplas conexões WebSocket de mudanças de estado.

### Interface Pública

```go
type Event struct {
  Type    string
  Payload interface{}
}

type Hub struct {
  // ...
}

func NewHub() *Hub

func (h *Hub) Publish(eventType string, payload interface{})

func (h *Hub) Subscribe() (<-chan Event, func())
  // Retorna canal de leitura + função de cancelamento
```

### Fluxo Típico

```go
// Subscriber (conexão WebSocket)
events, unsubscribe := hub.Subscribe()
defer unsubscribe()

for event := range events {
  switch event.Type {
  case "cameras.updated":
    // Escrever para WebSocket client
  case "system.status":
    // Escrever para WebSocket client
  }
}

// Publisher (Orchestrator)
hub.Publish("cameras.updated", []Camera{...})
hub.Publish("system.status", SystemStatus{...})
```

### Implementação

- Goroutine por subscriber
- Canal buffered para não bloquear publisher
- Unsubscribe() fecha subscriber

---

## 5. Orchestrator

**Arquivo**: `internal/orchestrator/orchestrator.go`

### Responsabilidade

Core da lógica: sincronizar câmeras entre MediaMTX e OBS, gerenciar estado, permitir seleção de "câmera ao vivo".

### Interface Pública

```go
type Orchestrator struct {
  mediaClient    MediaServerClient
  obsCtl         obs.Controller
  hub            *events.Hub
  programScene   string
  syncInterval   time.Duration
  // ... state interno
}

func New(
  mediaClient MediaServerClient,
  obsCtl obs.Controller,
  hub *events.Hub,
  programScene string,
  syncInterval time.Duration,
  mediaSourceBaseURL string,
) *Orchestrator

func (o *Orchestrator) Cameras() []Camera
func (o *Orchestrator) Status() SystemStatus
func (o *Orchestrator) SetLive(cameraID string) (SystemStatus, error)
func (o *Orchestrator) SyncOnce(ctx context.Context) []Camera
func (o *Orchestrator) Run(ctx context.Context)  // Sync loop
```

### Estado Gerenciado

```go
type Orchestrator struct {
  cameras map[string]*Camera  // id → estado da câmera
  // ...
  mu sync.Mutex  // Protege cameras
  
  liveCamera string  // ID da câmera atualmente ao vivo
  obsConnected bool
  mediaConnected bool
  streaming bool
  activeScene string
}

type Camera struct {
  ID               string
  Name             string
  SourceURL        string
  Status           string  // "online" | "offline"
  ObsSourceCreated bool
  IsLive           bool
  LastSeenAt       time.Time
  missingFor       time.Duration  // Tempo offline (interno)
}
```

### Sync Loop

**Método**: `Run(ctx context.Context)`

```go
func (o *Orchestrator) Run(ctx context.Context) {
  ticker := time.NewTicker(o.syncInterval)
  defer ticker.Stop()

  for {
    select {
    case <-ticker.C:
      o.syncOnce()
    case <-ctx.Done():
      return
    }
  }
}

func (o *Orchestrator) syncOnce() {
  // 1. Query MediaMTX
  streams, err := o.mediaClient.ListActiveStreams(ctx)
  
  // 2. Comparar com estado
  for _, stream := range streams {
    if camera não existe {
      criar camera
      o.obsCtl.CreateInput(...)
      o.hub.Publish("cameras.updated")
    } else if camera estava offline {
      marcar como online
      o.hub.Publish("cameras.updated")
    }
  }
  
  // 3. Detectar câmeras perdidas
  for id, camera := range o.cameras {
    if não está em streams {
      camera.missingFor += o.syncInterval
      if camera.missingFor > offlineRemoveAfter {
        deletar input do OBS
        deletar do estado
        o.hub.Publish("cameras.updated")
      }
    }
  }
  
  // 4. Atualizar status
  o.hub.Publish("system.status", o.Status())
}
```

### SetLive

**Método**: `SetLive(cameraID string) (SystemStatus, error)`

```go
func (o *Orchestrator) SetLive(cameraID string) (SystemStatus, error) {
  o.mu.Lock()
  defer o.mu.Unlock()

  // Validar câmera
  camera, ok := o.cameras[cameraID]
  if !ok {
    return SystemStatus{}, ErrCameraNotFound
  }
  if camera.Status == StatusOffline {
    return SystemStatus{}, ErrCameraOffline
  }

  // Desligar camera anterior (se houver)
  if o.liveCamera != "" && o.liveCamera != cameraID {
    old := o.cameras[o.liveCamera]
    o.obsCtl.SetItemVisible(o.programScene, old.Name, false)
  }

  // Ligar nova câmera
  o.liveCamera = cameraID
  camera.IsLive = true
  obsInputName := "cam_" + cameraID
  o.obsCtl.SetItemVisible(o.programScene, obsInputName, true)

  // Emitir evento
  o.hub.Publish("system.status", o.Status())

  return o.Status(), nil
}
```

### Testes

**Arquivo**: `internal/orchestrator/orchestrator_test.go`

- Table-driven tests para sync loop
- Usa `obsmock.Mock` para simular OBS
- Testes de reconexão, desconexão, etc.

---

## 6. HTTP API Server

**Arquivo**: `internal/httpapi/httpapi.go`

### Responsabilidade

Expor endpoints REST e gerenciar conexões WebSocket.

### Rotas

| Método | Caminho | Descrição | Autenticação |
|--------|---------|-----------|--------------|
| GET | `/api/v1/health` | Health check | Nenhuma |
| GET | `/api/v1/cameras` | Lista câmeras | Token |
| GET | `/api/v1/status` | Status do sistema | Token |
| POST | `/api/v1/sync` | Força sync | Token |
| POST | `/api/v1/cameras/{id}/live` | Set live | Token |
| WS | `/api/v1/ws` | WebSocket | Token |

### Handlers

#### GET /api/v1/health
```go
// Retorna 200 OK, sem body
// Usado para verificar se servidor está up
```

#### GET /api/v1/cameras
```go
// Retorna []Camera
// Exemplo: [{"id":"camera1","status":"online",...}]
```

#### GET /api/v1/status
```go
// Retorna SystemStatus
// Exemplo: {"obsConnected":true,"streaming":true,...}
```

#### POST /api/v1/sync
```go
// Força uma sincronização imediata
// Retorna []Camera após sync
```

#### POST /api/v1/cameras/{id}/live
```go
// Seleciona câmera como ao vivo
// Retorna SystemStatus atualizado ou erro
// Possíveis erros: 404 (câmera não existe), 400 (offline)
```

#### WS /api/v1/ws
```
// Upgrade HTTP → WebSocket
// Client recebe eventos de "cameras.updated" e "system.status"
// Cliente deve enviar ping para manter conexão viva
```

### Middleware

**Arquivo**: `internal/httpapi/middleware.go`

```go
func (s *Server) tokenMiddleware(next http.Handler) http.Handler
  // Valida header X-Api-Token
  // Retorna 401 se inválido ou faltando
  // Passa para next se válido
```

---

## Fluxo de Integração

### Startup

```go
// main.go
cfg := config.Load()
msClient := mediaserver.NewClient(cfg.MediaMTXAPIURL)
obsCtl := obs.New(cfg.OBSAddr, cfg.OBSPassword)
hub := events.NewHub()
orch := orchestrator.New(msClient, obsCtl, hub, ...)

go orch.Run(ctx)  // Inicia sync loop em background

server := httpapi.NewServer(orch, hub, cfg.APIToken)
httpServer := http.Server{Addr: cfg.HTTPAddr, Handler: server.Handler()}
go httpServer.ListenAndServe()

<-ctx.Done()
// Graceful shutdown...
```

### Request Flow (Exemplo: POST /cameras/camera1/live)

```
1. Client → POST /api/v1/cameras/camera1/live (header X-Api-Token)
2. Router → Match route
3. Middleware → Validar token (se inválido, return 401)
4. Handler → orch.SetLive("camera1")
5. Orchestrator:
   - Validar câmera
   - SetItemVisible(false) para câmera anterior
   - SetItemVisible(true) para câmera nova
   - hub.Publish("system.status", ...)
6. Handler → Retornar SystemStatus como JSON (200)
7. Todos WebSocket subscribers recebem evento
```

### WebSocket Flow

```
1. Client → GET /api/v1/ws (upgrade)
2. Server → Accept WebSocket upgrade
3. Server → Subscribe ao hub
4. Loop:
   - Aguardar evento do hub
   - Escrever JSON envelope para cliente
   - Se erro ao escrever: close conexão
5. Ao fechar → Unsubscribe do hub
```
