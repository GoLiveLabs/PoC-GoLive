# Fluxos de Comunicação — Frontend

Detalhamento de todos os fluxos de comunicação entre frontend, serviços, backend e interações com usuário.

---

## 1. Inicialização da Aplicação

### Sequência Completa

```
Bootstrap Angular
  │
  ├─ Carregar appConfig
  │  ├─ Providers: ApiService, WebSocketService
  │  ├─ Interceptors: ApiTokenInterceptor
  │  └─ HttpClient setup
  │
  ├─ Instanciar AppComponent
  │
  ├─ Renderizar template
  │
  └─ Chamar AppComponent.ngOnInit()
     │
     ├─ Resolver WebSocket URL
     │  └─ ws://localhost:8080/api/v1/ws
     │
     ├─ WebSocketService.connect()
     │  │
     │  ├─ WebSocket ← open connection
     │  │  └─ connectionState: 'open' (ou 'closed' + retry)
     │  │
     │  └─ Aguardar eventos (onMessage$)
     │
     ├─ ApiService.getCameras()
     │  │
     │  ├─ HTTP GET /api/v1/cameras
     │  │  (Interceptor adiciona X-Api-Token)
     │  │
     │  └─ cameras.set([...])
     │     └─ UI re-renderiza grid
     │
     ├─ ApiService.getStatus()
     │  │
     │  ├─ HTTP GET /api/v1/status
     │  │
     │  └─ systemStatus.set({...})
     │     └─ UI atualiza control bar
     │
     └─ Subscribe a WebSocketService.onMessage$
        │
        └─ Para cada evento:
           ├─ cameras.updated → cameras.set()
           ├─ system.status → systemStatus.set()
           └─ error → mostrar alert
```

### Timeline

```
T=0ms: Bootstrap inicia
T=10ms: AppComponent criado
T=50ms: Template renderizado (valores iniciais vazios)
T=100ms: ngOnInit() executa
T=110ms: WebSocket.connect() inicia
T=120ms: HTTP getCameras() + getStatus() disparam
T=180ms: HTTP responses chegam
   cameras.set([...]) → Camera grid renderiza
   systemStatus.set({...}) → Control bar atualiza
T=200ms: WebSocket conecta
   Recebe snapshot cameras.updated
   cameras já atualizado via HTTP, sem mudança
   Recebe snapshot system.status
   systemStatus atualizado, animação suave
```

---

## 2. WebSocket — Manter Conexão Viva

### Ping/Pong (Keepalive)

```
Frontend WebSocket
  │
  ├─ A cada 30s:
  │  └─ Enviar ping frame (feito automaticamente pelo navegador)
  │
  └─ Backend (OBS Server, etc.) responde com pong
     └─ Conexão permanece viva
```

### Detecção de Desconexão

```
WebSocket close/error event
  │
  ├─ Causa possível:
  │  ├─ Backend server crashed
  │  ├─ Network loss (WiFi drop)
  │  ├─ Browser tab foi minimizado por muito tempo
  │  └─ Servidor fechou conexão
  │
  └─ Handler:
     │
     ├─ connectionState.set('closed')
     │
     ├─ onError$.next('WebSocket connection lost')
     │
     └─ reconnectWithBackoff()
        │
        └─ Retry com exponential backoff
           1s → 2s → 4s → 8s → 16s → 32s
```

### Reconexão

```
Retry loop ativo
  │
  ├─ Wait 1s
  ├─ Tentar conectar
  │  ├─ Sucesso → connectionState: 'open'
  │  │  └─ Retornar (resetar counter)
  │  └─ Falha → Continuar retry
  │
  ├─ Wait 2s
  ├─ Tentar conectar
  │  └─ ...
  │
  └─ ... (up to 32s)

Quando reconecta com sucesso:
  │
  ├─ connectionState: 'open'
  ├─ reconnectAttempts: reset a 0
  ├─ Aguardar eventos normalmente
  └─ Snapshot inicial enviado pelo backend
     ├─ cameras.updated
     └─ system.status
```

---

## 3. Operador Clica em Card — Set Live

### User Action → HTTP Request

