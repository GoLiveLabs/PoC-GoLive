# Plano de Implementação — Orquestrador de Câmeras para Live

> **Instruções para o Claude Code:** Este documento é a especificação completa do projeto.
> Implemente as fases NA ORDEM. Ao final de cada fase, rode os critérios de aceite antes
> de avançar. Não pule fases. Se algo estiver ambíguo, prefira a solução mais simples
> que atenda ao critério de aceite e registre a decisão em `DECISIONS.md`.

---

## 1. Visão geral

Sistema de orquestração de múltiplas câmeras para transmissão ao vivo.

**Fluxo:** Câmeras enviam vídeo (RTMP/SRT) para um **MediaMTX** (media server).
Um **backend em Go** monitora as câmeras ativas no MediaMTX, sincroniza-as como
fontes (Media Source / `ffmpeg_source`) dentro do **OBS Studio** via **obs-websocket v5**,
e expõe uma API REST + WebSocket para um **painel Angular**, onde o operador
vê as câmeras disponíveis e troca a câmera/cena ativa da transmissão.

```
Câmeras → MediaMTX → [Backend Go] → OBS Studio → Plataformas (Twitch/YouTube)
                          ↑↓ REST/WS
                     Painel Angular
```

**Fora do escopo desta versão (NÃO implementar):**
- Autenticação de usuários (usar um token estático simples via header)
- Banco de dados (estado em memória; persistência virá depois)
- Composição de vídeo própria (o OBS faz a composição)
- Deploy em produção (apenas ambiente local via docker-compose para o MediaMTX)

---

## 2. Stack tecnológica

| Camada | Tecnologia | Versão mínima |
|---|---|---|
| Backend | Go | 1.22+ |
| Cliente OBS | `github.com/andreykaipov/goobs` | mais recente |
| HTTP router | `net/http` padrão (Go 1.22 tem routing com métodos) | stdlib |
| WebSocket (para o frontend) | `github.com/coder/websocket` | mais recente |
| Frontend | Angular | 17+ (standalone components, signals) |
| Estilo | CSS puro ou Tailwind (escolher o mais simples) | — |
| Media server (dev) | MediaMTX via Docker | mais recente |
| Testes Go | `testing` padrão + `httptest` | stdlib |
| Testes Angular | Jasmine/Karma padrão do CLI | padrão |

**Regras gerais:**
- Sem frameworks web pesados no Go (não usar Gin/Echo; stdlib dá conta).
- Frontend consome o backend apenas via REST + WebSocket (nenhuma chamada direta ao OBS ou MediaMTX).
- Todo código, nomes de variáveis e comentários em **inglês**. Mensagens exibidas ao usuário final (frontend) em **português (pt-BR)**.

---

## 3. Estrutura do repositório (monorepo)

```
live-orchestrator/
├── PLANO_IMPLEMENTACAO.md      # este arquivo
├── DECISIONS.md                 # decisões tomadas durante a implementação
├── docker-compose.yml           # MediaMTX para desenvolvimento
├── Makefile                     # atalhos: run, test, lint, dev
├── backend/
│   ├── go.mod
│   ├── cmd/
│   │   └── server/
│   │       └── main.go          # entrypoint: carrega config, injeta dependências, sobe HTTP
│   └── internal/
│       ├── config/              # leitura de env vars (porta, senha OBS, URL MediaMTX, token API)
│       ├── mediaserver/         # cliente HTTP do MediaMTX (lista paths ativos)
│       ├── obs/                 # wrapper da goobs (criar/remover input, trocar cena, eventos)
│       ├── orchestrator/        # regra de negócio: loop de sync + estado em memória
│       ├── httpapi/             # handlers REST + upgrade WebSocket + middleware de token
│       └── events/              # hub de eventos: broadcast de mudanças p/ clientes WS
└── frontend/
    └── (projeto Angular CLI padrão)
        └── src/app/
            ├── core/            # ApiService, WebSocketService, modelos TS
            ├── features/
            │   ├── camera-grid/ # grade de câmeras com status
            │   └── control-bar/ # botões de troca de cena / câmera ativa
            └── app.component.ts # layout: grid + barra de controle + indicador de conexão
```

---

## 4. Modelos de dados (contrato compartilhado)

### 4.1 Camera (backend `orchestrator`, frontend `core/models`)

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

- `id`: derivado do path no MediaMTX (único).
- `status`: `"online"` | `"offline"`.
- `isLive`: `true` se esta câmera é a fonte visível na cena de programa do OBS.

### 4.2 SystemStatus

