# Fluxos de Comunicação — Backend

Detalhamento de todos os fluxos de comunicação entre backend, dependências externas e frontend.

---

## 1. Inicialização do Sistema

### Sequência

```
Backend Startup
  ↓
1. config.Load() — carregar variáveis de ambiente
2. mediaserver.NewClient() — criar cliente HTTP do MediaMTX
3. obs.New() — iniciar conexão WebSocket com OBS
4. events.NewHub() — criar event hub
5. orchestrator.New() — criar orquestrador
6. go orch.Run() — iniciar sync loop em background
7. httpapi.NewServer() — criar servidor HTTP
8. http.Server.ListenAndServe() — iniciar servidor HTTP
  ↓
Backend pronto para requisições
```

### OBS.New() — Fluxo Interno

```
obs.New(addr, password)
  ↓
1. Tentar conectar via WebSocket
   ↓
   Sucesso → Estado = Connected, retornar
   Falha → Estado = Connecting, iniciar retry loop
   
2. Retry loop em background:
   Wait 1s → Try connect
   Wait 2s → Try connect
   Wait 4s → Try connect
   ... (exponential backoff)
   
3. Quando conecta:
   - Enviar Hello frame
   - Aguardar Hello response
   - Ler versão do obs-websocket
   - Estado = Connected
   - Iniciar reader goroutine
   - Criar cena "Program" (se não existir)
```

---

## 2. Sincronização Periódica (Sync Loop)

### Timing

- Intervalo: `SYNC_INTERVAL` (padrão 3 segundos)
- Goroutine: `orchestrator.Run()`
- Executado: a cada intervalo (exceto durante shutdown)

### Sequência Detalhada

```
Sync Loop (a cada 3s)
  ↓
1. mediaserver.ListActiveStreams()
   ↓
   HTTP GET /v3/list no MediaMTX
   ↓
   Retorna: [{"name": "camera1"}, {"name": "camera2"}]
   
2. Compare com estado em memória (orchestrator.cameras)
   ↓
   Detectar:
   - Streams novos
   - Streams desaparecidos
   - Status updates

3. Para cada stream novo:
   ↓
   a. Criar Camera struct
   b. obs.CreateInput("cam_" + streamName, rtmpUrl)
      ↓
      OBS WebSocket RPC:
      {
        "request_type": "CreateInput",
        "request_id": "uuid",
        "request_data": {
          "sceneName": "Program",
          "inputName": "cam_camera1",
          "inputKind": "ffmpeg_source",
          "inputSettings": {
            "local_file": false,
            "input": "rtmp://localhost:1935/live/camera1"
          }
        }
      }
      ↓
      Se sucesso → obs.IsConnected() ainda true
      Se erro → logging, continua com próxima câmera
   
   c. hub.Publish("cameras.updated", [...])

4. Para cada stream desaparecido:
   ↓
   a. Incrementar camera.missingFor
   b. Se missingFor < offlineRemoveAfter (60s):
      - Marcar status como "offline"
      - hub.Publish("cameras.updated", [...])
   c. Se missingFor >= offlineRemoveAfter:
      - obs.DeleteInput("cam_" + streamName)
      - Remover camera do estado
      - hub.Publish("cameras.updated", [...])

5. Atualizar SystemStatus:
   ↓
   a. obsConnected = obs.IsConnected()
   b. mediaServerConnected = (erro do step 1 == nil)
   c. streaming = (len(cameras com isLive == true) > 0)
   d. activeSceneName = orchestrator.programScene
   e. liveCameraId = orchestrator.liveCamera

6. hub.Publish("system.status", SystemStatus{...})

7. Retornar e aguardar próximo tick
```

### Cenários de Erro

#### MediaMTX offline
```
ListActiveStreams() retorna erro
  ↓
Sync continua
  ↓
Cameras anteriores permanecem no estado
  ↓
Próxima sync (3s depois) tenta novamente
  ↓
Se persistir, sistema degrada mas não quebra
```