```
UI: CameraCardComponent
  │
  ├─ Renderizar botão "Set Live"
  │  └─ [disabled]="camera.status === 'offline'"
  │
  └─ Operador clica no botão
     │
     └─ selectLive() executa
        │
        └─ this.setLive.emit('camera1')
           │
           └─ CameraGridComponent.onSetLive('camera1')
              │
              └─ this.api.setLive('camera1').subscribe({
                   next: (status) => {
                     // Sucesso
                     // Status já foi atualizado via WebSocket
                   },
                   error: (error) => {
                     // Mostrar erro ao operador
                   }
                 })
```

### HTTP Request Pipeline

```
1. ApiService.setLive()
   └─ Criar HttpRequest para POST /api/v1/cameras/camera1/live

2. HttpClient.post()
   └─ Enviar para pipeline de interceptors

3. ApiTokenInterceptor.intercept()
   └─ req.clone({ setHeaders: { 'X-Api-Token': 'dev-token' } })

4. Browser (XHR)
   ├─ POST http://localhost:8080/api/v1/cameras/camera1/live
   ├─ Headers:
   │  ├─ X-Api-Token: dev-token
   │  ├─ Content-Type: application/json
   │  └─ ...
   └─ Body: {} (empty)

5. Backend Handler
   ├─ Validar token
   ├─ orch.SetLive('camera1')
   ├─ hub.Publish('system.status')
   └─ Retornar 200 + JSON

6. Browser recebe response
   └─ Passar para subscribe observers

7. CameraGridComponent.onSetLive()
   └─ Receber SystemStatus no next()
      └─ (signal já atualizado via WebSocket)
```

### Simultaneous Events

```
HTTP Response chega
  │
  └─ CameraGridComponent.next(systemStatus)
     └─ Log ou toast: "Success"

Paralelamente:

Events Hub (Backend)
  │
  └─ Broadcast para todas WebSocket connections
     │
     ├─ Connection 1
     │  └─ Recebe system.status
     │     └─ systemStatus.set(...)
     │
     ├─ Connection 2
     │  └─ Recebe system.status
     │
     └─ ... (todos subscribers)

Frontend UI React:

1. systemStatus.set({liveCameraId: 'camera1'})
   │
   └─ Angular Signals detecta mudança
      │
      └─ Componentes que dependem de systemStatus() re-renderizam
         │
         ├─ CameraCardComponent:
         │  ├─ camera1: [class.live]="isLive" → aplicar estilos
         │  └─ Outros: [class.live]="isLive" → remover estilos
         │
         └─ ControlBarComponent atualiza
```

---

## 4. Camera Grid — Atualizar Lista

### WebSocket evento cameras.updated

```
Backend Sync Loop
  │
  └─ Detecta nova câmera
     │
     └─ hub.Publish('cameras.updated', [...])

WebSocket broadcast
  │
  ├─ Envelope JSON:
  │  {
  │    "type": "cameras.updated",
  │    "payload": [{...}, {...}, ...]
  │  }
  │
  └─ Transmitir para todos clients

Frontend WebSocket Handler
  │
  └─ onMessage$.next(envelope)
     │
     └─ AppComponent.ngOnInit subscribe:
        │
        ├─ Verificar envelope.type === 'cameras.updated'
        │
        └─ this.cameras.set(envelope.payload)
           │
           └─ Signal actualizado
              │
              └─ CameraGridComponent renderiza
                 │
                 ├─ @for (camera of cameras(); track camera.id)
                 │  └─ Angular Signals change detection
                 │     │
                 │     ├─ Câmeras novas → criar CameraCardComponent
                 │     ├─ Câmeras removidas → deletar CameraCardComponent
                 │     └─ Câmeras atualizadas → re-render card (se status mudou)
                 │
                 └─ UI mostra estado atualizado (smooth animation)
```

### Exemplo de Mudança

```
Estado Anterior:
cameras = [
  { id: 'camera1', status: 'online', isLive: true }
]

Backend Sync:
  Detecta camera2 novo
  Detecta camera1 está offline

Backend Publish:
  cameras.updated → [
    { id: 'camera1', status: 'offline', isLive: false },
    { id: 'camera2', status: 'online', isLive: false }
  ]

Frontend Recebe:
  this.cameras.set([...])

UI Renderiza:
  Câmera 1: 
    - Mudar cor para vermelho (offline)
    - Desabilitar botão "Set Live"
  
  Câmera 2:
    - Novo card aparece
    - Verde (online)
    - Botão "Set Live" habilitado
```

