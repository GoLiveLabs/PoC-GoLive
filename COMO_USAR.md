# Como utilizar o projeto — Guia passo a passo

Este guia leva você do zero até uma demonstração completa: câmeras publicando
no MediaMTX, fontes aparecendo sozinhas no OBS e troca de câmera ao vivo pelo
painel web.

> Documentação técnica detalhada: [`docs/BACKEND.md`](docs/BACKEND.md) e
> [`docs/FRONTEND.md`](docs/FRONTEND.md). Decisões de implementação:
> [`DECISIONS.md`](DECISIONS.md).

## Índice

- [1. O que você vai precisar](#1-o-que-você-vai-precisar)
- [2. Preparação (uma única vez)](#2-preparação-uma-única-vez)
- [3. Subindo o sistema (ordem recomendada)](#3-subindo-o-sistema-ordem-recomendada)
- [4. Usando o painel](#4-usando-o-painel)
- [5. Simulando câmeras sem hardware](#5-simulando-câmeras-sem-hardware)
- [6. Usando câmeras reais](#6-usando-câmeras-reais)
- [7. Roteiro de demonstração completo](#7-roteiro-de-demonstração-completo)
- [8. Usando a API diretamente (curl)](#8-usando-a-api-diretamente-curl)
- [9. Encerrando tudo](#9-encerrando-tudo)
- [10. Solução de problemas](#10-solução-de-problemas)

---

## 1. O que você vai precisar

| Ferramenta | Versão | Para quê |
|---|---|---|
| Go | 1.22+ | backend |
| Node.js + npm | 18+ | frontend Angular |
| Docker Desktop | qualquer recente | MediaMTX (media server) |
| OBS Studio | 28+ (obs-websocket v5 embutido) | composição e transmissão |
| ffmpeg | qualquer recente | simular câmeras em dev (opcional se tiver câmeras reais) |
| make | opcional | atalhos do `Makefile` (dá para rodar os comandos manualmente) |

## 2. Preparação (uma única vez)

### 2.1 Habilitar o obs-websocket no OBS

1. Abra o OBS Studio.
2. Menu **Ferramentas → Configurações do Servidor WebSocket**
   (Tools → WebSocket Server Settings).
3. Marque **Habilitar servidor WebSocket** (Enable WebSocket server).
4. Confirme a porta **4455** (padrão).
5. Defina uma senha (ou desabilite a autenticação para dev).
6. Clique em **OK**.

> Se definir senha, você vai passá-la ao backend via `OBS_PASSWORD` (passo 3.3).

### 2.2 Instalar as dependências do frontend

```bash
cd frontend
npm install
```

### 2.3 (Opcional) Gerar um vídeo de teste para as câmeras fake

O alvo `make fake-camera` espera um `sample.mp4` na raiz do repositório. Se você
não tiver um vídeo à mão, gere um sintético (barras de teste + tom de 440 Hz):

```bash
ffmpeg -y -f lavfi -i testsrc=size=640x360:rate=30 -f lavfi -i sine=frequency=440 \
  -t 30 -c:v libx264 -preset ultrafast -c:a aac -shortest sample.mp4
```

## 3. Subindo o sistema (ordem recomendada)

A ordem não é obrigatória (tudo se reconecta sozinho), mas esta sequência evita
avisos de conexão nos logs.

### 3.1 MediaMTX (media server)

```bash
make mediamtx-up
# ou: docker compose up -d mediamtx
```

Verifique: `curl http://localhost:9997/v3/paths/list` deve responder JSON
(`{"itemCount":0,...}` quando não há câmeras).

Portas expostas: `1935` (RTMP), `8554` (RTSP), `8890/udp` (SRT), `9997` (API).

### 3.2 OBS Studio

Abra o OBS (com o WebSocket habilitado, passo 2.1). Só isso — a cena `Program`
e as fontes serão criadas automaticamente pelo backend.

### 3.3 Backend

Sem senha no OBS:

```bash
make dev-backend
# ou: cd backend && go run ./cmd/server
```

Com senha no OBS:

```bash
# Git Bash / Linux / macOS
OBS_PASSWORD=suasenha make dev-backend

# PowerShell
$env:OBS_PASSWORD = 'suasenha'; cd backend; go run ./cmd/server
```

Você deve ver no log: `http server listening addr=:8080`. Verifique:
`curl http://localhost:8080/api/v1/health` → `{"status":"ok"}`.

Todas as variáveis disponíveis (com defaults) estão em [`.env.example`](.env.example):

```
HTTP_ADDR=:8080
OBS_ADDR=localhost:4455
OBS_PASSWORD=
MEDIAMTX_API_URL=http://localhost:9997
API_TOKEN=dev-token
SYNC_INTERVAL=3s
PROGRAM_SCENE=Program
LOG_LEVEL=info
```

### 3.4 Frontend

```bash
make dev-frontend
# ou: cd frontend && npm start
```

Abra **http://localhost:4200**.

> Se você mudou `API_TOKEN` ou a porta do backend, ajuste
> `frontend/src/environments/environment.ts` para bater.

## 4. Usando o painel

A barra do topo mostra quatro indicadores, atualizados em tempo real:

| Indicador | Verde | Vermelho |
|---|---|---|
| **Painel** | WebSocket com o backend conectado | backend fora do ar / reconectando |
| **OBS** | backend conectado ao obs-websocket | OBS fechado ou WebSocket desabilitado |
| **MediaMTX** | API do MediaMTX respondendo | container parado |
| **No ar** | nome da câmera ao vivo | — ("nenhuma câmera") |

Comportamentos:

- **Câmera aparece sozinha** na grade em até ~3s (intervalo do sync) depois de
  começar a publicar no MediaMTX. Junto, a fonte `cam_<nome>` aparece na cena
  `Program` do OBS.
- **"Colocar no ar"** em um card: a fonte daquela câmera fica visível no OBS e
  todas as outras fontes `cam_*` são ocultadas. O card ganha destaque vermelho
  e badge "NO AR". Só uma câmera fica no ar por vez.
- **Câmera cai**: o card fica "Offline" (cinza, botão desabilitado). A fonte
  **não** é removida do OBS imediatamente — há uma tolerância de **60 segundos**
  para quedas rápidas de conexão. Se voltar dentro desse tempo, nada muda no OBS.
  Passados 60s offline, a fonte é removida do OBS e o card some da grade.
- **A câmera no ar caiu**: aparece um toast vermelho avisando. O sistema **não**
  troca de câmera sozinho — o operador decide qual colocar no ar.
- **"Sincronizar agora"**: força um ciclo de sincronização imediato (útil se
  você não quiser esperar os 3s do ciclo automático).
- **Backend caiu**: banner "Sem conexão com o backend. Tentando reconectar...".
  A reconexão é automática (tentativas de 1s a 30s) e o painel se re-hidrata
  sozinho quando o backend volta.

## 5. Simulando câmeras sem hardware

Com o `sample.mp4` na raiz (passo 2.3), cada comando abaixo vira uma "câmera"
publicando em loop via RTMP (um terminal por câmera):

```bash
make fake-camera NAME=camera1
make fake-camera NAME=camera2
# ou diretamente:
# ffmpeg -re -stream_loop -1 -i sample.mp4 -c copy -f flv rtmp://localhost:1935/camera1
```

O nome do path (`camera1`) vira o ID e o nome do card no painel. Para "derrubar"
a câmera, basta interromper o ffmpeg (`Ctrl+C` no terminal dela).

## 6. Usando câmeras reais

Qualquer dispositivo que publique RTMP ou SRT serve. Aponte-o para a máquina que
roda o MediaMTX:

- **RTMP**: `rtmp://<ip-da-máquina>:1935/<nome-da-camera>`
  (em apps tipo Larix/prism, use server `rtmp://<ip>:1935` e stream key `<nome-da-camera>`)
- **SRT (caller)**: `srt://<ip-da-máquina>:8890?streamid=publish:<nome-da-camera>`

O `<nome-da-camera>` que você escolher é o que aparece no painel. Nada precisa
ser pré-cadastrado — o MediaMTX aceita qualquer nome de path (config
`all_others`) e o backend detecta automaticamente.

## 7. Roteiro de demonstração completo

O fluxo de ponta a ponta do projeto (seção 9 do plano), já validado:

1. `make mediamtx-up`
2. `make fake-camera NAME=camera1` e, em outro terminal, `make fake-camera NAME=camera2`
3. Abra o OBS (WebSocket habilitado) → `make dev-backend` → `make dev-frontend`
4. No navegador (`http://localhost:4200`): **duas câmeras "Online" na grade**
   em poucos segundos; no OBS, a cena `Program` com `cam_camera1` e `cam_camera2`.
5. Clique **"Colocar no ar"** na camera2 → no OBS, só `cam_camera2` fica visível;
   no painel, o card ganha o destaque "NO AR" e o topo mostra "No ar: camera2".
6. Interrompa o ffmpeg da camera1 → o card fica **Offline**; após ~60s a fonte
   `cam_camera1` some do OBS e o card sai da grade.
7. Rode a camera1 de novo → ela **volta sozinha** à grade e ao OBS, sem nenhuma
   ação manual.

Para transmitir de verdade, configure a saída de stream do próprio OBS
(Twitch/YouTube) normalmente — o orquestrador só controla quais fontes estão
visíveis na cena.

## 8. Usando a API diretamente (curl)

Todas as rotas (exceto `/health`) exigem o header `X-Api-Token` (default
`dev-token`):

```bash
# liveness (sem token)
curl http://localhost:8080/api/v1/health

# listar câmeras
curl -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/cameras

# status geral
curl -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/status

# colocar camera1 no ar
curl -X POST -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/cameras/camera1/live

# forçar sincronização
curl -X POST -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/sync
```

Eventos em tempo real via WebSocket (o token pode ir na query string):

```bash
websocat "ws://localhost:8080/api/v1/ws?api_token=dev-token"
```

Ao conectar você recebe imediatamente um `cameras.updated` e um `system.status`
(snapshot), e depois um evento a cada mudança. Contratos completos em
[`docs/BACKEND.md`](docs/BACKEND.md).

## 9. Encerrando tudo

```bash
# backend: Ctrl+C no terminal (shutdown gracioso, < 3s)
# frontend: Ctrl+C no terminal do ng serve
# câmeras fake: Ctrl+C em cada terminal do ffmpeg
make mediamtx-down        # para e remove o container do MediaMTX
```

## 10. Solução de problemas

| Sintoma | Causa provável | Solução |
|---|---|---|
| Indicador **OBS: desconectado** | OBS fechado, WebSocket desabilitado ou senha errada | Abra o OBS, confira o passo 2.1 e o `OBS_PASSWORD`. O backend reconecta sozinho (backoff 1s→30s) — acompanhe o log `obs reconnected`. |
| Indicador **MediaMTX: desconectado** | container parado | `make mediamtx-up`; confira `docker ps` e `curl http://localhost:9997/v3/paths/list`. |
| ffmpeg falha com `path 'X' is not configured` | `mediamtx.yml` sem o path coringa | garanta que o `mediamtx.yml` da raiz tem o bloco `paths: { all_others: }` e reinicie: `docker compose restart mediamtx`. |
| `curl` na API do MediaMTX responde `authentication error` | container usando config default (com auth) em vez do `mediamtx.yml` do repo | confira o volume no `docker-compose.yml` e recrie: `docker compose up -d --force-recreate mediamtx`. |
| Painel mostra 401 / nada carrega | token do frontend ≠ token do backend | alinhe `environment.ts` (`apiToken`) com `API_TOKEN` do backend. |
| Backend não sobe: porta 8080 em uso | instância anterior ainda rodando | encerre o processo antigo (`Get-NetTCPConnection -LocalPort 8080` no PowerShell para achar o PID) ou use outra porta via `HTTP_ADDR`. |
| Câmera não aparece no painel | stream não está `ready` no MediaMTX | `curl http://localhost:9997/v3/paths/list` e confira `"ready": true`; veja os logs do ffmpeg/câmera. |
| Fontes aparecem no OBS mas a imagem fica preta | o OBS não alcança a URL RTMP | por padrão a URL da fonte usa o host `mediamtx` (pensado para OBS em rede Docker). Com OBS rodando direto no Windows, adicione `127.0.0.1 mediamtx` ao arquivo `hosts`, ou ajuste a URL gerada em `orchestrator.go` para `localhost`. |
| Aviso "A câmera ao vivo ficou offline" | a câmera no ar parou de publicar | escolha outra câmera e clique "Colocar no ar" — a troca é sempre decisão do operador. |
