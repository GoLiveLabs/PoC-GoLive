# Visão Geral — O Que é Esta Aplicação?

## Em Uma Frase

Uma **ferramenta para controlar qual câmera está sendo transmitida no OBS quando você tem múltiplas câmeras disponíveis**, sem precisar sair da transmissão ao vivo.

## O Problema que Resolve

### Cenário Tradicional (Sem a Aplicação)

```
Operador está transmitindo uma live...

Produto: "Preciso mudar para a câmera de cima!"

Operador:
1. Para tudo
2. Vai ao OBS
3. Procura a câmera no menu
4. Clica para mostrar
5. Volta para o seu programa
6. Volta ao ar

Audiência: Viu um "borrão" preto na transmissão
```

### Com Esta Aplicação

```
Operador está transmitindo uma live no OBS...

Produto: "Preciso mudar para a câmera de cima!"

Operador:
1. Abre uma aba do navegador (que estava aberta)
2. Clica em um botão
3. PRONTO

Audiência: Não viu nada diferente, câmera mudou suavemente
```

## Como Funciona (Visão Geral)

### **A Arquitetura em Camadas**

```
┌─────────────────────────────────────────────────┐
│        VOCÊ (Navegador)                         │
│    Vê as câmeras em cards                       │
│    Clica em "Colocar no ar"                     │
└─────────────────────────────────────────────────┘
                      ↑ ↓
┌─────────────────────────────────────────────────┐
│    BACKEND (Go) — O "maestro"                  │
│  - Descobre câmeras                             │
│  - Controla o OBS                               │
│  - Envia atualizações em tempo real             │
└─────────────────────────────────────────────────┘
         ↑        ↑        ↑
         │        │        │
     CÂMERAS   OBS STUDIO  MEDIAMTX
                           (servidor vídeo)
```

### **Os 3 Atores Principais**

#### **1. MediaMTX** (Servidor de Vídeo)

**O que faz:** Recebe transmissões de câmeras (via RTMP), armazena e redistribui.

**Analogia:** Um "hub" de conectar várias câmeras em um só lugar.

**Exemplo de câmeras chegando:**
- Câmera 1 (ffmpeg) → `rtmp://localhost:1935/camera1`
- Câmera 2 (Moblin) → `rtmp://localhost:1935/camera2`
- Câmera 3 (real) → `rtmp://localhost:1935/camera3`

#### **2. Backend** (Go)

**O que faz:**
- Fala com MediaMTX: "Quais câmeras estão aí?"
- Fala com OBS: "Cria um input dessa câmera"
- Fala com o Navegador: "A câmera X apareceu!"

**Analogia:** Um "gerenciador" que traduz entre câmeras e OBS.

#### **3. Frontend** (Navegador)

**O que faz:**
- Mostra as câmeras disponíveis
- Deixa você clicar para trocar
- Mantém você informado sobre o estado

**Analogia:** Um "painel de controle" visual e fácil.

## O Fluxo de Uma Transmissão

### **Minuto 0: Preparação**

```
Você liga tudo:
1. MediaMTX (docker-compose)
2. Backend (Go)
3. Frontend (Angular no navegador)
4. OBS Studio

Câmera 1 envia vídeo → MediaMTX
Câmera 2 envia vídeo → MediaMTX

Backend descobre: "Tem 2 câmeras!"
Backend cria no OBS: "cam_camera1" e "cam_camera2"
```

### **Minuto 1-2: Transmissão começa**

```
Você no navegador:
- Vê 2 câmeras "Online"
- Clica em "Colocar no ar" na câmera 1

Backend:
- Recebe pedido
- Diz ao OBS: "Esconde cam_camera2"
- Diz ao OBS: "Mostra cam_camera1"

OBS:
- Câmera 1 aparece na cena "Program"

Você transmite para a audiência
```

### **Minuto 5: Precisa mudar de câmera**

```
Você no navegador:
- Clica em "Colocar no ar" na câmera 2

Tudo muda em menos de 1 segundo na transmissão
Audiência quase não nota
```

## Componentes Técnicos

### **Backend (Go)**

**Arquivo principal:** `cmd/server/main.go`

**Responsabilidades:**
- HTTP API (endpoints para o navegador)
- WebSocket (mensagens em tempo real)
- Sincronização com MediaMTX
- Controle do OBS
- Reconexão automática

**Linguagem:** Go (rápido, simples)

### **Frontend (Angular)**

**Pasta principal:** `frontend/src/app/`

**Responsabilidades:**
- Tela visual
- Conexão WebSocket com backend
- Requisições HTTP
- Atualização em tempo real

**Linguagem:** TypeScript + Angular

### **MediaMTX (Docker)**

**Configuração:** `docker-compose.yml` + `mediamtx.yml`

**Responsabilidades:**
- Receber vídeos das câmeras
- Disponibilizar API para listar câmeras
- Manter os vídeos fluindo

**Linguagem:** Go (terceiros, não você escreve)

## Dados que Trafegam

### **Uma Câmera (objeto)**

```json
{
  "id": "camera1",
  "name": "camera1",
  "sourceUrl": "rtmp://localhost:1935/camera1",
  "status": "online",
  "obsSourceCreated": true,
  "isLive": true,
  "lastSeenAt": "2024-07-17T10:00:00Z"
}
```

