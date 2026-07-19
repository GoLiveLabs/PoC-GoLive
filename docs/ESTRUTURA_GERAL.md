# Estrutura Geral — Live Orchestrator

## Visão Geral

O **Live Orchestrator** é um sistema de orquestração de câmeras para transmissão ao vivo. Funciona como intermediário entre múltiplas fontes de vídeo (câmeras, MediaMTX), um controlador OBS Studio e um painel de controle web para o operador.

### Arquitetura em Alto Nível

```
┌──────────────────────────────────────────────────────────────┐
│                      FRONTEND (Angular)                       │
│  Painel de controle web — seleção de câmeras, status de live  │
└────────────────────────┬─────────────────────────────────────┘
                         │ REST + WebSocket
                         │
┌────────────────────────▼─────────────────────────────────────┐
│                    BACKEND (Go)                               │
│  Orquestrador central — sincronização e lógica de câmeras      │
│                                                                │
│  ┌─────────────────────────────────────────────────────────┐ │
│  │  HTTP API + WebSocket (httpapi)                         │ │
│  │  Endpoints: GET /cameras, POST /cameras/{id}/live, etc. │ │
│  └─────────────────────────────────────────────────────────┘ │
│                         △                                      │
│    ┌────────────────────┴─────────────────────┐               │
│    │                                          │               │
│  ┌─┴──────────────────┐         ┌────────────┴──────┐        │
│  │  Orchestrator      │         │  Events Hub       │        │
│  │  (core logic)      │         │  (pub/sub)        │        │
│  │  - Sync loop       │         └───────────────────┘        │
│  │  - Camera state    │                                       │
│  │  - Live selection  │                                       │
│  └─┬──────────────────┘                                       │
│    │      │         │                                         │
│    │      │         └─────────────────┐                      │
│    │      │                           │                      │
│  ┌─┴──────▼──┐         ┌──────────────┴──────┐               │
│  │ MediaMTX  │         │  OBS Studio        │               │
│  │ Client    │         │  Controller        │               │
│  │ (streams) │         │  (inputs/scenes)   │               │
│  └───────────┘         └────────────────────┘               │
└────────────────────────────────────────────────────────────┘
         △                              △
         │                              │
         │                              │
    ┌────┴────┐                   ┌──────────┐
    │ MediaMTX │                   │ OBS      │
    │ (HTTP)   │                   │ Studio   │
    └──────────┘                   │(WebSocket│
                                   │v5)       │
                                   └──────────┘
                                        △
         ┌──────────────────────────────┘
         │
    ┌────┴──────┐
    │  Câmeras  │
    │  (RTMP)   │
    └───────────┘
```

---

## Estrutura de Diretórios

### Backend (`/backend`)
```
backend/
├── cmd/
│   └── server/
│       └── main.go              Entry point do servidor
├── internal/
│   ├── config/
│   │   └── config.go             Carregamento de variáveis de ambiente
│   ├── httpapi/
│   │   ├── httpapi.go            Servidor HTTP + rotas
│   │   ├── middleware.go         Middleware (autenticação)
│   │   ├── ws.go                 Handler de WebSocket
│   │   └── httpapi_test.go       Testes
│   ├── mediaserver/
│   │   ├── client.go             Cliente HTTP da API MediaMTX
│   │   └── client_test.go        Testes
│   ├── obs/
│   │   ├── obs.go                Cliente WebSocket do OBS Studio
│   │   └── obsmock/
│   │       └── obsmock.go        Mock para testes
│   ├── orchestrator/
│   │   ├── models.go             Contratos (Camera, SystemStatus)
│   │   ├── orchestrator.go       Core da lógica de orquestração
│   │   └── orchestrator_test.go  Testes
│   └── events/
│       └── hub.go                Event hub (pub/sub)
```

