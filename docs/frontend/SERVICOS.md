# Serviços do Frontend

Descrição detalhada de cada serviço, suas responsabilidades e como se integram.

---

## 1. API Service

**Arquivo**: `src/app/core/api.service.ts`

### Responsabilidade

Fornecer interface tipada para requisições HTTP ao backend.

### Interface Pública

```typescript
@Injectable({ providedIn: 'root' })
export class ApiService {
  getCameras(): Observable<Camera[]>
  getStatus(): Observable<SystemStatus>
  setLive(cameraId: string): Observable<SystemStatus>
  sync(): Observable<Camera[]>
}
```

### Métodos

#### getCameras()

```typescript
getCameras(): Observable<Camera[]>
```

**HTTP:**
```
GET /api/v1/cameras
X-Api-Token: dev-token
```

**Retorno:**
```typescript
Observable<Camera[]>

// Exemplo:
[
  {
    id: "camera1",
    name: "camera1",
    sourceUrl: "rtmp://localhost:1935/live/camera1",
    status: "online",
    obsSourceCreated: true,
    isLive: true,
    lastSeenAt: "2025-07-19T15:30:45Z"
  }
]
```

**Uso:**
```typescript
this.api.getCameras().subscribe(cameras => {
  this.cameras.set(cameras);
});
```

**Tipicamente chamado:**
- Ao iniciar a aplicação (snapshot inicial)
- Manualmente via "Sync" button

#### getStatus()

```typescript
getStatus(): Observable<SystemStatus>
```

**HTTP:**
```
GET /api/v1/status
X-Api-Token: dev-token
```

**Retorno:**
```typescript
Observable<SystemStatus>

// Exemplo:
{
  obsConnected: true,
  mediaServerConnected: true,
  streaming: true,
  activeSceneName: "Program",
  liveCameraId: "camera1"
}
```

**Uso:**
```typescript
this.api.getStatus().subscribe(status => {
  this.systemStatus.set(status);
});
```

#### setLive()

```typescript
setLive(cameraId: string): Observable<SystemStatus>
```

**HTTP:**
```
POST /api/v1/cameras/{id}/live
X-Api-Token: dev-token
```

**Retorno:**
```typescript
Observable<SystemStatus>

// Sucesso: SystemStatus atualizado
// Erro: 404 camera not found, 400 offline
```

**Uso:**
```typescript
this.api.setLive("camera1").subscribe({
  next: (status) => {
    // Sucesso (signal já foi atualizado via WebSocket)
    console.log("Camera set to live:", status.liveCameraId);
  },
  error: (error) => {
    // Erro
    if (error.status === 404) {
      console.error("Camera not found");
    } else if (error.status === 400) {
      console.error("Camera is offline");
    }
  }
});
```

#### sync()

```typescript
sync(): Observable<Camera[]>
```

**HTTP:**
```
POST /api/v1/sync
X-Api-Token: dev-token
```

**Retorno:**
```typescript
Observable<Camera[]>

// Força sincronização imediata, retorna lista atualizada
```

**Uso:**
```typescript
this.api.sync().subscribe(cameras => {
  this.cameras.set(cameras);
  console.log("Sync complete");
});
```

**Tipicamente chamado:**
- Via botão "Sync Now" no painel
- Debug: verificar descoberta de câmeras

---

## 2. API Token Interceptor

**Arquivo**: `src/app/core/api-token.interceptor.ts`

### Responsabilidade

Adicionar header `X-Api-Token` a todas requisições HTTP automaticamente.

### Interface Pública

```typescript
@Injectable()
export class ApiTokenInterceptor implements HttpInterceptor {
  intercept(
    req: HttpRequest<unknown>,
    next: HttpHandler
  ): Observable<HttpEvent<unknown>>
}
```

### Fluxo

```
API Service faz requisição
  │
  └─→ HttpClient
       │
       └─→ Interceptor.intercept()
            │
            ├─ req.clone({
            │   setHeaders: {
            │     'X-Api-Token': 'dev-token'
            │   }
            │ })
            │
            └─→ next.handle(req com token)
                 │
                 └─→ Retornar para API Service
```

### Token Hardcoded

Para PoC, token é hardcoded como `'dev-token'`:

```typescript
const TOKEN = 'dev-token';

const newReq = req.clone({
  setHeaders: {
    'X-Api-Token': TOKEN
  }
});
```

**Em produção:**
```typescript
// Obter de localStorage, sessionStorage, etc.
const token = localStorage.getItem('api_token') || 'dev-token';
```

### Registro em app.config.ts

```typescript
export const appConfig: ApplicationConfig = {
  providers: [
    provideHttpClient(
      withInterceptors([
        (req, next) => {
          const newReq = req.clone({
            setHeaders: { 'X-Api-Token': 'dev-token' }
          });
          return next(newReq);
        }
      ])
    ),
    // ...
  ]
};
```

---

## 3. WebSocket Service

**Arquivo**: `src/app/core/websocket.service.ts`

