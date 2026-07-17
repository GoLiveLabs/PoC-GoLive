# Documentação do Backend (Go)

Backend do orquestrador de câmeras para transmissão ao vivo. Monitora as câmeras
ativas no MediaMTX, sincroniza-as como fontes (`ffmpeg_source`) dentro do OBS Studio
via obs-websocket v5, e expõe uma API REST + WebSocket para o painel do operador.

## Índice

- [Arquitetura geral](#arquitetura-geral)
- [Estrutura de pacotes](#estrutura-de-pacotes)
- [Configuração (env vars)](#configuração-env-vars)
- [Pacotes em detalhe](#pacotes-em-detalhe)
  - [config](#internalconfig)
  - [mediaserver](#internalmediaserver)
  - [obs](#internalobs)
  - [orchestrator](#internalorchestrator)
  - [events](#internalevents)
  - [httpapi](#internalhttpapi)
- [Contrato da API REST](#contrato-da-api-rest)
- [Contrato do WebSocket](#contrato-do-websocket)
- [Modelos de dados](#modelos-de-dados)
- [Ciclo de vida e shutdown](#ciclo-de-vida-e-shutdown)
- [Logging](#logging)
- [Testes](#testes)
- [Dependências externas](#dependências-externas)

---

## Arquitetura geral

```
                     ┌─────────────────────────────────────────────┐
                     │                Backend (Go)                 │
                     │                                             │
 Câmeras ──RTMP──▶ MediaMTX ◀──HTTP──┐                             │
                     │               │                             │
                     │        ┌──────┴───────┐    ┌─────────────┐  │
                     │        │ mediaserver  │    │     obs     │──┼──ws──▶ OBS Studio
                     │        │   (client)   │    │ (Controller)│  │       (obs-websocket v5)
                     │        └──────┬───────┘    └──────▲──────┘  │
                     │               │                   │         │
                     │        ┌──────▼───────────────────┴──────┐  │
                     │        │          orchestrator           │  │
                     │        │  (sync loop + estado em memória)│  │
                     │        └──────┬───────────────┬──────────┘  │
                     │               │               │             │
                     │        ┌──────▼──────┐ ┌──────▼──────┐      │
                     │        │   events    │ │   httpapi   │──────┼──REST/WS──▶ Painel Angular
                     │        │    (hub)    │ │  (handlers) │      │
                     │        └─────────────┘ └─────────────┘      │
                     └─────────────────────────────────────────────┘
```

Princípios de projeto:

- **Somente stdlib para HTTP** — `net/http` com o routing por método do Go 1.22+
  (`GET /api/v1/cameras`, `POST /api/v1/cameras/{id}/live`). Sem Gin/Echo.
- **Dependências injetadas por interface** — o orquestrador depende de
  `obs.Controller` e `orchestrator.MediaServerClient` (interfaces), permitindo
  testes unitários com mocks sem OBS/MediaMTX reais.
- **`context.Context` como primeiro parâmetro** em toda operação de I/O.
- **Erros embrulhados** com `fmt.Errorf("...: %w", err)` e erros sentinela
  (`ErrCameraNotFound` etc.) mapeados para status HTTP na camada `httpapi`.
- **Estado em memória** — nenhum banco de dados; o estado é reconstruído a partir
  do MediaMTX a cada ciclo de sync.

## Estrutura de pacotes

```
backend/
├── go.mod                       # módulo: live-orchestrator/backend
├── cmd/
│   └── server/
│       └── main.go              # entrypoint: config, DI, HTTP server, shutdown
└── internal/
    ├── config/                  # leitura de env vars com defaults
    ├── mediaserver/             # cliente HTTP da API do MediaMTX
    ├── obs/                     # wrapper da goobs (interface Controller)
    │   └── obsmock/             # mock in-memory de Controller para testes
    ├── orchestrator/            # regra de negócio: sync loop + SetLive
    ├── events/                  # hub pub/sub in-memory para eventos WS
    └── httpapi/                 # handlers REST, middleware, WebSocket
```

## Configuração (env vars)

Lidas em `internal/config.Load()`. Toda variável tem default — o backend sobe sem
nenhuma configuração em ambiente de dev padrão.

| Variável | Default | Campo | Descrição |
|---|---|---|---|
| `HTTP_ADDR` | `:8080` | `HTTPAddr` | endereço de escuta do servidor HTTP |
| `OBS_ADDR` | `localhost:4455` | `OBSAddr` | host:porta do obs-websocket |
| `OBS_PASSWORD` | *(vazio)* | `OBSPassword` | senha do obs-websocket (vazio = sem auth) |
| `MEDIAMTX_API_URL` | `http://localhost:9997` | `MediaMTXAPIURL` | URL base da API do MediaMTX |
| `API_TOKEN` | `dev-token` | `APIToken` | token exigido no header `X-Api-Token` |
| `SYNC_INTERVAL` | `3s` | `SyncInterval` | intervalo do loop de sincronização (formato `time.ParseDuration`) |
| `PROGRAM_SCENE` | `Program` | `ProgramScene` | nome da cena de programa no OBS |
| `LOG_LEVEL` | `info` | `LogLevel` | `debug` \| `info` \| `warn` \| `error` |

Valores inválidos de `SYNC_INTERVAL` caem silenciosamente no default. Variáveis
definidas mas vazias também usam o default.

## Pacotes em detalhe

### `internal/config`

Um único arquivo com a struct `Config` e `Load()`. Sem dependências além de
`os` e `time`. Não valida combinações — a responsabilidade é só ler e aplicar defaults.

### `internal/mediaserver`

Cliente HTTP mínimo da API v3 do MediaMTX.

```go
type StreamInfo struct {
    Name  string
    Ready bool
}

func NewClient(baseURL string) *Client
func (c *Client) ListActiveStreams(ctx context.Context) ([]StreamInfo, error)
```

- Chama `GET {baseURL}/v3/paths/list` e decodifica `{ items: [{ name, ready, ... }] }`.
- **Filtra apenas paths com `ready: true`** — um path registrado sem publisher ativo
  não vira câmera.
- Timeout de **5s** por chamada (tanto no `http.Client` quanto via `context.WithTimeout`).
- Erros de rede retornam `error` para o chamador; nunca derrubam o processo
  (o orquestrador trata a falha marcando `mediaServerConnected=false`).

Shape real da resposta (confirmado contra MediaMTX v1.19.2 — ver `DECISIONS.md`):

```json
{
  "itemCount": 2,
  "pageCount": 1,
  "items": [
    { "name": "camera1", "ready": true, "readyTime": "...", "source": {...}, "tracks": [...] }
  ]
}
```

### `internal/obs`

Wrapper da lib [`goobs`](https://github.com/andreykaipov/goobs) atrás de uma
interface própria:

```go
type Controller interface {
    EnsureScene(name string) error
    CreateCameraInput(sceneName, inputName, url string) error
    RemoveInput(inputName string) error
    SetOnlyVisibleSource(sceneName, inputName string) error
    IsConnected() bool
    Reconnect() error
}
```

Constantes importantes:

- `InputKind = "ffmpeg_source"` — kind dos inputs de câmera no OBS.
- `CamPrefix = "cam_"` — prefixo de todo input criado pelo orquestrador. Evita
  colisão com fontes criadas manualmente pelo operador e delimita o escopo do
  `SetOnlyVisibleSource` (fontes sem o prefixo nunca são tocadas).

Comportamento dos métodos:

| Método | Comportamento |
|---|---|
| `EnsureScene` | Lista as cenas; cria a cena só se não existir (idempotente). |
| `CreateCameraInput` | Se o input já existe, atualiza settings via `SetInputSettings` (overlay=false); senão cria via `CreateInput` já anexado à cena e habilitado. |
| `RemoveInput` | Remove o input pelo nome (o OBS remove os scene items associados). |
| `SetOnlyVisibleSource` | Lista os scene items da cena; habilita o item alvo e **desabilita todos os outros itens com prefixo `cam_`**. Fontes do operador (sem prefixo) ficam intactas. |

Settings do `ffmpeg_source` (nomes confirmados em runtime via `GetInputDefaultSettings`):

```json
{
  "input": "<url rtmp>",
  "is_local_file": false,
  "reconnect_delay_sec": 2,
  "buffering_mb": 1
}
```

**Reconexão:** `New(addr, password)` tenta conectar imediatamente (best-effort;
se o OBS não estiver de pé, o controller inicia desconectado) e dispara uma
goroutine `watchLoop` que:

1. A cada **5s** faz um health check ativo (`General.GetVersion`).
2. Se falhar, marca desconectado e tenta `Reconnect()` com **backoff exponencial
   1s → 2s → 4s → ... → máx 30s**.
3. Ao reconectar, reseta o backoff.

`IsConnected()` é thread-safe (`sync.RWMutex`). `Close()` para o loop e desconecta.
A lib `goobs` não expõe um canal de desconexão público, por isso o health check
ativo (ver `DECISIONS.md`).

#### `internal/obs/obsmock`

Mock in-memory de `Controller` para testes (`obsmock.New()`). Mantém mapas de
cenas, inputs (`inputName → url`) e fonte visível por cena; é thread-safe e tem
campos públicos (`Connected`, `ReconnectErr`, `ReconnectCalls`) para os testes
manipularem o estado. Uma asserção `var _ obs.Controller = (*Mock)(nil)` garante
que o mock não desalinha da interface.

### `internal/orchestrator`

Núcleo da regra de negócio.

```go
func New(mediaClient MediaServerClient, obsCtl obs.Controller, hub *events.Hub,
         programScene string, syncInterval time.Duration) *Orchestrator

func (o *Orchestrator) Run(ctx context.Context)              // loop de sync (bloqueante)
func (o *Orchestrator) SyncOnce(ctx context.Context) []Camera // um ciclo de sync
func (o *Orchestrator) Cameras() []Camera                     // snapshot ordenado por ID
func (o *Orchestrator) Status() SystemStatus
func (o *Orchestrator) SetLive(cameraID string) (SystemStatus, error)
```

Estado interno (protegido por `sync.RWMutex`):

- `cameras map[string]*Camera` — mapa por ID (ID = nome do path no MediaMTX).
- `offlineSince map[string]time.Time` — quando cada câmera ficou offline.
- `liveCameraID string` — ID da câmera atualmente no ar (vazio = nenhuma).
- `mediaConnected bool` — resultado do último contato com o MediaMTX.

**Algoritmo do ciclo de sync (`SyncOnce`):**

1. Busca streams ativos no MediaMTX.
   - Em erro: marca `mediaServerConnected=false`; se era `true`, publica evento
     `error` ("Não foi possível conectar ao servidor de mídia.") e `system.status`.
2. Para cada stream ativo:
   - **Câmera nova** → adiciona ao mapa (`status=online`) e agenda criação do
     input no OBS (`cam_<id>`); ao criar com sucesso, `obsSourceCreated=true`.
   - **Câmera que estava offline e voltou** → `status=online`, timer de offline
     descartado (o input do OBS nunca chegou a ser removido — sem flicker).
3. Para cada câmera do mapa que **não** apareceu no sync:
   - Se estava online → `status=offline`, inicia o timer; se era a câmera live,
     publica evento `error` avisando o operador (**não troca automaticamente** —
     decisão do plano: o operador escolhe a próxima câmera).
   - Se está offline há **60s ou mais** → remove o input do OBS e tira do mapa;
     se era a live, limpa `liveCameraID`.
4. Publica `cameras.updated` se algo mudou, e `system.status` quando relevante.

As chamadas ao OBS acontecem **fora do lock** (padrão collect-then-apply via
struct `pendingActions`) para não segurar o mutex durante I/O de rede.

**`SetLive(cameraID)`:**

| Condição | Resultado |
|---|---|
| ID não existe no mapa | `ErrCameraNotFound` (→ HTTP 404) |
| Câmera `offline` | `ErrCameraOffline` (→ HTTP 409) |
| OBS desconectado ou chamada falha | `ErrOBSUnreachable` (→ HTTP 502) |
| Sucesso | `SetOnlyVisibleSource` no OBS; `isLive=true` só nela; publica `cameras.updated` + `system.status` |

Invariante: **no máximo uma câmera com `isLive=true`** a qualquer momento.

### `internal/events`

Hub pub/sub mínimo, sem dependências externas:

```go
type Event struct {
    Type    string
    Payload any
}

func NewHub() *Hub
func (h *Hub) Subscribe() (<-chan Event, func()) // canal + cancel
func (h *Hub) Publish(e Event)
```

- Cada assinante recebe um canal com buffer de **16** eventos.
- `Publish` é não-bloqueante: assinante lento (buffer cheio) **perde o evento**
  em vez de travar o publisher. Aceitável porque todo evento de estado carrega o
  snapshot completo — o próximo evento corrige qualquer perda.
- O `cancel` retornado remove a assinatura e fecha o canal (idempotente).

### `internal/httpapi`

Camada HTTP. Depende do orquestrador via interface local (`httpapi.Orchestrator`),
o que permite testar os handlers com um fake.

```go
func NewServer(orch Orchestrator, hub *events.Hub, apiToken string) *Server
func (s *Server) Handler() http.Handler   // rotas + CORS + auth
func (s *Server) CloseAllWS()             // fecha todas as conexões WS (shutdown)
```

**Middlewares (ordem: CORS → auth → mux):**

- **CORS** — libera `http://localhost:4200` (Angular dev server), headers
  `Content-Type, X-Api-Token`, métodos `GET, POST, OPTIONS`. Responde preflight
  `OPTIONS` com 204.
- **Auth** — exige `X-Api-Token: <token>` em todas as rotas **exceto**
  `/api/v1/health` e requisições `OPTIONS`. Como a API `WebSocket` do navegador
  não permite headers customizados, o token também é aceito via query string
  `?api_token=<token>` (usado pelo frontend no `/ws`). Token ausente/errado → 401.

**WebSocket (`GET /api/v1/ws`):**

1. `websocket.Accept` (lib `coder/websocket`) com `OriginPatterns` restrito a
   `localhost:4200` / `127.0.0.1:4200`.
2. Registra a conexão para o shutdown (`trackConn`/`CloseAllWS`).
3. `CloseRead` — clientes não enviam mensagens; frames de controle são
   respondidos automaticamente e o contexto retornado é cancelado no disconnect.
4. Envia **snapshot inicial**: `cameras.updated` + `system.status`.
5. Assina o hub e repassa cada evento como JSON.
6. **Heartbeat**: ping a cada 30s; falha no ping encerra o handler.
7. `defer cancel()` limpa a assinatura do hub ao desconectar.

## Contrato da API REST

Base: `http://localhost:8080/api/v1`. Todas as rotas exigem `X-Api-Token`,
exceto `/health`.

| Método | Rota | Descrição | Resposta OK |
|---|---|---|---|
| GET | `/health` | liveness (sem token) | `{"status":"ok"}` |
| GET | `/cameras` | lista câmeras conhecidas | `Camera[]` |
| GET | `/status` | status geral do sistema | `SystemStatus` |
| POST | `/cameras/{id}/live` | coloca a câmera no ar | `SystemStatus` |
| POST | `/sync` | força um ciclo de sync imediato | `Camera[]` |
| GET | `/ws` | upgrade para WebSocket | — |

**Erros** — JSON `{"error": "<mensagem pt-BR>"}` com status:

| Status | Quando | Mensagem |
|---|---|---|
| 401 | token ausente/inválido | `token de acesso inválido ou ausente` |
| 404 | câmera não existe | `câmera não encontrada` |
| 409 | câmera offline no `SetLive` | `câmera está offline` |
| 502 | OBS inacessível na ação | `OBS está inacessível no momento` |
| 500 | erro inesperado | `erro interno` |

## Contrato do WebSocket

Envelope: `{"type": string, "payload": object}`.

| type | payload | quando |
|---|---|---|
| `cameras.updated` | `Camera[]` (lista completa) | qualquer mudança na lista/estado das câmeras |
| `system.status` | `SystemStatus` | mudança de conexão OBS/MediaMTX, troca de live, remoção de câmera |
| `error` | `{"message": string}` (pt-BR) | queda da câmera live, perda do MediaMTX |

O snapshot inicial (`cameras.updated` + `system.status`) é enviado imediatamente
após a conexão — o cliente nunca precisa fazer GET para se hidratar.

## Modelos de dados

### Camera

```json
{
  "id": "camera1",
  "name": "camera1",
  "sourceUrl": "rtmp://mediamtx:1935/camera1",
  "status": "online",
  "obsSourceCreated": true,
  "isLive": false,
  "lastSeenAt": "2026-07-16T14:00:00Z"
}
```

- `id` / `name`: derivados do nome do path no MediaMTX (únicos).
- `status`: `"online" | "offline"` (constantes `StatusOnline`/`StatusOffline`).
- `obsSourceCreated`: `true` depois que o input `cam_<id>` foi criado no OBS.
- `isLive`: `true` apenas na câmera visível na cena de programa.
- `lastSeenAt`: última vez que o path apareceu `ready` num sync.

### SystemStatus

```json
{
  "obsConnected": true,
  "mediaServerConnected": true,
  "streaming": false,
  "activeSceneName": "Program",
  "liveCameraId": "camera1"
}
```

- `streaming`: nesta versão, `true` sse `liveCameraId != ""` (não consulta o
  estado de streaming do OBS).

## Ciclo de vida e shutdown

`cmd/server/main.go`:

1. `config.Load()` + `setupLogging()`.
2. `signal.NotifyContext(SIGINT/SIGTERM)` — `Ctrl+C` cancela o contexto raiz.
3. Cria `mediaserver.Client`, `obs.ObsController` (já inicia a goroutine de
   reconexão), `events.Hub` e `orchestrator.Orchestrator`.
4. `go orch.Run(orchCtx)` — o loop de sync roda em contexto próprio, cancelável.
5. `httpServer.ListenAndServe()` em goroutine.
6. Ao receber sinal: cancela o loop de sync → `CloseAllWS()` (destrava os
   handlers WS de longa duração) → `httpServer.Shutdown` com timeout de **3s** →
   `obsCtl.Close()`.

Critério validado: encerra em bem menos de 3s, sem stack trace.

## Logging

`log/slog` com `TextHandler` em stderr. Nível via `LOG_LEVEL`. Eventos logados:

- `INFO` — servidor escutando, OBS reconectado, shutdown.
- `WARN` — falha de conexão inicial/reconexão com OBS (com backoff atual), perda
  do MediaMTX, falha ao criar/remover input de câmera, falha no accept do WS.
- `ERROR` — erro fatal do servidor HTTP, erro no shutdown.

## Testes

`go test ./...` — tudo com a stdlib (`testing` + `httptest`), sem frameworks.

| Pacote | O que cobre |
|---|---|
| `mediaserver` | lista vazia; 2 streams ativos; filtro de não-ready; erro 500; timeout — tudo via `httptest.Server` |
| `orchestrator` | câmera aparece; some e volta antes de 60s; some por 60s+ (remoção); `SetLive` em offline (409) e inexistente (404); duas trocas seguidas (invariante de 1 live); OBS desconectado |
| `httpapi` | health sem token; 401 sem token; listagem com token; `SetLive` 404/409/502/feliz — com orquestrador fake |
| `obs` | sem testes unitários (fala com servidor real); coberto pelo mock + checklist manual do README, executado com sucesso contra OBS real (ver `DECISIONS.md`) |

## Dependências externas

| Módulo | Uso |
|---|---|
| `github.com/andreykaipov/goobs` v1.9.0 | cliente obs-websocket v5 |
| `github.com/coder/websocket` v1.8.15 | WebSocket server (handlers `/ws`) |

Todo o resto é stdlib.