#### OBS offline
```
obs.CreateInput() ou DeleteInput() retorna erro
  ↓
Logging: "obs: failed to create input"
  ↓
Sync continua (nada é deletado)
  ↓
OBS Controller em background tenta reconectar
  ↓
Quando reconecta:
  - Recria cena "Program"
  - Recria todos os inputs
  - hub.Publish("system.status", {obsConnected: true})
```

---

## 3. Operador Seleciona Câmera "Ao Vivo"

### Sequência HTTP + WebSocket

```
Frontend (Painel)
  │
  ├─ Operador clica em card de camera1
  │
  └─→ POST /api/v1/cameras/camera1/live
      (header: X-Api-Token: dev-token)
        │
        ↓
Backend HTTP Server
  │
  ├─ Router → Encontra handler
  │
  ├─ Middleware tokenMiddleware()
  │   └─ Valida X-Api-Token
  │       └─ Se inválido: return 401 Unauthorized
  │
  ├─ Handler → orch.SetLive("camera1")
  │    │
  │    ├─ Validar:
  │    │  - camera1 existe? (se não: return ErrCameraNotFound)
  │    │  - camera1 está online? (se não: return ErrCameraOffline)
  │    │
  │    ├─ Se camera anterior era live:
  │    │  └─ obs.SetItemVisible("Program", "cam_old_id", false)
  │    │     │
  │    │     └─ OBS WebSocket RPC:
  │    │        {
  │    │          "request_type": "SetSourceFilterEnabled",
  │    │          "request_data": {
  │    │            "sceneName": "Program",
  │    │            "itemName": "cam_old_id",
  │    │            "itemEnabled": false
  │    │          }
  │    │        }
  │    │
  │    ├─ Ligar camera selecionada:
  │    │  └─ obs.SetItemVisible("Program", "cam_camera1", true)
  │    │     │
  │    │     └─ OBS WebSocket RPC:
  │    │        {
  │    │          "request_type": "SetSourceFilterEnabled",
  │    │          "request_data": {
  │    │            "sceneName": "Program",
  │            "itemName": "cam_camera1",
  │    │            "itemEnabled": true
  │    │          }
  │    │        }
  │    │
  │    └─ hub.Publish("system.status", {...liveCameraId: "camera1"})
  │
  └─→ Retornar 200 OK + JSON
      {
        "obsConnected": true,
        "mediaServerConnected": true,
        "streaming": true,
        "activeSceneName": "Program",
        "liveCameraId": "camera1"
      }
      │
      └─→ Frontend recebe resposta
          
Simultaneamente:

Events Hub
  │
  ├─ Broadcast para todas WebSocket connections
  │  │
  │  ├─ WebSocket Connection 1 (Frontend 1)
  │  │  └─ Recebe: {"type":"system.status", "payload":{...}}
  │  │     └─ Atualiza estado local
  │  │        └─ UI marca camera1 como live
  │  │
  │  ├─ WebSocket Connection 2 (Frontend 2)
  │  │  └─ Recebe: {"type":"system.status", "payload":{...}}
  │  │     └─ Atualiza estado local
  │  │        └─ UI marca camera1 como live
  │  │
  │  └─ ... (todos subscribers)
```

### Handlers & Responses

#### Success (200)
```
POST /api/v1/cameras/camera1/live HTTP/1.1
X-Api-Token: dev-token

HTTP/1.1 200 OK
Content-Type: application/json

{
  "obsConnected": true,
  "mediaServerConnected": true,
  "streaming": true,
  "activeSceneName": "Program",
  "liveCameraId": "camera1"
}
```

#### Camera não encontrada (404)
```
HTTP/1.1 404 Not Found
Content-Type: application/json

{"error": "camera not found"}
```

#### Camera offline (400)
```
HTTP/1.1 400 Bad Request
Content-Type: application/json

{"error": "camera is offline"}
```

#### Sem autenticação (401)
```
HTTP/1.1 401 Unauthorized
Content-Type: application/json

{"error": "missing or invalid api token"}
```

---

## 4. WebSocket — Push de Eventos

### Conexão WebSocket

