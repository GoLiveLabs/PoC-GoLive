# Documentação do Backend — Entendendo o Coração da Aplicação

## O que faz o Backend?

O backend é o "maestro" da aplicação. Ele fica escutando todas as câmeras que estão transmitindo, e controla qual câmera aparece no OBS (seu programa de transmissão). É como um maestro de orquestra que decide qual instrumento deve tocar agora.

## Como funciona, passo a passo?

### 1. **Inicialização (Quando você liga o servidor)**

Quando você digita `make dev-backend`, o backend faz o seguinte:

- **Conecta ao MediaMTX** (o servidor de vídeo que recebe as câmeras)
- **Conecta ao OBS** (o programa onde você transmite)
- **Cria a cena "Program"** no OBS (ou usa se já existir)
- **Inicia um loop de sincronização** que verifica a cada 3 segundos se há câmeras novas ou desconectadas

### 2. **Descobrindo câmeras (Loop de sincronização)**

A cada 3 segundos, o backend pergunta ao MediaMTX: "Quais câmeras estão transmitindo agora?"

**O que acontece com cada câmera:**

- **Câmera aparece pela primeira vez:** 
  - Backend cria um input ffmpeg no OBS com o nome `cam_<nome_da_camera>`
  - Adiciona um indicador visual que a câmera está "Online"
  
- **Câmera que estava online desaparece:**
  - Backend marca como "Offline" (não remove logo, espera 60 segundos)
  - Se continua offline por 60 segundos, remove o input do OBS
  - Razão: absorver pequenas interrupções de rede sem piscar a transmissão

- **Câmera volta a aparecer:**
  - Muda o status de "Offline" para "Online" novamente

### 3. **Quando você escolhe uma câmera para ir ao ar**

Quando você clica em "Colocar no ar" na tela do navegador:

1. Backend recebe o pedido
2. Procura pela câmera no OBS
3. Torna essa câmera visível na cena "Program"
4. Esconde todas as outras câmeras
5. Avisa todos os navegadores conectados (via WebSocket) que a câmera mudou

## As partes principais do Backend

### **1. Orquestrador (orchestrator)**

É o "cérebro" da aplicação. Faz:
- Mantém a lista de câmeras em memória
- Faz a sincronização a cada 3 segundos
- Controla qual câmera está ao vivo
- Envia mensagens para todos os navegadores sobre mudanças

**Arquivos principais:**
- `orchestrator.go` — a lógica principal
- `models.go` — os dados das câmeras

### **2. Conector MediaMTX (mediaserver)**

É o "ouvido" que escuta o servidor de vídeo.

**Faz:**
- Pergunta ao MediaMTX: "Quais câmeras estão transmitindo?"
- Recebe a lista com informações de cada câmera
- Passa para o orquestrador processar

**Arquivo:** `mediaserver/client.go`

### **3. Conector OBS (obs)**

É as "mãos" que controlam o OBS.

**Faz:**
- Cria inputs ffmpeg no OBS
- Mostra/esconde câmeras
- Remove câmeras que não estão mais em uso
- Verifica se o OBS está conectado a cada 5 segundos
- Se perder conexão, tenta reconectar com paciência crescente (1s, 2s, 4s, 8s... até 30s)

**Arquivo:** `obs/obs.go`

### **4. API HTTP + WebSocket (httpapi)**

É o "porta-voz" que fala com o navegador.

**Faz:**
- Fornece informações sobre câmeras em tempo real
- Recebe pedidos para mudar qual câmera vai ao ar
- Envia atualizações para todos os navegadores conectados via WebSocket
- Rejeita requisições que não têm o token de segurança correto

**Arquivos principais:**
- `httpapi/httpapi.go` — as rotas e endpoints
- `httpapi/ws.go` — a comunicação em tempo real

### **5. Hub de Eventos (events)**

É o "mural de avisos" onde todos os eventos importantes são publicados.

**Faz:**
- Quando algo importante acontece (câmera nova, câmera desconectada, câmera foi ao ar), publica um evento
- Todos os WebSocket conectados recebem a mensagem

**Arquivo:** `events/hub.go`

### **6. Configuração (config)**

Lê as configurações de variáveis de ambiente.

**Arquivo:** `config/config.go`

### **7. Gestão de Clients, Ingests, Streaming Platforms e Live IDs**

Ao lado do orquestrador de câmeras (que não guarda nada em disco), o backend
também expõe uma API de gestão para quatro cadastros que **são persistidos em
Postgres**: clientes finais do operador, fontes de ingestão de mídia desses
clientes, o catálogo de plataformas de streaming (YouTube, Twitch, ...) e os
ids de transmissão associando um client a uma plataforma.