### Responsabilidade

Gerenciar conexão WebSocket, auto-reconexão, e distribuição de eventos em tempo real.

### Interface Pública

```typescript
@Injectable({ providedIn: 'root' })
export class WebSocketService {
  // State
  connectionState = signal<ConnectionState>('connecting');
  
  // Events
  onMessage$: Observable<WsEnvelope>;
  onError$: Observable<string>;
  
  // Methods
  connect(url: string): void;
  disconnect(): void;
}
```

### Propriedades

#### connectionState

Signal que rastreia estado da conexão.

```typescript
connectionState = signal<ConnectionState>('connecting');

// Possíveis valores:
type ConnectionState = 'connecting' | 'open' | 'closed';

// Uso na view:
<div [class.connected]="ws.connectionState() === 'open'">
  Status: {{ ws.connectionState() }}
</div>
```

#### onMessage$

Observable que emite eventos vindos do servidor.

```typescript
onMessage$: Observable<WsEnvelope>;

// Exemplo:
ws.onMessage$.subscribe(envelope => {
  switch (envelope.type) {
    case 'cameras.updated':
      console.log('Cameras updated:', envelope.payload);
      break;
    case 'system.status':
      console.log('System status:', envelope.payload);
      break;
  }
});
```

#### onError$

Observable que emite mensagens de erro.

```typescript
onError$: Observable<string>;

// Exemplo:
ws.onError$.subscribe(error => {
  console.error('WebSocket error:', error);
});
```

### Métodos

#### connect()

```typescript
connect(url: string): void
```

Inicia conexão WebSocket com reconexão automática.

**Fluxo:**
```
1. Tentar conectar a url
2. Se sucesso:
   - connectionState → 'open'
   - Iniciar reader para eventos
3. Se falha:
   - connectionState → 'closed'
   - Iniciar retry loop com exponential backoff
```

**Chamado em:**
- `AppComponent.ngOnInit()`

**Exemplo:**
```typescript
ngOnInit() {
  this.ws.connect('ws://localhost:8080/api/v1/ws');
}
```

#### disconnect()

```typescript
disconnect(): void
```

Fecha conexão WebSocket explicitamente.

**Usado em:**
- Cleanup (ngOnDestroy)
- Manual disconnect via UI button

---

### Reconexão Automática

Quando a conexão cai ou falha em conectar:

```
1s wait → Try connect → Fail
2s wait → Try connect → Fail
4s wait → Try connect → Fail
8s wait → Try connect → Fail
16s wait → Try connect → Fail
32s wait → Try connect → Fail
32s wait → ... (máximo 32s)
```

**Configurável:**
```typescript
private readonly MIN_RECONNECT_DELAY = 1000;
private readonly MAX_RECONNECT_DELAY = 32000;

private getReconnectDelay(): number {
  return Math.min(
    Math.pow(2, this.reconnectAttempts) * this.MIN_RECONNECT_DELAY,
    this.MAX_RECONNECT_DELAY
  );
}
```

---

### Eventos Recebidos

#### cameras.updated

```typescript
envelope = {
  type: 'cameras.updated',
  payload: [
    {
      id: 'camera1',
      name: 'camera1',
      sourceUrl: 'rtmp://localhost:1935/live/camera1',
      status: 'online',
      obsSourceCreated: true,
      isLive: true,
      lastSeenAt: '2025-07-19T15:30:45Z'
    },
    // ...
  ]
}
```

**Emitido quando:**
- Cliente conecta (snapshot)
- Nova câmera detectada
- Câmera muda de status
- Câmera é removida

#### system.status

```typescript
envelope = {
  type: 'system.status',
  payload: {
    obsConnected: true,
    mediaServerConnected: true,
    streaming: true,
    activeSceneName: 'Program',
    liveCameraId: 'camera1'
  }
}
```

**Emitido quando:**
- Cliente conecta (snapshot)
- A cada sync loop do backend
- Serviço reconecta/desconecta

#### error

```typescript
envelope = {
  type: 'error',
  payload: {
    message: 'Some error message'
  }
}
```

---

### Implementação Interna