```json
{
  "obsConnected": true,
  "mediaServerConnected": true,
  "streaming": false,
  "activeSceneName": "Program",
  "liveCameraId": "camera1"
}
```

### 4.3 Eventos WebSocket (backend → frontend)

Formato do envelope: `{ "type": string, "payload": object }`

| type | payload | quando |
|---|---|---|
| `cameras.updated` | `Camera[]` (lista completa) | qualquer mudança na lista/estado das câmeras |
| `system.status` | `SystemStatus` | mudança de conexão OBS/MediaMTX ou troca de câmera ao vivo |
| `error` | `{ "message": string }` | erro relevante para o operador |

O backend envia `cameras.updated` e `system.status` imediatamente após o cliente conectar (snapshot inicial).

---

## 5. Contrato da API REST (backend)

Base: `http://localhost:8080/api/v1` — todas as rotas exigem header `X-Api-Token: <token>`.

| Método | Rota | Descrição | Resposta |
|---|---|---|---|
| GET | `/health` | liveness (sem token) | `{ "status": "ok" }` |
| GET | `/cameras` | lista câmeras conhecidas | `Camera[]` |
| GET | `/status` | status geral | `SystemStatus` |
| POST | `/cameras/{id}/live` | coloca a câmera no ar (mostra a fonte dela e esconde as demais na cena de programa) | `SystemStatus` |
| POST | `/sync` | força um ciclo de sincronização imediato | `Camera[]` |
| GET | `/ws` | upgrade para WebSocket de eventos | — |

**Erros:** JSON `{ "error": string }` com status HTTP adequado (400/404/409/502).
- 404 se `{id}` não existe.
- 409 se a câmera está `offline` ao tentar colocá-la no ar.
- 502 se o OBS ou MediaMTX estiverem inacessíveis no momento da ação.

---

## 6. Integrações externas

### 6.1 MediaMTX (dev via docker-compose)

`docker-compose.yml` na raiz:

```yaml
services:
  mediamtx:
    image: bluenviron/mediamtx:latest
    ports:
      - "1935:1935"   # RTMP ingest
      - "8554:8554"   # RTSP
      - "8890:8890/udp" # SRT
      - "9997:9997"   # API HTTP
    environment:
      - MTX_API=yes
      - MTX_APIADDRESS=:9997
```

- Endpoint para listar streams ativos: `GET http://localhost:9997/v3/paths/list`.
- O cliente em `internal/mediaserver` deve: fazer a chamada, filtrar apenas paths com
  fonte ativa (`ready: true`), e mapear para `[]Camera`.
- **Validar o shape real da resposta da API na primeira execução** (a estrutura pode
  variar entre versões do MediaMTX); ajustar o struct de decode conforme necessário e
  registrar em `DECISIONS.md`.
- Para simular câmeras em dev: `ffmpeg -re -stream_loop -1 -i sample.mp4 -c copy -f flv rtmp://localhost:1935/camera1`
  (documentar esse comando no README; incluir um alvo `make fake-camera NAME=camera1`).

### 6.2 OBS Studio (obs-websocket v5)

Wrapper em `internal/obs` usando `goobs`, expondo interface própria (para permitir mock nos testes):

```go
type Controller interface {
    EnsureScene(name string) error
    CreateCameraInput(sceneName, inputName, url string) error
    RemoveInput(inputName string) error
    SetOnlyVisibleSource(sceneName, inputName string) error // mostra um, esconde os demais
    IsConnected() bool
    Reconnect() error
}
```

- Cena de trabalho padrão: `"Program"` (criar se não existir).
- Inputs criados com kind `ffmpeg_source`, settings: `{"input": url, "is_local_file": false, "reconnect": true (se o campo existir na versão), "buffering_mb": valor baixo}`.
  **Confirmar os nomes exatos dos campos de settings do `ffmpeg_source` consultando
  `GetInputDefaultSettings` do obs-websocket em runtime na primeira execução** — não
  chutar nomes de campos; registrar os nomes reais em `DECISIONS.md`.
- Nome do input no OBS = `cam_<camera.id>` (prefixo evita colisão com fontes manuais do usuário).
- Reconexão: se a conexão com o OBS cair, tentar reconectar com backoff exponencial (1s, 2s, 4s... máx 30s) sem derrubar o servidor HTTP.

---

## 7. Fases de implementação

### FASE 0 — Bootstrap do repositório
**Tarefas**
1. Criar estrutura de pastas conforme seção 3.
2. `backend`: `go mod init`, `main.go` que sobe HTTP em `:8080` com `GET /api/v1/health`.
3. `frontend`: gerar projeto com Angular CLI (standalone, routing off, CSS simples).
4. Criar `docker-compose.yml`, `Makefile` (`make dev-backend`, `make dev-frontend`, `make mediamtx-up`, `make test`), `DECISIONS.md` vazio e `README.md` com instruções de execução.

