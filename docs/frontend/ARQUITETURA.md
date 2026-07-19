# Arquitetura — Frontend (Angular)

## Visão Geral

O frontend é uma aplicação Angular que fornece um painel de controle web para o operador gerenciar câmeras e transmissão ao vivo. Comunica-se com o backend via HTTP REST e WebSocket, consumindo dados de câmeras e estado do sistema em tempo real.

## Princípios de Design

1. **Sinal-based** — Usa Angular Signals para reatividade
2. **Services centralizados** — API e WebSocket como serviços injetáveis
3. **Componentes funcionais** — Standalone components, sem módulos
4. **Tipagem forte** — TypeScript com modelos espelhados do backend
5. **Auto-reconexão** — WebSocket se reconecta automaticamente com backoff

## Estrutura de Diretórios

```
frontend/src/app/
├── app.ts                          Componente raiz
├── app.config.ts                   Configuração Angular
├── core/
│   ├── models.ts                   Tipos (Camera, SystemStatus)
│   ├── api.service.ts              Serviço HTTP
│   ├── api-token.interceptor.ts    Interceptor (token)
│   ├── websocket.service.ts        Gerenciador WebSocket
│   └── (spec files)
└── features/
    ├── camera-grid/
    │   ├── camera-grid.component.ts       Grid principal
    │   ├── camera-card.component.ts       Card individual
    │   └── (spec files)
    └── control-bar/
        ├── control-bar.component.ts       Barra de controle
        └── (spec files)
```

## Camadas

### 1. Models (`core/models.ts`)

Tipos TypeScript espelhando os contratos do backend.

```typescript
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

export interface SystemStatus {
  obsConnected: boolean;
  mediaServerConnected: boolean;
  streaming: boolean;
  activeSceneName: string;
  liveCameraId: string;
}

export type WsEventType = 'cameras.updated' | 'system.status' | 'error';

export interface WsEnvelope<T = unknown> {
  type: WsEventType;
  payload: T;
}
```

**Mantém sincronização com backend** via JSON serialization.

---

### 2. API Service (`core/api.service.ts`)

Gerencia requisições HTTP para o backend.

```typescript
@Injectable({ providedIn: 'root' })
export class ApiService {
  private readonly http = inject(HttpClient);
  private readonly baseUrl = environment.apiBaseUrl;

  getCameras(): Observable<Camera[]>
  getStatus(): Observable<SystemStatus>
  setLive(cameraId: string): Observable<SystemStatus>
  sync(): Observable<Camera[]>
}
```

**Responsabilidades:**
- Construir URLs
- Fazer requisições HTTP
- Retornar Observables
- (Interceptor adiciona token automaticamente)

---

### 3. API Token Interceptor (`core/api-token.interceptor.ts`)

Interceptor HTTP que adiciona `X-Api-Token` header a todas as requisições.

```typescript
@Injectable()
export class ApiTokenInterceptor implements HttpInterceptor {
  intercept(req: HttpRequest<unknown>, next: HttpHandler): Observable<HttpEvent<unknown>> {
    const newReq = req.clone({
      setHeaders: {
        'X-Api-Token': 'dev-token'
      }
    });
    return next.handle(newReq);
  }
}
```

**Configurado em** `app.config.ts` via `HTTP_INTERCEPTORS`.

---

### 4. WebSocket Service (`core/websocket.service.ts`)

Gerencia conexão WebSocket, auto-reconexão e distribuição de eventos.

```typescript
@Injectable({ providedIn: 'root' })
export class WebSocketService {
  connectionState = signal<ConnectionState>('connecting');
  
  onMessage$: Observable<WsEnvelope>;
  onError$: Observable<string>;
  
  connect(url: string): void
  disconnect(): void
  private reconnectWithBackoff(): void
  private openConnection(): void
}
```

**Características:**
- Tenta conectar ao startup
- Auto-reconexão com exponential backoff
- Expõe Observables para eventos de mensagem e erro
- Gerencia pings para manter conexão viva

**Algoritmo de Reconexão:**
```
1s → 2s → 4s → 8s → 16s → 32s (máximo)
```

---

### 5. Componentes

#### CameraGridComponent

Renderiza grid de cards de câmeras.