```typescript
@Injectable({ providedIn: 'root' })
export class WebSocketService {
  connectionState = signal<ConnectionState>('connecting');
  
  private wsSubject: Subject<WsEnvelope>;
  onMessage$: Observable<WsEnvelope>;
  
  private errorSubject: Subject<string>;
  onError$: Observable<string>;
  
  private ws: WebSocket | null = null;
  private reconnectAttempts = 0;

  connect(url: string): void {
    this.wsUrl = url;
    this.openConnection();
  }

  private openConnection(): void {
    try {
      this.ws = new WebSocket(this.wsUrl);
      
      this.ws.onopen = () => {
        this.connectionState.set('open');
        this.reconnectAttempts = 0;
        this.wsSubject.next({ type: 'connected', payload: {} });
      };
      
      this.ws.onmessage = (event) => {
        const envelope = JSON.parse(event.data) as WsEnvelope;
        this.wsSubject.next(envelope);
      };
      
      this.ws.onerror = (event) => {
        this.connectionState.set('closed');
        this.errorSubject.next(`WebSocket error: ${event}`);
        this.reconnectWithBackoff();
      };
      
      this.ws.onclose = () => {
        this.connectionState.set('closed');
        this.reconnectWithBackoff();
      };
    } catch (error) {
      this.connectionState.set('closed');
      this.errorSubject.next(`Failed to connect: ${error}`);
      this.reconnectWithBackoff();
    }
  }

  private reconnectWithBackoff(): void {
    const delay = this.getReconnectDelay();
    setTimeout(() => {
      this.reconnectAttempts++;
      this.openConnection();
    }, delay);
  }

  disconnect(): void {
    if (this.ws) {
      this.ws.close();
      this.ws = null;
    }
    this.connectionState.set('closed');
  }
}
```

---

## 4. Tipos / Models

**Arquivo**: `src/app/core/models.ts`

Tipos TypeScript compartilhados entre serviços e componentes.

```typescript
// Camera
export type CameraStatus = 'online' | 'offline';

export interface Camera {
  id: string;
  name: string;
  sourceUrl: string;
  status: CameraStatus;
  obsSourceCreated: boolean;
  isLive: boolean;
  lastSeenAt: string;
}

// System Status
export interface SystemStatus {
  obsConnected: boolean;
  mediaServerConnected: boolean;
  streaming: boolean;
  activeSceneName: string;
  liveCameraId: string;
}

// WebSocket
export type WsEventType = 'cameras.updated' | 'system.status' | 'error';

export interface WsEnvelope<T = unknown> {
  type: WsEventType;
  payload: T;
}

export interface ErrorPayload {
  message: string;
}

// Connection
export type ConnectionState = 'connecting' | 'open' | 'closed';
```

**Mantém sincronização com backend** — tipos espelham exatamente Go models.

---

## Fluxo de Inicialização Completo

### AppComponent.ngOnInit()

```typescript
ngOnInit(): void {
  // 1. Conectar WebSocket
  const wsUrl = `${environment.apiBaseUrl.replace('http', 'ws')}/ws`;
  this.ws.connect(wsUrl);

  // 2. Carregar snapshot inicial (HTTP)
  this.api.getCameras().subscribe(cameras => {
    this.cameras.set(cameras);
  });
  
  this.api.getStatus().subscribe(status => {
    this.systemStatus.set(status);
  });

  // 3. Subscrever a eventos WebSocket
  this.ws.onMessage$.subscribe(envelope => {
    switch (envelope.type) {
      case 'cameras.updated':
        this.cameras.set(envelope.payload);
        break;
      case 'system.status':
        this.systemStatus.set(envelope.payload);
        break;
      case 'error':
        console.error('WebSocket error:', envelope.payload);
        // Mostrar toast de erro
        break;
    }
  });
  
  // 4. Monitorar erro de WebSocket
  this.ws.onError$.subscribe(error => {
    console.error('WebSocket connection error:', error);
    // Mostrar "Attempting to reconnect..."
  });
}
```

---

## Exemplo de Fluxo: Operador Seleciona Câmera

```
CameraCardComponent.selectLive()
  │
  ├─ this.setLive.emit(this.camera.id)
  │
  └─ CameraGridComponent.onSetLive()
     │
     └─ this.api.setLive(cameraId).subscribe({
          next: (status) => {
            // Sucesso
            // UI já foi atualizada via WebSocket
            console.log("Set live succeeded");
          },
          error: (error) => {
            // Erro
            if (error.status === 404) {
              alert("Camera not found");
            } else if (error.status === 400) {
              alert("Camera is offline");
            } else {
              alert(`Error: ${error.message}`);
            }
          }
        })

Simultaneamente:

1. HTTP POST retorna
2. Backend pub/sub dispara evento
3. WebSocket listener recebe "system.status"
4. this.systemStatus.set(new status)
5. UI re-renderiza (signal reactivity)
6. Card que estava marcado como live se desmarca
7. Novo card marca como live
```

---

## Tratamento de Erro

### HTTP Error

```typescript
// ApiService
this.api.setLive(cameraId).subscribe({
  next: (status) => { /* sucesso */ },
  error: (error: HttpErrorResponse) => {
    // error.status: 404, 400, 500, etc.
    // error.message: string
    // error.error: any (JSON response do server)
  }
});
```

### WebSocket Error

```typescript
// AppComponent
this.ws.onError$.subscribe(error => {
  // String mensagem de erro
  console.error(error);
  // Mostrar visual feedback
});
```

---

## Testes

**Estrutura:**
- `*.spec.ts` ao lado de cada serviço
- Usa Jasmine + Karma
- Mock HttpClient via `HttpClientTestingModule`
- Mock WebSocket (criar mock local)

**Executar:**
```bash
npm test
```