**Critérios de aceite**
- `make mediamtx-up` sobe o MediaMTX e `curl http://localhost:9997/v3/paths/list` responde JSON.
- `curl http://localhost:8080/api/v1/health` responde `{"status":"ok"}`.
- `ng serve` abre a página padrão do Angular sem erros.

---

### FASE 1 — Config + cliente MediaMTX
**Tarefas**
1. `internal/config`: struct `Config` lida de env vars com defaults:
   `HTTP_ADDR=:8080`, `OBS_ADDR=localhost:4455`, `OBS_PASSWORD=""`,
   `MEDIAMTX_API_URL=http://localhost:9997`, `API_TOKEN=dev-token`,
   `SYNC_INTERVAL=3s`, `PROGRAM_SCENE=Program`.
2. `internal/mediaserver`: `Client` com método `ListActiveStreams(ctx) ([]StreamInfo, error)`.
   Timeout de 5s por chamada. Erros de rede não derrubam o processo.
3. Testes unitários do client usando `httptest.Server` com resposta fake do MediaMTX
   (casos: lista vazia, 2 streams ativos, stream não-ready filtrado, erro 500, timeout).

**Critérios de aceite**
- `go test ./...` verde.
- Programa de exemplo (ou log no startup) imprime as câmeras ativas reais do MediaMTX local.

---

### FASE 2 — Wrapper OBS
**Tarefas**
1. Implementar `internal/obs` conforme interface da seção 6.2, usando `goobs`.
2. `EnsureScene`: cria a cena `Program` se não existir e não falha se já existir.
3. `CreateCameraInput`: usa `Inputs.CreateInput` com kind `ffmpeg_source`; se o input já existir, atualiza settings em vez de falhar.
4. `SetOnlyVisibleSource`: lista os scene items da cena, habilita o item alvo e desabilita todos os outros itens com prefixo `cam_`.
5. Reconexão com backoff em goroutine dedicada; `IsConnected()` thread-safe.
6. Testes: como a goobs fala com um servidor real, isolar a lógica de orquestração atrás da interface e testar com um mock (`obsmock`) — o wrapper em si terá cobertura via teste manual documentado no README (checklist).

**Critérios de aceite**
- Com o OBS aberto e obs-websocket habilitado: rodar o backend cria a cena `Program`.
- Checklist manual do README executado: criar input fake, alternar visibilidade, matar o OBS e reabrir → backend reconecta sozinho (verificar por log).

---

### FASE 3 — Orquestrador (núcleo)
**Tarefas**
1. `internal/orchestrator`: struct `Orchestrator` com estado em memória `map[string]*Camera` protegido por mutex.
2. Loop de sync (ticker com `SYNC_INTERVAL`):
   - Busca streams ativos no MediaMTX.
   - Câmera nova → cria input no OBS (`cam_<id>`), marca `obsSourceCreated=true`, status `online`.
   - Câmera sumiu → marca `offline` (NÃO remove o input do OBS imediatamente; remover apenas após 60s offline — evita flicker em quedas rápidas de conexão).
   - Câmera offline há 60s+ → `RemoveInput` e remove do mapa.
   - Se era a câmera `isLive` que caiu → publicar evento de erro (o operador decide a troca; não trocar automaticamente nesta versão).
3. Método `SetLive(cameraID)`: valida existência e status, chama `SetOnlyVisibleSource`, atualiza `isLive` (apenas uma câmera com `isLive=true`).
4. Publicar toda mudança de estado no hub de eventos (`internal/events`): hub simples com `Subscribe() (ch, cancel)` e `Publish(event)`, sem dependências externas.
5. Testes unitários do orquestrador com mocks de `mediaserver` e `obs` cobrindo: câmera aparece, câmera some e volta antes de 60s, câmera some por 60s+, SetLive em câmera offline (409), duas trocas seguidas de live.

**Critérios de aceite**
- `go test ./...` verde, incluindo os cenários acima.
- Teste manual: subir 2 câmeras fake via ffmpeg → fontes aparecem no OBS sozinhas; derrubar uma → após ~60s a fonte some do OBS.

---