```typescript
@Component({
  selector: 'app-camera-grid',
  standalone: true,
  imports: [CommonModule, CameraCardComponent],
  template: `
    <div class="camera-grid">
      @for (camera of cameras(); track camera.id) {
        <app-camera-card
          [camera]="camera"
          (setLive)="onSetLive($event)"
        ></app-camera-card>
      }
    </div>
  `
})
export class CameraGridComponent implements OnInit {
  cameras = signal<Camera[]>([]);
  
  constructor(private api: ApiService, private ws: WebSocketService) {}
  
  ngOnInit(): void {
    // Carregar câmeras iniciais
    // Subscrever a eventos de atualização
  }
  
  onSetLive(cameraId: string): void {
    // POST /api/v1/cameras/{id}/live
  }
}
```

**Usos:**
- Exibir lista de câmeras em grid
- Reagir a eventos WebSocket (cameras.updated)
- Passar eventos de click para cards

#### CameraCardComponent

Card individual renderizando uma câmera.

```typescript
@Component({
  selector: 'app-camera-card',
  standalone: true,
  inputs: ['camera'],
  outputs: ['setLive'],
  template: `
    <div [class.live]="camera.isLive">
      <h3>{{ camera.name }}</h3>
      <p [class.online]="camera.status === 'online'">
        {{ camera.status }}
      </p>
      <button (click)="selectLive()" [disabled]="camera.status === 'offline'">
        Set Live
      </button>
    </div>
  `,
  styles: [`
    .card { border: 2px solid #ddd; padding: 1rem; }
    .card.live { border-color: #0f0; background: #f0fff0; }
    .online { color: green; }
    [class.online]:not(.online) { color: red; }
  `]
})
export class CameraCardComponent {
  camera = input.required<Camera>();
  setLive = output<string>();
  
  selectLive(): void {
    this.setLive.emit(this.camera.id);
  }
}
```

**Responsabilidades:**
- Renderizar informações da câmera
- Aplicar estilos baseado em status
- Emitir evento ao clicar "Set Live"

#### ControlBarComponent

Barra de controle mostrando status do sistema.

```typescript
@Component({
  selector: 'app-control-bar',
  standalone: true,
  template: `
    <div class="control-bar">
      <div [class.connected]="status().obsConnected">
        OBS: {{ status().obsConnected ? 'Connected' : 'Disconnected' }}
      </div>
      <div [class.connected]="status().mediaServerConnected">
        Media Server: {{ status().mediaServerConnected ? 'Connected' : 'Disconnected' }}
      </div>
      <div [class.streaming]="status().streaming">
        Streaming: {{ status().streaming ? 'Active' : 'Inactive' }}
      </div>
    </div>
  `
})
export class ControlBarComponent {
  status = signal<SystemStatus>({
    obsConnected: false,
    mediaServerConnected: false,
    streaming: false,
    activeSceneName: '',
    liveCameraId: ''
  });
}
```

**Responsabilidades:**
- Mostrar status de conexão dos serviços
- Visual feedback para operador

---

### 6. Componente Raiz (`app.ts`)

```typescript
@Component({
  selector: 'app-root',
  standalone: true,
  imports: [CameraGridComponent, ControlBarComponent],
  template: `
    <div class="container">
      <h1>Live Orchestrator</h1>
      <app-control-bar [status]="systemStatus()"></app-control-bar>
      <app-camera-grid [cameras]="cameras()"></app-camera-grid>
    </div>
  `
})
export class AppComponent implements OnInit {
  cameras = signal<Camera[]>([]);
  systemStatus = signal<SystemStatus>({...});
  
  constructor(
    private api: ApiService,
    private ws: WebSocketService
  ) {}
  
  ngOnInit(): void {
    // Conectar WebSocket
    // Carregar snapshot inicial
    // Subscrever a eventos
  }
}
```

---

## Fluxo de Inicialização (Startup)

```
bootstrap(AppComponent, appConfig)
  │
  ├─ Injetar providers:
  │  ├─ ApiService
  │  ├─ WebSocketService
  │  ├─ HTTP_INTERCEPTORS → ApiTokenInterceptor
  │
  ├─ AppComponent.ngOnInit():
  │  │
  │  ├─ WebSocketService.connect('ws://localhost:8080/api/v1/ws')
  │  │  └─ Tentar conectar (com retry)
  │  │
  │  ├─ ApiService.getCameras().subscribe(...)
  │  │  └─ GET /api/v1/cameras
  │  │     └─ Atualizar signal cameras
  │  │
  │  ├─ ApiService.getStatus().subscribe(...)
  │  │  └─ GET /api/v1/status
  │  │     └─ Atualizar signal systemStatus
  │  │
  │  └─ WebSocketService.onMessage$.subscribe(...):
  │     │
  │     ├─ Se tipo 'cameras.updated':
  │     │  └─ Atualizar signal cameras
  │     │
  │     ├─ Se tipo 'system.status':
  │     │  └─ Atualizar signal systemStatus
  │     │
  │     └─ Se tipo 'error':
  │        └─ Mostrar toast/alert
  │
  └─ UI Renderizada
```

---

## Reatividade com Signals

O frontend usa **Angular Signals** para reatividade granular.

### Signals Principais

```typescript
// AppComponent
cameras = signal<Camera[]>([]);
systemStatus = signal<SystemStatus>({...});

// WebSocketService
connectionState = signal<ConnectionState>('connecting');
```

### Mudanças Propagam Automaticamente

```typescript
// Na view:
@for (camera of cameras(); track camera.id) {
  <app-camera-card [camera]="camera"></app-camera-card>
}

// Quando HttpClient retorna:
this.cameras.set(newCameras);
// Angular detecta → Re-renderiza apenas o necessário
```

---

## Configuração de Ambiente

**Arquivo**: `src/environments/environment.ts`

```typescript
export const environment = {
  apiBaseUrl: 'http://localhost:8080/api/v1'
};
```

- **Dev**: `http://localhost:8080/api/v1`
- **Prod**: Substituir pela URL do servidor real

---

## Interceptor de Token

O `ApiTokenInterceptor` adiciona token a **todas** requisições HTTP (GET, POST, etc.).

```typescript
// Antes:
GET /api/v1/cameras

// Depois do interceptor:
GET /api/v1/cameras
X-Api-Token: dev-token
```

**Token hardcoded** para PoC; em produção seria dinâmico (OAuth2, etc.).

---

## WebSocket — Evento-Driven

O frontend não poll o backend; ao invés disso, **ouve eventos via WebSocket**.

```typescript
// WebSocketService inicia connection
// Quando evento chega:
onMessage$.subscribe(envelope => {
  switch (envelope.type) {
    case 'cameras.updated':
      this.cameras.set(envelope.payload);
      break;
    case 'system.status':
      this.systemStatus.set(envelope.payload);
      break;
  }
});
```

**Vantagens:**
- Atualização em tempo real
- Sem polling (economiza recursos)
- Estado sincronizado entre múltiplos clientes

---

## Tratamento de Desconexão

```typescript
// WebSocketService
private reconnectWithBackoff(): void {
  this.connectionState.set('connecting');
  
  this.retryAttempts = 0;
  this.maxRetries = 10;
  
  const retry = () => {
    if (this.retryAttempts >= this.maxRetries) {
      return; // Dar up (user pode clicar botão para retry)
    }
    
    const delay = Math.pow(2, this.retryAttempts) * 1000; // 1s, 2s, 4s, ...
    
    setTimeout(() => {
      this.openConnection();
      this.retryAttempts++;
    }, delay);
  };
  
  retry();
}
```

**Comportamento do Frontend:**
- Tenta conectar ao startup
- Se falhar → exponential backoff (até ~10 tentativas)
- Mostra "Connecting..." na UI
- Se reconectar → Snapshot inicial (cameras + status)

---

## Tratamento de Erro na API

```typescript
// Exemplo no CameraGridComponent
onSetLive(cameraId: string): void {
  this.api.setLive(cameraId).subscribe({
    next: (status) => {
      // Sucesso — signal já foi atualizado via WebSocket
      // Apenas feedback visual (toast)
    },
    error: (error) => {
      // Erro: 404 camera not found, 400 offline, etc.
      console.error('Failed to set live:', error);
      // Mostrar toast de erro
    }
  });
}
```

---

## Extensibilidade

### Adicionar Novo Componente

1. Criar novo componente em `features/`
2. Importar em `app.ts`
3. Injetar `ApiService` ou `WebSocketService` conforme necessário

### Adicionar Novo Endpoint

1. Adicionar método em `ApiService`
2. Retornar `Observable<T>`
3. Subscrever em componente

### Adicionar Novo Evento WebSocket

1. Adicionar tipo em `WsEventType`
2. Manipular em `WebSocketService.onMessage$.subscribe()`
3. Atualizar signal apropriado

---

## Build & Deployment

```bash
# Development
npm start
# Abre http://localhost:4200

# Production
npm run build
# Gera dist/frontend/
```

---

## Dependências Principais

- **Angular 21** — Framework
- **TypeScript** — Linguagem
- **RxJS** — Reactive programming
- **HttpClientModule** — HTTP requests