---

## 5. Control Bar — Monitorar Status

### SystemStatus Atualizado

```
WebSocket evento system.status
  │
  └─ Backend emite a cada sync loop
     │
     ├─ obsConnected: true/false
     ├─ mediaServerConnected: true/false
     ├─ streaming: true/false
     └─ ...

Frontend Recebe
  │
  ├─ onMessage$ (cameras.updated ou system.status)
  │
  └─ Verificar type === 'system.status'
     │
     └─ this.systemStatus.set(payload)
        │
        └─ ControlBarComponent re-renderiza
           │
           ├─ Status OBS: conectado/desconectado (verde/vermelho)
           ├─ Status MediaServer: conectado/desconectado
           └─ Streaming: ativo/inativo
```

### Exemplo Visual

```
ControlBarComponent Template:

<div class="control-bar">
  <div [class.connected]="status().obsConnected">
    OBS: {{ status().obsConnected ? '✓ Connected' : '✗ Disconnected' }}
  </div>
  <div [class.connected]="status().mediaServerConnected">
    Media Server: {{ status().mediaServerConnected ? '✓' : '✗' }}
  </div>
  <div [class.streaming]="status().streaming">
    Streaming: {{ status().streaming ? '▶ Active' : '⏹ Inactive' }}
  </div>
</div>

CSS:
.connected { color: green; }
.connected:not(.connected) { color: red; }
.streaming { color: green; }
.streaming:not(.streaming) { color: gray; }
```

---

## 6. Error Handling — API Error

### Scenario: Camera Not Found

```
User clica em card "Set Live"
  │
  ├─ this.api.setLive('camera-fake').subscribe({
  │
  └─ Backend retorna 404
     │
     └─ error: HttpErrorResponse {
          status: 404,
          message: 'camera not found'
        }

Frontend Error Handler:
  │
  ├─ error: (error: HttpErrorResponse) => {
  │  │
  │  └─ if (error.status === 404) {
  │     └─ alert('Camera not found');
  │        // Ou toast mais elegante
  │     }
  │  }
```

### Scenario: Camera is Offline

```
User tenta setLive() em câmera offline
  │
  └─ Backend retorna 400
     │
     └─ error: { status: 400, message: 'camera is offline' }

Frontend:
  │
  ├─ error: (error) => {
  │  └─ if (error.status === 400) {
  │     └─ alert('Camera is offline. Waiting for reconnection...');
  │     }
  │  }
```

---

## 7. Error Handling — WebSocket Connection Lost

### Scenario: Backend Crashed

```
Backend server cai inesperadamente
  │
  └─ WebSocket connection drops

Frontend WebSocket Handler:
  │
  ├─ onclose event
  │
  ├─ connectionState.set('closed')
  ├─ onError$.next('WebSocket connection lost')
  │
  └─ reconnectWithBackoff()
     │
     ├─ 1s wait → try
     ├─ 2s wait → try
     ├─ 4s wait → try
     │ ... (até 32s)
     │
     └─ Quando backend volta online:
        │
        └─ Reconectar
           │
           └─ UI mostra "Reconnected" (message desaparece)

Frontend UI Feedback:

Durante desconexão:
  │
  ├─ connectionState() === 'closed'
  │
  └─ Mostrar badge: "Connecting..." ou "Connection Lost"
     Dados antigos permanecem visíveis (stale)

Após reconexão:
  │
  ├─ connectionState() === 'open'
  │
  ├─ Recebe snapshots (cameras.updated + system.status)
  │
  └─ UI atualiza com dados frescos
```

---

## 8. Manual Sync Button

### User clica "Sync Now"