### Frontend (`/frontend`)
```
frontend/src/
├── app/
│   ├── app.ts                    Componente raiz
│   ├── app.config.ts             Configuração do Angular
│   ├── core/
│   │   ├── models.ts             Tipos (Camera, SystemStatus)
│   │   ├── api.service.ts        Serviço HTTP da API
│   │   ├── api-token.interceptor.ts  Interceptor (token no header)
│   │   └── websocket.service.ts  Gerenciador de conexão WebSocket
│   └── features/
│       ├── camera-grid/
│       │   ├── camera-grid.component.ts      Grid de câmeras
│       │   └── camera-card.component.ts      Card individual
│       └── control-bar/
│           └── control-bar.component.ts      Barra de controle
├── environments/
│   └── environment.ts            Config de ambiente (URL da API)
└── main.ts                       Bootstrap do app
```

---

## Responsabilidades por Camada

### Backend

| Pacote | Responsabilidade |
|--------|------------------|
| `config` | Carregar variáveis de ambiente e fornecê-las aos componentes |
| `httpapi` | Expor a API REST + WebSocket; autenticar requisições via token |
| `mediaserver` | Comunicar-se com a API HTTP do MediaMTX; listar streams ativos |
| `obs` | Comunicar-se com OBS Studio via WebSocket; gerenciar inputs e cenas |
| `orchestrator` | Sincronizar câmeras (MediaMTX ↔ OBS), gerenciar estado em memória, lógica de "câmera ao vivo" |
| `events` | Pub/sub para notificar clientes WebSocket de mudanças de estado |

### Frontend

| Módulo | Responsabilidade |
|--------|------------------|
| `ApiService` | Fazer chamadas HTTP; construir URLs; injetar token |
| `WebSocketService` | Manter conexão WebSocket aberta; reconectar com backoff; notificar listeners |
| `Components` (grid, card, control-bar) | Renderizar UI; reagir a mudanças de estado; chamar actions do orquestrador |
| `Interceptor` | Adicionar `X-Api-Token` a todas as requisições HTTP |

---

## Fluxo de Comunicação

### 1. Inicialização do Sistema

```
Backend startup:
1. Carregar config
2. Criar MediaMTX client
3. Criar OBS controller
4. Criar Orchestrator (inicia sync loop)
5. Criar HTTP server

Frontend startup:
1. Carregar ambiente (URL da API)
2. Conectar WebSocket
3. Requestar snapshot inicial (GET /cameras, GET /status)
4. Renderizar grid
```

### 2. Chegada de Nova Câmera

```
Câmera envia RTMP → MediaMTX
    ↓
Sync loop do Orchestrator (a cada SYNC_INTERVAL):
  1. Query MediaMTX: /api/streams (lista de streams ativos)
  2. Compara com estado em memória
  3. Detecta stream novo
  4. Cria input no OBS com nome "cam_<camera_id>"
  5. Emite evento "cameras.updated"
    ↓
Event hub dispara para todas conexões WebSocket
    ↓
Frontend recebe { type: 'cameras.updated', payload: [...] }
    ↓
UI atualiza grid (novo card com status "offline")
```

### 3. Operador Seleciona Câmera como "Ao Vivo"

```
UI: Operador clica em card
    ↓
Frontend: POST /api/v1/cameras/{id}/live
    ↓
Backend SetLive():
  1. Valida se câmera existe e está online
  2. Faz source da câmera visível no OBS
  3. Emite evento "system.status" (atualiza liveCameraId)
    ↓
Event hub dispara para todas conexões WebSocket
    ↓
Frontend recebe { type: 'system.status', payload: {...} }
    ↓
UI destaca câmera selecionada no grid
```

### 4. Câmera Desconecta

```
RTMP cai → MediaMTX para receber stream
    ↓
Sync loop (próxima iteração):
  1. Detecta que stream desapareceu
  2. Marca câmera como offline (missingFor += interval)
  3. Aguarda offlineRemoveAfter (60s default) antes de remover OBS input
  4. Emite "cameras.updated"
    ↓
Frontend: UI marca câmera como "offline" (visual feedback)
    ↓
Se era câmera ao vivo: Frontend emite ação para "desligar" aquela câmera
```