Diferente do orquestrador, esses dados sobrevivem a um restart do backend.
Cada um vive em seu próprio pacote, seguindo o mesmo padrão de "pacote por
responsabilidade" do resto do backend:

- `internal/client` — clientes (nome + e-mail opcional), soft delete.
- `internal/ingest` — fontes de ingestão de um client; o protocolo
  (`http`/`https`/`ftp`/`sftp`/`s3`) é sempre derivado da própria URL, nunca
  aceito como campo separado. URLs com credenciais embutidas são rejeitadas.
- `internal/streamplatform` — catálogo global de plataformas de streaming,
  compartilhado (não há conceito de tenant nesta API hoje).
- `internal/liveid` — associação entre um client e um id de transmissão numa
  plataforma; um client pode ter vários live ids na mesma plataforma.
- `internal/pagination` — paginação por cursor opaco (base64 sobre
  `created_at`+`id`), usada por todas as listagens acima.
- `internal/dbconn` — conexão com Postgres via GORM e criação do schema
  (SQL simples e idempotente, não `AutoMigrate` — ver comentário no arquivo).

Essas rotas passam pela mesma autenticação por `X-Api-Token` do resto da API.
Não há isolamento multi-tenant nesta primeira versão: todo client, ingest,
platform e live id é visível a qualquer requisição autenticada.

## Os dados que o backend trabalha

### **Câmera**

Cada câmera tem essas informações:

```
{
  "id": "camera1",                    // Nome da câmera
  "name": "camera1",                  // Nome legível
  "sourceUrl": "rtmp://localhost:1935/camera1",  // Link de vídeo (monta a partir de MEDIA_SOURCE_BASE_URL + nome)
  "status": "online",                 // online ou offline
  "obsSourceCreated": true,           // Se já foi criada no OBS
  "isLive": true,                     // Se está sendo transmitida agora
  "lastSeenAt": "2024-07-17T10:00:00Z" // Quando foi vista por último
}
```

### **Status do Sistema**

O estado geral da aplicação:

```
{
  "obsConnected": true,               // OBS ligado?
  "mediaServerConnected": true,       // MediaMTX ligado?
  "streaming": true,                  // Tem câmera ao vivo?
  "activeSceneName": "Program",       // Nome da cena no OBS
  "liveCameraId": "camera1"           // Qual câmera está ao vivo
}
```

## Endpoints da API (como falar com o backend)

### GET `/api/v1/health`
"Backend, você está vivo?" → Responde com OK

### GET `/api/v1/cameras`
"Quais são todas as câmeras?" → Lista todas as câmeras

### GET `/api/v1/status`
"Qual é o estado geral?" → Retorna o SystemStatus

### POST `/api/v1/cameras/:id/live`
"Coloca a câmera X ao vivo" → Muda a câmera transmitida

### POST `/api/v1/sync`
"Sincroniza agora, não espera 3 segundos" → Força uma sincronização imediata

### WebSocket `/api/v1/ws`
Conexão em tempo real para receber atualizações automáticas

### Gestão de Clients / Ingests / Streaming Platforms / Live IDs

Estas rotas usam paginação por cursor (`?limit=&cursor=`, resposta
`{data, nextCursor, hasMore}`) e retornam `422` com um mapa `errors` em
falhas de validação de campo.

| Método | Rota | Sucesso | Observação |
|---|---|---|---|
| POST | `/api/v1/clients` | 201 | nome duplicado → 409 |
| GET | `/api/v1/clients` | 200 | só clients não deletados |
| GET | `/api/v1/clients/{id}` | 200 | |
| PATCH | `/api/v1/clients/{id}` | 200 | `email` ausente = não mexe; `null` = limpa |
| DELETE | `/api/v1/clients/{id}` | 204 | soft delete |
| POST | `/api/v1/clients/{clientID}/ingests` | 201 | protocolo derivado da URL |
| GET | `/api/v1/clients/{clientID}/ingests` | 200 | filtros `isActive`, paginação |
| GET | `/api/v1/ingests` | 200 | filtros `clientId`, `isActive` |
| GET,PATCH,DELETE | `/api/v1/ingests/{id}` | 200/200/204 | DELETE é hard delete |
| POST | `/api/v1/streaming-platforms` | 201 | catálogo global; slug duplicado → 409 |
| GET | `/api/v1/streaming-platforms` | 200 | |
| GET,PATCH,DELETE | `/api/v1/streaming-platforms/{id}` | 200/200/204 | DELETE falha 409 se houver live id em uso |
| POST | `/api/v1/clients/{clientID}/live-ids` | 201 | client e platform checados na mesma transação |
| GET | `/api/v1/clients/{clientID}/live-ids` | 200 | filtros `platformId`, `isActive` |
| GET | `/api/v1/live-ids` | 200 | filtros `clientId`, `platformId`, `isActive` |
| GET,PATCH,DELETE | `/api/v1/live-ids/{id}` | 200/200/204 | `platformId`/`clientId` não são editáveis

