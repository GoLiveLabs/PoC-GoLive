# Documentação — Live Orchestrator

Bem-vindo à documentação técnica do **Live Orchestrator**. Este guia fornece uma visão abrangente da arquitetura, serviços, fluxos de comunicação e responsabilidades de cada componente.

---

## 📋 Índice

### Visão Geral
- **[ESTRUTURA_GERAL.md](ESTRUTURA_GERAL.md)** — Apanhado geral do projeto, arquitetura em alto nível, estrutura de diretórios e fluxos principais

### Backend (Go)
- **[backend/ARQUITETURA.md](backend/ARQUITETURA.md)** — Arquitetura do backend, pacotes, modelos de concorrência, tratamento de erros
- **[backend/SERVICOS.md](backend/SERVICOS.md)** — Detalhamento de cada serviço (config, API, OBS, MediaMTX, Orchestrator, Events)
- **[backend/COMUNICACAO.md](backend/COMUNICACAO.md)** — Fluxos de comunicação detalhados (inicialização, sync, reconexão, shutdown)

### Frontend (Angular)
- **[frontend/ARQUITETURA.md](frontend/ARQUITETURA.md)** — Arquitetura do frontend, componentes, signals, reatividade
- **[frontend/SERVICOS.md](frontend/SERVICOS.md)** — Serviços (ApiService, WebSocketService, Interceptor, Models)
- **[frontend/COMUNICACAO.md](frontend/COMUNICACAO.md)** — Fluxos de interação (HTTP requests, WebSocket events, error handling)

---

## 🎯 Por Onde Começar?

**Primeiro acesso?** Comece por:
1. [ESTRUTURA_GERAL.md](ESTRUTURA_GERAL.md) — Entender visão geral
2. [backend/ARQUITETURA.md](backend/ARQUITETURA.md) — Aprender core da lógica
3. [frontend/ARQUITETURA.md](frontend/ARQUITETURA.md) — Entender UI

**Debugar um fluxo específico?**
1. [backend/COMUNICACAO.md](backend/COMUNICACAO.md) — Entender o que backend faz
2. [frontend/COMUNICACAO.md](frontend/COMUNICACAO.md) — Entender o que frontend faz
3. [backend/SERVICOS.md](backend/SERVICOS.md) / [frontend/SERVICOS.md](frontend/SERVICOS.md) — Detalhar implementação

**Adicionar nova feature?**
1. [backend/SERVICOS.md](backend/SERVICOS.md) — Onde adicionar no backend?
2. [frontend/SERVICOS.md](frontend/SERVICOS.md) — Onde adicionar no frontend?
3. [backend/COMUNICACAO.md](backend/COMUNICACAO.md) — Como integrar ao fluxo?

---

## 🏗️ Estrutura Rápida

### Backend
```
cmd/server/main.go          Entry point
internal/
  config/                   Variáveis de ambiente
  httpapi/                  Endpoints REST + WebSocket
  mediaserver/              Cliente MediaMTX
  obs/                      Cliente OBS Studio
  orchestrator/             Core da lógica
  events/                   Pub/sub para eventos
```

### Frontend
```
app.ts                      Componente raiz
core/
  models.ts                 Tipos TypeScript
  api.service.ts            Serviço HTTP
  api-token.interceptor.ts  Token no header
  websocket.service.ts      Conexão WebSocket
features/
  camera-grid/              Grid de câmeras
  control-bar/              Status do sistema
```

---

## 🔄 Fluxos Principais

### 1. Inicialização
```
Backend starts → MediaMTX + OBS conectam → Sync loop inicia
     ↓
Frontend loads → HTTP snapshot → WebSocket abre
     ↓
Sistema pronto
```

### 2. Sincronização (a cada 3s)
```
Sync loop query MediaMTX → Detecta câmeras → Atualiza OBS
     ↓
Publica evento para WebSocket
     ↓
Frontend recebe → Atualiza UI
```

### 3. Seleção de Câmera (Ao Vivo)
```
User clica card → POST /api/v1/cameras/{id}/live
     ↓
Backend SetLive() → OBS visibilidade
     ↓
Publica event → WebSocket
     ↓
Frontend atualiza UI
```

---

## 📡 Contratos de Dados

### Modelos Compartilhados

**Camera**
```typescript
{
  id: string;
  name: string;
  sourceUrl: string;
  status: "online" | "offline";
  obsSourceCreated: boolean;
  isLive: boolean;
  lastSeenAt: string;
}
```

**SystemStatus**
```typescript
{
  obsConnected: boolean;
  mediaServerConnected: boolean;
  streaming: boolean;
  activeSceneName: string;
  liveCameraId: string;
}
```

---

## 🔗 Dependências Externas

| Serviço | Porta | Protocolo | Usado por |
|---------|-------|-----------|-----------|
| MediaMTX API | 9997 | HTTP | Backend |
| OBS Studio | 4455 | WebSocket | Backend |
| Backend | 8080 | HTTP + WS | Frontend |
| Frontend | 4200 | HTTP (dev) | Navegador |

---

## 🚀 Cheat Sheet — Encontrar Algo