### FASE 4 — API REST + WebSocket
**Tarefas**
1. `internal/httpapi`: implementar todas as rotas da seção 5 sobre o orquestrador.
2. Middleware de token (`X-Api-Token`), exceto `/health`. CORS liberado para `http://localhost:4200` em modo dev.
3. `GET /ws`: upgrade com `coder/websocket`; ao conectar, envia snapshot (`cameras.updated` + `system.status`); depois repassa eventos do hub; heartbeat ping a cada 30s; limpar assinatura no disconnect.
4. Testes de handler com `httptest` (auth negada sem token, 404, 409, fluxo feliz do `/cameras/{id}/live` com orquestrador mockado).

**Critérios de aceite**
- `go test ./...` verde.
- Roteiro manual com `curl` documentado no README executa sem erro (listar, colocar no ar, status).
- Conectar um cliente WS (ex: `websocat`) e ver o snapshot + eventos chegando ao ligar/desligar câmera fake.

---

### FASE 5 — Frontend Angular (painel do operador)
**Tarefas**
1. `core/`:
   - `models.ts` espelhando os modelos da seção 4.
   - `ApiService` (HttpClient) para as rotas REST; token via `HttpInterceptor` lendo de `environment.ts`.
   - `WebSocketService`: conecta em `/ws`, expõe o estado como **signals** (`cameras`, `systemStatus`, `connectionState`), reconexão automática com backoff (1s→30s) e re-snapshot ao reconectar.
2. `features/camera-grid`:
   - Grade responsiva de cards, um por câmera: nome, badge de status (`online` verde / `offline` cinza), destaque visual forte quando `isLive`.
   - Botão "Colocar no ar" em cada card (desabilitado se offline ou se já é a live).
   - Sem preview de vídeo nesta versão — apenas estado. (Preview via HLS do MediaMTX é uma fase futura; deixar espaço no card.)
3. `features/control-bar`:
   - Indicadores: conexão com backend (WS), OBS conectado, MediaMTX conectado, nome da câmera no ar.
   - Botão "Sincronizar agora" → `POST /sync`.
4. `app.component`: layout com control-bar no topo e camera-grid abaixo. Toast/banner simples para eventos `error`.
5. Textos da interface em pt-BR.
6. Testes: `WebSocketService` (reduce de eventos → estado) e componente do card (render dos estados, clique chama o service) — pelo menos esses dois specs.

**Critérios de aceite**
- `ng test` verde; `ng build` sem erros.
- Fluxo completo manual: subir MediaMTX + OBS + backend + frontend; 2 câmeras fake aparecem na grade em até 5s; clicar "Colocar no ar" troca a fonte visível no OBS e o destaque na grade acompanha; derrubar o backend → banner de desconexão; religar → painel se recupera sozinho.

---

### FASE 6 — Acabamento
**Tarefas**
1. Logging estruturado no backend (`log/slog`), níveis via env `LOG_LEVEL`.
2. Graceful shutdown (SIGINT/SIGTERM): parar loop de sync, fechar WS dos clientes, desconectar do OBS.
3. `make check`: `go vet` + `go test ./...` + `ng lint` (se configurado) + `ng test --watch=false`.
4. README final: arquitetura (diagrama ASCII), setup passo a passo, variáveis de ambiente, roteiros de teste manual, limitações conhecidas e próximos passos (persistência em banco, auth real, preview HLS, múltiplas cenas/layouts).

**Critérios de aceite**
- `make check` verde do zero em um clone limpo.
- `Ctrl+C` no backend encerra em menos de 3s sem stack trace.

---

## 8. Convenções e qualidade

- **Go:** pacotes pequenos e coesos; dependências injetadas por interface (nada de globals); `context.Context` como primeiro parâmetro em toda operação de I/O; erros com `fmt.Errorf("...: %w", err)`.
- **Angular:** standalone components; signals para estado; nada de lógica de negócio em componentes (fica nos services); `OnPush`/signals por padrão.
- **Commits:** um commit por tarefa concluída, mensagem no formato `fase-N: descrição curta`.
- **Nunca** commitar tokens/senhas; usar `.env.example` com placeholders.

## 9. Definição de pronto (projeto)

O projeto está pronto quando todos os critérios de aceite das fases 0–6 passam e o
fluxo de demonstração abaixo funciona de ponta a ponta:

1. `make mediamtx-up` → `make fake-camera NAME=camera1` e `NAME=camera2`.
2. Abrir OBS (obs-websocket habilitado) → `make dev-backend` → `make dev-frontend`.
3. No navegador: duas câmeras online na grade.
4. Clicar "Colocar no ar" na camera2 → OBS mostra apenas a camera2 na cena `Program`.
5. Matar a camera1 (parar o ffmpeg) → card fica offline; após 60s a fonte some do OBS.
6. Religar a camera1 → volta a aparecer sem nenhuma ação manual.