```
UI Button:
  │
  └─ <button (click)="onSyncNow()">Sync Now</button>

Component:
  │
  └─ onSyncNow(): void {
       │
       ├─ this.isSyncing = true; // Desabilitar botão
       │
       └─ this.api.sync().subscribe({
            next: (cameras) => {
              this.cameras.set(cameras);
              this.isSyncing = false;
              console.log('Sync complete');
            },
            error: (error) => {
              console.error('Sync failed:', error);
              this.isSyncing = false;
            }
          })
     }

Backend:
  │
  └─ POST /api/v1/sync
     │
     ├─ orch.SyncOnce() — executar sync imediatamente
     │
     └─ Retornar []Camera atualizado

Frontend Recebe:
  │
  └─ cameras.set([...])
     │
     └─ UI atualiza
```

---

## 9. Multi-Client Synchronization

### Cenário: 2 Operadores

```
Operador 1 (Browser A)
  │
  ├─ WebSocket conectado
  ├─ cameras signal
  └─ systemStatus signal

Operador 2 (Browser B)
  │
  ├─ WebSocket conectado
  ├─ cameras signal
  └─ systemStatus signal

Cenário:

Operador 1 clica "Set Live" para camera1
  │
  ├─ HTTP POST /api/v1/cameras/camera1/live
  │
  └─ Backend:
     ├─ SetLive('camera1')
     ├─ hub.Publish('system.status', {...liveCameraId: camera1})
     │
     └─ Broadcast para TODOS clients:
        │
        ├─ Client A (Operador 1)
        │  └─ systemStatus.set({...liveCameraId: camera1})
        │     └─ UI atualiza (camera1 destacado)
        │
        └─ Client B (Operador 2)
           └─ systemStatus.set({...liveCameraId: camera1})
              └─ UI atualiza (camera1 destacado)

Resultado:
  Ambos operadores veem estado sincronizado em tempo real.
```

---

## 10. Graceful Shutdown (Backend)

### Backend Fecha

```
Backend server recebe SIGTERM
  │
  ├─ apiServer.CloseAllWS()
  │  └─ Fecha todas conexões WebSocket
  │     └─ Envia close frame (1000 Normal Closure)

Frontend WebSocket Listener:
  │
  ├─ onclose event (code: 1000)
  │
  ├─ connectionState.set('closed')
  │
  ├─ onError$.next('Connection closed')
  │
  └─ reconnectWithBackoff()
     │
     └─ Tenta reconectar
        └─ Falha (backend offline)
           │
           └─ UI mostra "Attempting to reconnect..."

Operador aguarda backend voltar
  │
  └─ Quando backend sobe:
     │
     └─ Frontend reconecta
        │
        └─ Snapshots recebidos, UI atualiza
```

---

## 11. Browser Tab Closed

### Usuario fecha a aba

```
AppComponent.ngOnDestroy() (se implementado)
  │
  ├─ this.ws.disconnect()
  │  └─ WebSocket.close()
  │
  ├─ Unsubscribe de Observables (automático com takeUntilDestroyed)
  │
  └─ Cleanup de resources

Browser:
  │
  └─ Desaloca memória, fecha conexões
```

---

## 12. Network Transition (Mobile)

### WiFi → Mobile Data

```
Enquanto conectado via WiFi:
  │
  └─ WebSocket aberto, dados fluindo

User move para área sem WiFi
  │
  ├─ WiFi desconecta
  │
  ├─ Browser tenta manter WebSocket
  │  └─ Fails (sem conectividade)
  │
  ├─ connectionState.set('closed')
  │
  └─ reconnectWithBackoff()
     │
     └─ Aguarda Mobile Data conectar
        │
        └─ Após ~5-10s, Mobile Data está ativo
           │
           └─ Retry tenta conectar
              │
              └─ Sucesso!
                 └─ Reconectado
                    └─ Snapshots atualizam UI
```

---

## 13. Data Flow Summary

```
User Action / Backend Event
      │
      ├─ HTTP Request (sync)
      │  ├─ ApiTokenInterceptor (adicionar token)
      │  ├─ Backend processing
      │  └─ Resposta imediata (HTTP 200/4xx/5xx)
      │
      └─ WebSocket Broadcast (async)
         ├─ Subscription em onMessage$
         ├─ Update signal
         └─ UI re-renderiza (change detection)

Resultado Final:
  UI sempre reflete estado real do backend.
  Múltiplos clientes sincronizados.
  Reconexão automática após interrupção.
```