| Você quer... | Veja... |
|-------------|---------|
| Entender como câmeras são descobertas | backend/COMUNICACAO.md — Seção 2 (Sync Loop) |
| Debugar por que camera não aparece | backend/ARQUITETURA.md — Orchestrator |
| Entender fluxo de "set live" | backend/COMUNICACAO.md — Seção 3 |
| Saber por que WebSocket desconecta | frontend/COMUNICACAO.md — Seção 7 |
| Adicionar novo endpoint REST | backend/SERVICOS.md — HTTP API Server |
| Adicionar novo tipo de evento | frontend/SERVICOS.md — WebSocket Service |
| Entender como reconexão funciona | backend/COMUNICACAO.md — Seção 5 (OBS) |
| Implementar novo componente Angular | frontend/ARQUITETURA.md — Componentes |

---

## 📝 Convenções

### Nomes
- **Backend**: `camelCase` (Go)
- **Frontend**: `camelCase` (TypeScript)
- **Endpoints**: `/api/v1/<resource>/<action>`
- **WebSocket event types**: `snake.case`

### Tipos
- Backend e Frontend **mantêm sincronização** de tipos (JSON serialization)
- Modelos espelham estrutura exatamente

### Fluxo
- **Síncrono**: HTTP REST (requisição-resposta)
- **Assíncrono**: WebSocket (push de eventos)

---

## 🧪 Testes

### Backend
```bash
go test ./...              # Todos
go test ./internal/orchestrator  # Pacote específico
```

### Frontend
```bash
npm test                   # Karma + Jasmine
npm run build              # Build de produção
```

---

## 📚 Leitura Recomendada

### Para Backend Engineers
1. [backend/ARQUITETURA.md](backend/ARQUITETURA.md) — Conceitos principais
2. [backend/SERVICOS.md](backend/SERVICOS.md) — Implementação de cada serviço
3. [backend/COMUNICACAO.md](backend/COMUNICACAO.md) — Fluxos em tempo real

### Para Frontend Engineers
1. [frontend/ARQUITETURA.md](frontend/ARQUITETURA.md) — Estrutura Angular
2. [frontend/SERVICOS.md](frontend/SERVICOS.md) — Serviços e tipos
3. [frontend/COMUNICACAO.md](frontend/COMUNICACAO.md) — Interações com usuário

### Para Full-Stack / Product
1. [ESTRUTURA_GERAL.md](ESTRUTURA_GERAL.md) — Visão completa
2. [backend/COMUNICACAO.md](backend/COMUNICACAO.md) — Fluxos críticos
3. [frontend/COMUNICACAO.md](frontend/COMUNICACAO.md) — Experiência do usuário

---

## ❓ FAQ Rápido

**P: Como adicionar suporte a múltiplas cenas?**
A: Ver [backend/ARQUITETURA.md](backend/ARQUITETURA.md) — Extensibilidade

**P: Por que WebSocket em vez de polling?**
A: Eficiência, latência, múltiplos clientes sincronizados. Ver [frontend/SERVICOS.md](frontend/SERVICOS.md)

**P: Onde persistir estado?**
A: Atualmente em memória. Ver [backend/ARQUITETURA.md](backend/ARQUITETURA.md) — Extensibilidade

**P: Como testar sem OBS real?**
A: Usar `obsmock.Mock`. Ver [backend/SERVICOS.md](backend/SERVICOS.md) — OBS Controller

---

## 🔄 Ciclo de Desenvolvimento

```
1. Ler documentação relevante
   ├─ Qual camada? (frontend/backend/ambas)
   ├─ Qual fluxo afeta? (sync, API, UI)
   └─ Quais serviços?

2. Entender fluxo atual
   └─ Ver COMUNICACAO.md apropriada

3. Implementar mudanças
   ├─ Backend: adicionar método, integrar ao fluxo
   ├─ Frontend: consumir via API ou WebSocket
   └─ Testes: unitários + integração

4. Testar ponta a ponta
   ├─ Make dev-backend + dev-frontend
   ├─ Fake cameras se necessário
   └─ WebSocket listener para debug

5. Atualizar documentação
   ├─ Mudança quebra fluxo existente? → Atualizar COMUNICACAO.md
   ├─ Novo serviço? → Adicionar seção em SERVICOS.md
   └─ Arquitetura muda? → Atualizar ARQUITETURA.md + ESTRUTURA_GERAL.md
```

---

## 🎓 Glossário

| Termo | Significado |
|-------|------------|
| **Sync Loop** | Goroutine que roda a cada N segundos sincronizando câmeras |
| **OBS Input** | Fonte de vídeo no OBS Studio (criada para cada câmera) |
| **Live Camera** | A câmera selecionada para transmissão (visível no OBS) |
| **Hub** | Event hub (pub/sub) para distribuir eventos aos clientes |
| **Signal** | Angular Signals — reatividade granular (muda → renderiza) |
| **Envelope** | Mensagem JSON no WebSocket com {type, payload} |
| **Backoff** | Retry com delay crescente (1s, 2s, 4s, ...) |

---

## 📞 Suporte

Dúvidas?
- Revisite a seção apropriada na documentação
- Procure o fluxo correspondente em COMUNICACAO.md
- Cheque exemplos de código em SERVICOS.md

---

**Última atualização**: Julho 2025

Mantido em sincronização com o código-fonte.