### **Mensagens WebSocket** (em tempo real)

```json
{
  "type": "cameras.updated",
  "payload": [... lista de câmeras ...]
}

{
  "type": "system.status",
  "payload": { "obsConnected": true, ... }
}

{
  "type": "error",
  "payload": { "message": "Câmera caiu" }
}
```

## Limites Conhecidos (Versão Atual)

Esta é uma **versão inicial (PoC)**, então tem:

- ✅ Múltiplas câmeras
- ✅ Alternância entre câmeras (suave)
- ✅ Reconexão automática (OBS e MediaMTX)
- ✅ Time real (WebSocket)
- ✅ Responsivo (Mobile/Desktop)

Mas **não tem:**

- ❌ Autenticação de usuários (usa token fixo)
- ❌ Salvar estado (memória volátil)
- ❌ Preview de vídeo no painel (planejado)
- ❌ Múltiplas cenas/layouts (apenas "Program")
- ❌ Deploy automatizado (apenas local)

## Tecnologias Usadas

| Camada | Tecnologia | Por quê |
|--------|----------|--------|
| **Backend** | Go 1.22 | Rápido, fácil, bom para networked apps |
| **Frontend** | Angular 21 | Reativo, time-real com signals |
| **Vídeo** | MediaMTX | RTMP/RTSP, moderno, sem dependências pesadas |
| **WebSocket** | Go nativo + Angular | Tempo real garantido |
| **Deploy** | Docker Compose | Prototipagem rápida |
| **Testes** | Go test + Vitest | Padrão dos frameworks |

## Como Começa Aqui

### **Se quer entender o COMO:**

1. Leia `DOCUMENTACAO_BACKEND.md` (como funciona o maestro)
2. Leia `DOCUMENTACAO_FRONTEND.md` (como funciona a tela)

### **Se quer USAR agora:**

1. Siga `GUIA_USO.md` (passo a passo completo)
2. Pega algumas câmeras fake
3. Brinca com a interface

### **Se quer usar CÂMERAS REAIS (Moblin):**

1. Siga `GUIA_OBS_MOBLIN.md` (integração com smartphone)

### **Se quer ver o CÓDIGO:**

1. `backend/` — tudo o que faz (Go)
2. `frontend/src/` — interface (TypeScript + Angular)

## Analogias Úteis

### **Analogia 1: Videoclube**

- **MediaMTX** = Videoclube (tem vários filmes = câmeras)
- **Backend** = Gerente do cinema (sabe qual filme está rodando, controla qual sala mostra qual)
- **Frontend** = Cartaz externo (mostra para o público qual filme está rodando)
- **OBS** = A sala de cinema (mostra o filme pro público)

### **Analogia 2: Restaurante**

- **Câmeras** = Cozinhas
- **MediaMTX** = Servidor de pedidos
- **Backend** = Gerente da cozinha
- **OBS** = Prato que sai pro cliente
- **Frontend** = Menu digital

### **Analogia 3: Concerto**

- **Câmeras** = Músicos (saxofone, violão, bateria)
- **MediaMTX** = Palco (lugar onde todos tocam)
- **Backend** = Maestro (diz qual instrumento toca agora)
- **Frontend** = Placar (mostra qual instrumento está em destaque)
- **OBS** = Auditório (o público ouve/vê)

## Por que tudo isto?

Antes, se você queria transmitir com múltiplas câmeras:

1. Ou contrata alguém (caro)
2. Ou tira o OBS da tela (errado)
3. Ou usa cenas fixas (sem flexibilidade)

Agora:

1. Um único operador
2. Sem sair da transmissão
3. Interface fácil
4. Câmeras descobertas automaticamente
5. Reconexão automática
6. Muda em tempo real

## Próximas Iterações (Roadmap)

Com base no plano original, as próximas fases seriam:

- **Fase 3:** Preview de vídeo nos cards (HLS stream)
- **Fase 4:** Suporte a múltiplas cenas/layouts
- **Fase 5:** Persistência de dados (banco de dados)
- **Fase 6:** Deploy em produção (Kubernetes/containers)
- **Fase 7:** Autenticação de usuários
- **Fase 8:** Analytics e logs

Mas isso é futuro. Por enquanto, você tem o MVP (Minimum Viable Product).

## Resumo

```
┌──────────────────────────────────────────────┐
│ Live Orchestrator - Orquestrador de Câmeras │
│                                              │
│ Transforma várias câmeras em uma transmissão│
│ suave e profissional                        │
│                                              │
│ Fácil de usar: Clica em um botão            │
│ Rápido: Muda em <1s                        │
│ Automático: Descoberta de câmeras           │
│ Resiliente: Reconecta sozinho               │
│                                              │
│ Stack: Go + Angular + MediaMTX + Docker     │
└──────────────────────────────────────────────┘
```

## Agora Que Sabe o Que É...

- Quer testar? → Vá para `GUIA_USO.md`
- Quer entender o código? → Vá para `DOCUMENTACAO_BACKEND.md` e `DOCUMENTACAO_FRONTEND.md`
- Quer usar Moblin? → Vá para `GUIA_OBS_MOBLIN.md`
- Quer as decisões técnicas? → Vá para `DECISIONS.md` (na raiz)