### 5. OBS Desconecta / Reconecta

```
OBS fecha ← obs.Controller detecta desconexão
    ↓
Observer pega erro em write/read
    ↓
Emite log "obs: connection lost"
    ↓
Retry loop com exponential backoff (1s, 2s, 4s, 8s...)
    ↓
Quando reconecta:
  1. Re-cria cena "Program" (se não existir)
  2. Re-cria todos os inputs conhecidos
  3. Sincroniza estado
  4. Emite "system.status" (obsConnected = true)
    ↓
Frontend: UI mostra "OBS reconectado"
```

---

## Fluxo de Dados — Detalhado

### Direção: Backend → Frontend (Push via WebSocket)

**Evento `cameras.updated`**
```json
{
  "type": "cameras.updated",
  "payload": [
    {
      "id": "camera1",
      "name": "camera1",
      "sourceUrl": "rtmp://localhost:1935/live/camera1",
      "status": "online",
      "obsSourceCreated": true,
      "isLive": true,
      "lastSeenAt": "2025-07-19T15:30:45Z"
    }
  ]
}
```

**Evento `system.status`**
```json
{
  "type": "system.status",
  "payload": {
    "obsConnected": true,
    "mediaServerConnected": true,
    "streaming": true,
    "activeSceneName": "Program",
    "liveCameraId": "camera1"
  }
}
```

### Direção: Frontend → Backend (Requisições HTTP/REST)

**GET `/api/v1/cameras`** → Lista todas as câmeras conhecidas
**GET `/api/v1/status`** → Status do sistema agora
**POST `/api/v1/cameras/{id}/live`** → Seleciona câmera como ao vivo
**POST `/api/v1/sync`** → Força sincronização imediata

---

## Ciclo de Sincronização

O **Sync Loop** roda a cada `SYNC_INTERVAL` (padrão 3s):

```go
loop:
  1. ctx.ListActiveStreams()       // Query MediaMTX
  2. Compare com cameras in-memory
  3. Detecta novos, que saíram, etc.
  4. Para cada câmera:
     - Se nova: CriarInput no OBS
     - Se offline por 60s: RemoverInput do OBS
     - Se status mudou: EmitirEvento()
  5. Sleep até próxima iteração
```

---

## Contrato de Dados Compartilhado

### Backend ↔ Frontend

**Modelos** (Go `orchestrator/models.go` ↔ TS `core/models.ts`):
- `Camera`: id, name, sourceUrl, status, obsSourceCreated, isLive, lastSeenAt
- `SystemStatus`: obsConnected, mediaServerConnected, streaming, activeSceneName, liveCameraId
- `WsEnvelope`: type, payload

Mantém sincronização através de JSON; Frontend espelha tipos do Backend.

---

## Dependências Externas

| Serviço | Porta | Protocolo | Usado por |
|---------|-------|-----------|-----------|
| MediaMTX API | 9997 | HTTP | Backend `mediaserver.Client` |
| OBS Studio | 4455 | WebSocket | Backend `obs.Controller` |
| Frontend | 4200 | HTTP (dev) | Navegador |
| Backend | 8080 | HTTP + WS | Frontend |

---

## Próximos Passos — Extensibilidade

**Áreas para expansão:**
1. **Múltiplas cenas** — atualmente fixa em "Program"; permitir selecionar layout
2. **Presets de layout** — salvar/carregar configurações de câmeras
3. **Autenticação de usuário** — OAuth2 ou similar (atualmente apenas token)
4. **Persistência** — salvar estado em DB (atualmente em memória)
5. **Preview de vídeo** — HLS ou similar no frontend
6. **Métricas** — exposição de Prometheus, dashboards de latência