```
Frontend
  │
  └─→ GET /api/v1/ws (upgrade)
      (header: X-Api-Token: dev-token)
      (header: Connection: Upgrade)
      (header: Upgrade: websocket)
        │
        ↓
Backend Server
  │
  ├─ Middleware → Validar token
  │
  ├─ Handler:
  │  │
  │  ├─ http.Hijack() → Pegar conexão TCP subjacente
  │  │
  │  ├─ websocket.Accept(conn, opts)
  │  │
  │  ├─ Registrar conexão em server.conns (para graceful shutdown)
  │  │
  │  ├─ hub.Subscribe() → Obter canal de eventos
  │  │
  │  └─ Loop:
  │     │
  │     ├─ Aguardar evento:
  │     │  event := <-eventsChan
  │     │
  │     ├─ Serializar envelope:
  │     │  envelope := WsEnvelope{
  │     │    Type: event.Type,
  │     │    Payload: event.Payload,
  │     │  }
  │     │
  │     ├─ Escrever JSON para WebSocket:
  │     │  websocket.Write(ctx, websocket.MessageText, jsonBytes)
  │     │
  │     └─ Loop continua até:
  │        - Cliente fecha conexão (return)
  │        - Erro ao escrever (return)
  │        - Context cancelado (return)
  │
  └─ Ao sair do loop:
     │
     ├─ websocket.Close()
     ├─ Unsubscribe do hub
     ├─ Remover de server.conns
```

### Eventos Enviados ao Frontend

#### cameras.updated
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
    },
    {
      "id": "camera2",
      "name": "camera2",
      "sourceUrl": "rtmp://localhost:1935/live/camera2",
      "status": "offline",
      "obsSourceCreated": false,
      "isLive": false,
      "lastSeenAt": "2025-07-19T15:25:00Z"
    }
  ]
}
```

**Quando é enviado:**
- Ao conectar WebSocket (snapshot inicial)
- Quando câmera nova é detectada
- Quando câmera muda de status (online → offline)
- Quando câmera é removida (após 60s offline)

#### system.status
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

**Quando é enviado:**
- Ao conectar WebSocket (snapshot inicial)
- A cada sync loop (atualização de status)
- Quando camera é setada como live
- Quando OBS conecta/desconecta
- Quando MediaMTX conecta/desconecta

---

## 5. OBS Desconecta / Reconecta

### Desconexão

```
OBS Studio fechado pelo operador
  │
  ↓
obs.Reader goroutine
  │
  ├─ Tenta ler frame do WebSocket
  │
  └─ Retorna erro (EOF ou connection reset)
      │
      ↓
obs.Controller
  │
  ├─ Loga: "obs: connection lost"
  │
  ├─ Estado = Disconnected
  │
  ├─ Iniciar retry loop:
  │
  │  1s wait → try connect → fail
  │  2s wait → try connect → fail
  │  4s wait → try connect → fail
  │  ... (exponential backoff, max 32s)
```

### Reconexão

```
OBS Studio reaberto pelo operador
  │
  ↓
Retry loop detecta sucesso na próxima tentativa
  │
  ├─ WebSocket connect sucesso
  │
  ├─ Hello handshake com OBS
  │
  ├─ Estado = Connected
  │
  ├─ Início da recriação:
  │  │
  │  ├─ Verificar se cena "Program" existe
  │  │  └─ Se não existe: CreateScene("Program")
  │  │
  │  ├─ Para cada camera em memória:
  │  │  └─ CreateInput("cam_" + id, rtmpUrl)
  │  │
  │  └─ Para camera que estava live:
  │     └─ SetItemVisible("Program", "cam_" + id, true)
  │
  ├─ Loga: "obs: reconnected and restored state"
  │
  └─ hub.Publish("system.status", {obsConnected: true})
     │
     └─ WebSocket subscribers notificados
```

### Frontend Observa

```
Frontend WebSocket listener
  │
  ├─ Recebe: {"type":"system.status", "payload":{obsConnected:false}}
  │  └─ UI mostra: "OBS Desconectado"
  │
  ├─ ... aguarda ...
  │
  └─ Recebe: {"type":"system.status", "payload":{obsConnected:true}}
     └─ UI mostra: "OBS Conectado"
```

---

## 6. MediaMTX Offline Temporário

### Sequência

```
MediaMTX cai (ou porta 9997 fica inacessível)
  │
  ↓