## Como o backend se reconecta?

### Conexão com OBS

Se o OBS desligar ou desconectar:

1. A cada 5 segundos, backend tenta fazer uma requisição simples ao OBS ("Qual é sua versão?")
2. Se falhar, tira 1 segundo de espera antes de tentar novamente
3. A próxima tentativa espera 2 segundos, depois 4, depois 8... até 30 segundos
4. Quando conseguir conectar, volta ao normal (tenta a cada 5 segundos)

### Conexão com MediaMTX

Se perder conexão com o servidor de câmeras:

1. Backend marca como "desconectado"
2. Continua tentando sincronizar a cada 3 segundos
3. Quando conseguir conectar novamente, sincronia automática

## Coisas importantes para lembrar

### **Token de segurança**

Todo pedido para a API (exceto `/health`) precisa ter o header:
```
X-Api-Token: dev-token
```

Se não tiver, o servidor rejeita.

### **O estado das câmeras não é salvo — os cadastros de gestão, sim**

Quando você desliga o backend, tudo que o orquestrador sabia sobre câmeras é perdido: na próxima vez que ligar, ele descobre tudo de novo a partir do MediaMTX. Isso é proposital.

Já os clients, ingests, streaming platforms e live ids (seção acima) **são salvos em Postgres** e sobrevivem a um restart — são cadastros de gestão, não estado descartável do orquestrador.

### **Graceful shutdown (Encerramento limpo)**

Quando você pressiona `Ctrl+C`:

1. Backend avisa a todos os WebSocket que está fechando
2. Encerra a sincronização
3. Desconecta do OBS
4. Fecha o servidor HTTP

Tudo isso em menos de 3 segundos.

## Variáveis de Ambiente

Você pode customizar o comportamento mudando essas variáveis:

| Variável | Padrão | O que faz |
|----------|--------|----------|
| `HTTP_ADDR` | `:8080` | Porta onde o backend escuta |
| `OBS_ADDR` | `localhost:4455` | Endereço do OBS WebSocket |
| `OBS_PASSWORD` | vazio | Senha do OBS (se tiver) |
| `MEDIAMTX_API_URL` | `http://localhost:9997` | Endereço do MediaMTX |
| `MEDIA_SOURCE_BASE_URL` | `rtmp://localhost:1935` | Base usada para montar a URL de vídeo (`sourceUrl`) de cada câmera |
| `API_TOKEN` | `dev-token` | Token de segurança |
| `SYNC_INTERVAL` | `3s` | Frequência de sincronização |
| `PROGRAM_SCENE` | `Program` | Nome da cena no OBS |
| `LOG_LEVEL` | `info` | Verbosidade dos logs (debug/info/warn/error) |
| `DATABASE_URL` | `postgres://postgres:postgres@localhost:5433/live_orchestrator?sslmode=disable` | Conexão Postgres usada por clients/ingests/streaming-platforms/live-ids (porta 5433 no host para não colidir com outro Postgres local na 5432) |

## Fluxo completo: do início ao fim

1. **Backend liga**
   - Conecta ao MediaMTX e OBS
   - Cria a cena "Program" no OBS

2. **Câmera 1 começa a transmitir**
   - Backend sincroniza, descobre câmera 1
   - Cria input `cam_camera1` no OBS
   - Envia para o navegador: "câmera 1 está online"

3. **Câmera 2 começa a transmitir**
   - Backend sincroniza, descobre câmera 2
   - Cria input `cam_camera2` no OBS
   - Envia para o navegador: "câmera 2 está online"

4. **Usuário clica "Colocar no ar" na câmera 2**
   - Backend recebe pedido
   - Esconde `cam_camera1` no OBS
   - Mostra `cam_camera2` no OBS
   - Envia para o navegador: "câmera 2 está ao vivo"

5. **Câmera 1 cai (sem sinal)**
   - Backend sincroniza, câmera 1 desapareceu
   - Marca como "offline"
   - Depois de 60 segundos, remove do OBS

6. **Usuário desliga o programa (Ctrl+C)**
   - Backend desconecta graciosamente
   - Encerra em menos de 3 segundos
   - Tudo pronto para a próxima execução
