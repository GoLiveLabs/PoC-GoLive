# Documentação do Frontend (Angular)

Painel do operador para o orquestrador de câmeras. Mostra as câmeras detectadas
em tempo real, os indicadores de saúde do sistema, e permite trocar a câmera que
está no ar com um clique.

## Índice

- [Visão geral](#visão-geral)
- [Stack e convenções](#stack-e-convenções)
- [Estrutura de pastas](#estrutura-de-pastas)
- [Fluxo de dados](#fluxo-de-dados)
- [Camada core](#camada-core)
  - [models.ts](#coremodelsts)
  - [ApiService](#coreapiservicets)
  - [apiTokenInterceptor](#coreapi-tokeninterceptorts)
  - [WebSocketService](#corewebsocketservicets)
- [Componentes](#componentes)
  - [App (raiz)](#app-raiz)
  - [ControlBarComponent](#controlbarcomponent)
  - [CameraGridComponent](#cameragridcomponent)
  - [CameraCardComponent](#cameracardcomponent)
- [Ambientes e configuração](#ambientes-e-configuração)
- [Estados visuais](#estados-visuais)
- [Testes](#testes)
- [Comandos](#comandos)
- [Limitações e evolução prevista](#limitações-e-evolução-prevista)

---

## Visão geral

```
┌────────────────────────────────────────────────────────────┐
│ ControlBar   ● Painel  ● OBS  ● MediaMTX   No ar: camera2  │  ← indicadores + "Sincronizar agora"
├────────────────────────────────────────────────────────────┤
│ [toast de erro / banner de desconexão, quando aplicável]   │
├────────────────────────────────────────────────────────────┤
│  ┌──────────┐  ┌──────────┐  ┌──────────┐                  │
│  │ camera1  │  │ camera2  │  │ camera3  │   ← CameraGrid   │
│  │ ONLINE   │  │ NO AR    │  │ OFFLINE  │                  │
│  │ [no ar]  │  │ [——————] │  │ [——————] │                  │
│  └──────────┘  └──────────┘  └──────────┘                  │
└────────────────────────────────────────────────────────────┘
```

O frontend **nunca fala diretamente com o OBS ou o MediaMTX** — toda comunicação
passa pelo backend via REST (ações) e WebSocket (estado em tempo real).

## Stack e convenções

| Item | Escolha |
|---|---|
| Angular | 21 (standalone components, signals, control flow `@if`/`@for`) |
| Estado | **Signals** — nada de NgRx/BehaviorSubject para estado de UI |
| HTTP | `HttpClient` com functional interceptor |
| Estilo | CSS puro por componente + `styles.css` global (tema escuro) |
| Testes | Vitest (padrão do Angular CLI 21 — ver `DECISIONS.md`) |
| Idioma | Código/nomes em inglês; textos de interface em **pt-BR** |

Convenções aplicadas:

- Componentes **standalone**, com `input()`/`output()` funcionais (não decorators).
- **Nenhuma lógica de negócio em componentes** — componentes de feature são
  "burros": recebem dados por input e emitem eventos por output. Quem decide é o
  `App` (orquestra chamadas) e os services.
- Sem routing — aplicação de página única.

## Estrutura de pastas

```
frontend/src/
├── main.ts                          # bootstrapApplication(App, appConfig)
├── index.html
├── styles.css                       # reset + tema escuro global
├── environments/
│   └── environment.ts               # apiBaseUrl, wsUrl, apiToken
└── app/
    ├── app.ts / app.html / app.css  # componente raiz (layout + wiring)
    ├── app.config.ts                # providers: HttpClient + interceptor
    ├── app.spec.ts
    ├── core/
    │   ├── models.ts                # Camera, SystemStatus, WsEnvelope, ...
    │   ├── api.service.ts           # chamadas REST
    │   ├── api-token.interceptor.ts # injeta X-Api-Token
    │   ├── websocket.service.ts     # WS + signals de estado
    │   └── websocket.service.spec.ts
    └── features/
        ├── camera-grid/
        │   ├── camera-grid.component.{ts,html,css}
        │   ├── camera-card.component.{ts,html,css}
        │   └── camera-card.component.spec.ts
        └── control-bar/
            └── control-bar.component.{ts,html,css}
```

## Fluxo de dados

```
                    (estado, tempo real)
  Backend ══ WebSocket ══▶ WebSocketService ──signals──▶ App ──inputs──▶ ControlBar
                                                          │              CameraGrid ─▶ CameraCard
                    (ações, request/response)             │
  Backend ◀══ REST (ApiService) ◀───── outputs (goLive, sync) ◀──────────┘
```

Ponto importante do desenho: **as ações REST não atualizam o estado local**.
`App.onGoLive()` chama `POST /cameras/{id}/live` e ignora o corpo da resposta —
a UI só muda quando o backend publica `cameras.updated`/`system.status` no
WebSocket. Isso mantém uma única fonte de verdade (o backend) e garante que
todos os operadores conectados vejam o mesmo estado, sem reconciliação manual.

## Camada core

### `core/models.ts`

Espelha 1:1 os contratos JSON do backend:

```ts
type CameraStatus = 'online' | 'offline';

interface Camera {
  id: string;
  name: string;
  sourceUrl: string;
  status: CameraStatus;
  obsSourceCreated: boolean;
  isLive: boolean;
  lastSeenAt: string;          // ISO 8601
}

interface SystemStatus {
  obsConnected: boolean;
  mediaServerConnected: boolean;
  streaming: boolean;
  activeSceneName: string;
  liveCameraId: string;        // '' = nenhuma
}

type WsEventType = 'cameras.updated' | 'system.status' | 'error';
interface WsEnvelope<T = unknown> { type: WsEventType; payload: T; }
type ConnectionState = 'connecting' | 'open' | 'closed';
```

### `core/api.service.ts`

Service fino sobre `HttpClient` — um método por rota:

| Método | Chamada |
|---|---|
| `getCameras()` | `GET /cameras` → `Observable<Camera[]>` |
| `getStatus()` | `GET /status` → `Observable<SystemStatus>` |
| `setLive(cameraId)` | `POST /cameras/{id}/live` → `Observable<SystemStatus>` |
| `sync()` | `POST /sync` → `Observable<Camera[]>` |

Sem cache, sem retry, sem estado — estado é responsabilidade do `WebSocketService`.

### `core/api-token.interceptor.ts`

Functional interceptor (`HttpInterceptorFn`) registrado em `app.config.ts` via
`provideHttpClient(withInterceptors([...]))`. Clona toda requisição cujo destino
começa com `environment.apiBaseUrl` e adiciona `X-Api-Token: <environment.apiToken>`.
Requisições para outros hosts passam intactas.

### `core/websocket.service.ts`

O coração do estado do painel. Expõe quatro signals **somente-leitura para os
componentes** (escritos apenas pelo próprio service):

| Signal | Tipo | Conteúdo |
|---|---|---|
| `cameras` | `signal<Camera[]>` | última lista completa recebida |
| `systemStatus` | `signal<SystemStatus \| null>` | último status (null até o snapshot) |
| `connectionState` | `signal<ConnectionState>` | estado do próprio WS com o backend |
| `lastError` | `signal<string \| null>` | mensagem do último evento `error` (pt-BR) |

**Conexão:** `connect()` abre `ws://.../ws?api_token=<token>` (token via query
string porque a API `WebSocket` do navegador não aceita headers customizados).

**Reconexão automática:** quando o socket fecha sem `disconnect()` manual,
reagenda `open()` com backoff exponencial **1s → 2s → 4s → ... → máx 30s**.
Ao reconectar com sucesso, o backoff reseta para 1s. O **re-snapshot é
automático**: o backend envia `cameras.updated` + `system.status` a todo cliente
recém-conectado, então o painel se re-hidrata sozinho após qualquer queda.

**Processamento de eventos** (`handleMessage`, público para permitir teste
unitário sem socket real):

```
'cameras.updated' → cameras.set(payload)
'system.status'   → systemStatus.set(payload)
'error'           → lastError.set(payload.message)
JSON inválido     → ignorado silenciosamente
```

`disconnect()` marca fechamento manual (suprime a reconexão), cancela o timer
pendente e fecha o socket.

## Componentes

### App (raiz)

`app.ts` — único componente com acesso aos services:

- `ngOnInit` → `ws.connect()`; `ngOnDestroy` → `ws.disconnect()`.
- `onGoLive(cameraId)` → `api.setLive(cameraId).subscribe()` (fire-and-forget;
  o estado volta pelo WS).
- `onSync()` → `api.sync().subscribe()`.
- `dismissError()` → limpa `ws.lastError`.

`app.html` — layout e wiring declarativo:

- `<app-control-bar>` no topo, recebendo `ws.systemStatus()` e
  `ws.connectionState()`.
- Toast de erro (`role="alert"`, fechável) quando `ws.lastError()` não é null.
- Banner de desconexão ("Sem conexão com o backend. Tentando reconectar...")
  sempre que `ws.connectionState() !== 'open'`.
- `<app-camera-grid>` com `ws.cameras()`.

### ControlBarComponent

`features/control-bar/` — componente de apresentação puro.

| Binding | Tipo | Uso |
|---|---|---|
| `systemStatus` | `input<SystemStatus \| null>` | indicadores OBS/MediaMTX/câmera no ar |
| `connectionState` | `input<ConnectionState>` | indicador "Painel: conectado/conectando/desconectado" |
| `sync` | `output<void>` | botão "Sincronizar agora" |

Cada indicador é um "dot" colorido (verde `--ok` / vermelho `--fail` / cinza
neutro) seguido do rótulo em pt-BR. O indicador "No ar" mostra o `liveCameraId`
ou "nenhuma câmera".

### CameraGridComponent

`features/camera-grid/camera-grid.component.*` — grade responsiva
(`grid-template-columns: repeat(auto-fill, minmax(240px, 1fr))`).

| Binding | Tipo |
|---|---|
| `cameras` | `input.required<Camera[]>` |
| `goLive` | `output<string>` (repassa o evento dos cards) |

Com lista vazia renderiza "Nenhuma câmera detectada ainda.". Itera com
`@for (...; track camera.id)`.

### CameraCardComponent

`features/camera-grid/camera-card.component.*` — card individual.

| Binding | Tipo |
|---|---|
| `camera` | `input.required<Camera>` |
| `goLive` | `output<string>` (emite `camera().id` no clique) |

Anatomia do card:

- **Header**: nome + badge de status (`Online` verde / `Offline` cinza).
- **Área de preview**: placeholder 16:9 com "Pré-visualização indisponível" —
  espaço reservado para o preview HLS de uma fase futura.
- **Badge "NO AR"**: sobreposto no canto quando `isLive`.
- **Botão "Colocar no ar"**: desabilitado quando `status === 'offline'` **ou**
  quando a câmera já é a live.
- Destaque visual forte quando live: borda + glow vermelhos
  (`.camera-card--live`).

## Ambientes e configuração

`src/environments/environment.ts`:

```ts
export const environment = {
  production: false,
  apiBaseUrl: 'http://localhost:8080/api/v1',
  wsUrl: 'ws://localhost:8080/api/v1/ws',
  apiToken: 'dev-token',
};
```

Para apontar para outro backend, altere esses três valores (o `apiToken` deve
bater com o `API_TOKEN` do backend). Não há build de produção configurado com
file replacement — é uma PoC de ambiente local.

## Estados visuais

| Estado | Aparência |
|---|---|
| Câmera online | badge verde "ONLINE", botão azul habilitado |
| Câmera offline | badge cinza "OFFLINE", botão desabilitado |
| Câmera no ar | borda/glow vermelhos, badge "NO AR", botão desabilitado |
| WS conectando/caído | banner amarelo fixo + dot vermelho no indicador "Painel" |
| Evento `error` do backend | toast vermelho fechável no topo (mensagem em pt-BR vinda do backend) |
| OBS/MediaMTX fora | dot vermelho no indicador correspondente |

## Testes

`ng test` (Vitest). 11 specs em 3 arquivos:

| Arquivo | Cobre |
|---|---|
| `websocket.service.spec.ts` | estado inicial; reduce de `cameras.updated`, `system.status` e `error` para os signals; mensagem malformada não explode |
| `camera-card.component.spec.ts` | render online (botão habilitado); offline (desabilitado); live (badge "NO AR" + desabilitado); clique emite `goLive` com o id |
| `app.spec.ts` | criação do app; render de control-bar + camera-grid (com `WebSocketService` fake para não abrir socket real) |

Nota: use matchers do Vitest (`toBe(true)`) — os matchers do Jasmine
(`toBeTrue()`) não existem aqui.

## Comandos

```bash
npm start            # ng serve → http://localhost:4200
npm test             # ng test (Vitest)
npm run build        # ng build → dist/frontend
```

Ou pela raiz do repo: `make dev-frontend`.

## Limitações e evolução prevista

- **Sem preview de vídeo** — o card já reserva o espaço; o plano prevê preview
  via HLS do MediaMTX (`http://localhost:8888/<path>`) em fase futura.
- **Token estático em `environment.ts`** — auth real (login/sessão) fica para
  depois da PoC.
- **Sem tratamento visual de erro nas chamadas REST** — um `POST /live` que
  falhe com 409/502 hoje falha silenciosamente (o operador percebe porque o
  estado não muda); um toast de erro HTTP é uma melhoria natural.
- **Uma única cena/layout** — o painel controla apenas a visibilidade dentro da
  cena `Program`.