Sync loop (próxima iteração)
  │
  ├─ mediaserver.ListActiveStreams() retorna erro
  │
  ├─ Loga: "sync: failed to list streams: connection refused"
  │
  ├─ Estado anterior mantido (cameras não são deletadas)
  │
  ├─ hub.Publish("system.status", {...mediaServerConnected: false})
  │
  └─ WebSocket subscribers notificados
      │
      └─ Frontend UI mostra: "Media Server Desconectado"

... MediaMTX sobe novamente ...

Próximo sync loop:
  │
  ├─ ListActiveStreams() sucesso
  │
  ├─ hub.Publish("system.status", {...mediaServerConnected: true})
  │
  └─ WebSocket subscribers notificados
     │
     └─ Frontend UI mostra: "Media Server Conectado"
```

---

## 7. Camerá Desconecta (RTMP cai)

### Timeline

```
T=0s: Câmera conectada, streaming RTMP ativo
      │
      ├─ status = "online"
      ├─ obs.CreateInput() criado
      ├─ UI mostra: "online"

T=0.1s: Câmera para de enviar RTMP

T=3s (sync loop):
  │
  ├─ ListActiveStreams() não retorna camera1
  │
  ├─ Detectar: camera1.missingFor = 0 + 3s = 3s
  │
  ├─ 3s < 60s, então não deletar ainda
  │
  ├─ Marcar status = "offline"
  │
  ├─ hub.Publish("cameras.updated")
  │
  └─ UI mostra: "offline"

T=6s, T=9s, T=12s, ... (cada sync loop):
  │
  └─ camera1.missingFor += 3s

T=63s (sync loop, 21 iterações depois):
  │
  ├─ camera1.missingFor >= 60s
  │
  ├─ obs.DeleteInput("cam_camera1")
  │
  ├─ Remover camera1 do estado
  │
  ├─ hub.Publish("cameras.updated")
  │
  └─ UI mostra: camera1 desapareceu
```

---

## 8. Câmera Reconecta Após Breve Desconexão

### Timeline

```
T=0s: Camera1 offline, missingFor = 30s

T=33s (sync loop):
  │
  ├─ ListActiveStreams() retorna camera1
  │
  ├─ Detectar: camera1 volta!
  │
  ├─ Marcar status = "online"
  │
  ├─ camera.missingFor = 0
  │
  ├─ obs.CreateInput("cam_camera1", ...)  (já criado, pode ignorar erro)
  │
  ├─ hub.Publish("cameras.updated")
  │
  └─ UI mostra: "online"
```

---

## 9. Força Sincronização Manual (POST /sync)

### Sequência

```
Frontend
  │
  └─→ POST /api/v1/sync
      (header: X-Api-Token: dev-token)
        │
        ↓
Backend Handler
  │
  ├─ orch.SyncOnce(ctx)
  │  │
  │  └─ Executar sync imediatamente (não esperar timer)
  │
  └─→ Retornar 200 + []Camera
      │
      └─→ Frontend
          └─ Pode usar para atualizar UI
```

**Usado para:**
- Force refresh quando operador desconfia de outdated state
- Debug: verificar se câmeras são descobertas

---

## 10. Shutdown Gracioso

### Sequência

```
Sinal SIGINT/SIGTERM recebido (Ctrl+C)
  │
  ↓
signal.NotifyContext dispara
  │
  ├─ ctx.Done()
  │
  ├─ orchestrator.Run() recebe <-ctx.Done()
  │  └─ Sai do sync loop (sem panic)
  │
  ├─ apiServer.CloseAllWS()
  │  └─ Fecha todas conexões WebSocket
  │     └─ Clientes recebem close frame
  │
  ├─ httpServer.Shutdown(shutdownCtx)
  │  └─ Para de aceitar novas conexões
  │     └─ Aguarda conexões ativas finalizarem
  │        └─ Timeout de 3 segundos
  │
  ├─ obsCtl.Close()
  │  └─ Fecha WebSocket com OBS
  │
  └─ Sair (exit 0)
```

**Frontend observa:**
```
WebSocket close frame recebido
  │
  └─ onclose() → mostrar "Connection lost"
     └─ User pode fazer retry manual
```
