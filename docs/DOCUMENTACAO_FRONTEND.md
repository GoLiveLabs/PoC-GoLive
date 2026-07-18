# Documentação do Frontend — A Tela que Você Enxerga

## O que faz o Frontend?

O frontend é a "visão" da aplicação. É o painel que você vê no navegador. Ele mostra:
- Todas as câmeras disponíveis
- Qual câmera está ao vivo agora
- O estado da conexão com o backend
- Mensagens de erro

## Como funciona, passo a passo?

### 1. **Quando você acessa `http://localhost:4200`**

1. O navegador carrega a página Angular
2. Frontend tenta se conectar ao backend via WebSocket
3. Enquanto conecta, mostra "Conectando..."
4. Se conectar, pede a lista de câmeras e o estado do sistema

### 2. **Conectando com o Backend (WebSocket)**

O frontend abre uma conexão permanente com o backend:

```
Navegador → Backend (conexão aberta, sempre)
```

**Por que permanente?** Para receber atualizações em tempo real. Quando algo muda no backend (nova câmera, câmera desconectada, câmera foi ao ar), o backend avisa imediatamente.

**Se a conexão cair:**
- Frontend tenta reconectar automaticamente
- Começa esperando 1 segundo
- Se falhar, espera 2 segundos
- Depois 4, depois 8... até 30 segundos
- Continua tentando eternamente (até você fechar o navegador)

### 3. **Recebendo atualizações**

O backend envia mensagens em formato JSON:

**Mensagem de câmeras:**
```json
{
  "type": "cameras.updated",
  "payload": [
    {
      "id": "camera1",
      "name": "camera1",
      "status": "online",
      "isLive": true,
      ...
    }
  ]
}
```

**Mensagem de status do sistema:**
```json
{
  "type": "system.status",
  "payload": {
    "obsConnected": true,
    "mediaServerConnected": true,
    "streaming": true,
    "liveCameraId": "camera1"
  }
}
```

**Mensagem de erro:**
```json
{
  "type": "error",
  "payload": {
    "message": "A câmera caiu, escolha outra"
  }
}
```

### 4. **Quando você clica "Colocar no ar"**

1. Frontend envia um pedido para o backend:
   ```
   POST /api/v1/cameras/camera1/live
   ```

2. Backend processa e responde com o novo estado

3. Frontend atualiza a tela

## As partes principais do Frontend

### **1. Serviço de API (api.service.ts)**

É o "telefone" que liga para o backend pedindo informações.

**Faz:**
- Pede lista de câmeras
- Pede o status do sistema
- Envia pedido para mudar qual câmera vai ao ar
- Força uma sincronização imediata (se necessário)

**Requisições HTTP que faz:**
- `GET /api/v1/cameras` — lista de câmeras
- `GET /api/v1/status` — estado do sistema
- `POST /api/v1/cameras/:id/live` — coloca câmera ao vivo
- `POST /api/v1/sync` — força sincronização

### **2. Serviço de WebSocket (websocket.service.ts)**

É o "rádio" que fica escutando o backend.

**Faz:**
- Abre a conexão com o backend
- Recebe mensagens do backend em tempo real
- Processa as mensagens e atualiza as "variáveis reativas" (signals)
- Reconecta automaticamente se a conexão cair

**Signals (variáveis que atualizam a tela):**
- `cameras` — lista de câmeras
- `systemStatus` — estado do sistema
- `connectionState` — 'conectando' / 'aberto' / 'fechado'
- `lastError` — última mensagem de erro

### **3. Componentes (telas)**

#### **App Component (app.ts)**
É a tela principal que junta tudo.

**Faz:**
- Conecta ao WebSocket quando abre
- Desconecta quando fecha
- Mostra a barra de controle (topo)
- Mostra a grade de câmeras (centro)
- Mostra mensagens de erro (se houver)
- Mostra aviso de desconexão (se perder conexão)

#### **Barra de Controle (control-bar.component.ts)**
É o "painel de informações" no topo.

**Mostra:**
- Se OBS está conectado (com cor verde ou vermelha)
- Se MediaMTX está conectado
- Se tem transmissão ao vivo
- Nome da câmera ao vivo
- Botão "Sincronizar" (força verificação imediata)
- Estado da conexão WebSocket

#### **Grade de Câmeras (camera-grid.component.ts)**
É o "catálogo" de câmeras.

**Faz:**
- Mostra cada câmera em um cartão
- Ordena câmeras por nome
- Passa para cada cartão as informações dele

#### **Cartão de Câmera (camera-card.component.ts)**
É cada quadradinho da grade.

**Mostra:**
- Foto/ícone da câmera
- Nome da câmera
- Status (Online/Offline)
- Botão "Colocar no ar"
- Destaque visual se a câmera está ao vivo

**Cores:**
- Verde = Online
- Cinza = Offline
- Azul/destaque = Está ao vivo agora

### **4. Modelos de Dados (core/models.ts)**

Define o "formato" de cada coisa que se passa:

```typescript
// Uma câmera
interface Camera {
  id: string;              // Nome da câmera
  name: string;            // Nome legível
  sourceUrl: string;       // Link do vídeo
  status: 'online' | 'offline';
  obsSourceCreated: boolean;  // Já criada no OBS?
  isLive: boolean;         // Está ao vivo?
  lastSeenAt: string;      // Quando foi vista por último
}

// Estado do sistema
interface SystemStatus {
  obsConnected: boolean;          // OBS conectado?
  mediaServerConnected: boolean;  // MediaMTX conectado?
  streaming: boolean;             // Tem transmissão?
  activeSceneName: string;        // Nome da cena
  liveCameraId: string;           // Qual câmera está ao vivo
}
```

### **5. Interceptador de API (core/api-token.interceptor.ts)**

É um "carteiro" que adiciona o token de segurança em toda requisição.

**Faz:**
- Pega o token da configuração
- Adiciona o header `X-Api-Token` em toda requisição HTTP
- Garante que o backend reconheça a requisição

## O layout da tela

```
┌─────────────────────────────────────────────────┐
│  Barra de Controle                              │
│  [OBS: 🟢] [MediaMTX: 🟢] [Ao vivo: camera1]   │
│  [Sincronizar] [Conexão: aberto]                │
└─────────────────────────────────────────────────┘

┌─────────────────────────────────────────────────┐
│                 Grade de Câmeras                 │
│                                                 │
│  ┌──────────────┐  ┌──────────────┐            │
│  │ Camera1      │  │ Camera2      │            │
│  │ Online  ✓    │  │ Online       │            │
│  │ Ao vivo ✨   │  │              │            │
│  │ [Colocar]    │  │ [Colocar]    │            │
│  └──────────────┘  └──────────────┘            │
│                                                 │
│  ┌──────────────┐  ┌──────────────┐            │
│  │ Camera3      │  │ Camera4      │            │
│  │ Offline      │  │ Offline      │            │
│  │              │  │              │            │
│  │ [Colocar]    │  │ [Colocar]    │            │
│  └──────────────┘  └──────────────┘            │
│                                                 │
└─────────────────────────────────────────────────┘

(Mensagens de erro aparecem aqui no topo)
```

## Como os dados fluem

### **Fluxo 1: Você abre a página**

```
1. Navegador carrega a página
2. Angular inicia o App Component
3. App Component conecta ao WebSocket
4. WebSocket abre conexão com backend
5. Backend envia: "Aqui estão as câmeras: [camera1, camera2]"
6. WebSocket recebe e atualiza o signal "cameras"
7. Componentes percebem mudança e redesenham a tela
8. Você vê a grade de câmeras
```

### **Fluxo 2: Você clica "Colocar no ar"**

```
1. Você clica no botão
2. CameraCard Component emite um evento com o ID
3. App Component recebe e chama api.setLive(camera1)
4. ApiService faz POST para /cameras/camera1/live
5. Backend processa (esconde outras, mostra essa)
6. Backend envia via WebSocket: "Status mudou! Camera1 agora está ao vivo"
7. WebSocket recebe e atualiza os signals
8. Tela redesenha com o novo status
9. Você vê a câmera marcada como "Ao vivo"
```

### **Fluxo 3: Câmera desconecta no backend**

```
1. Backend detecta que câmera desapareceu
2. Backend envia via WebSocket: "Câmeras: [camera2, camera3]"
3. WebSocket atualiza o signal "cameras"
4. Tela redesenha
5. Você vê camera1 desaparecida
```

## Configurações

No arquivo `environments/environment.ts`:

```typescript
export const environment = {
  apiBaseUrl: 'http://localhost:8080/api/v1',  // Endereço do backend
  wsUrl: 'ws://localhost:8080/api/v1/ws',      // WebSocket do backend
  apiToken: 'dev-token'                         // Token de segurança
};
```

Se você mudar o endereço do backend, mude aqui também.

## Responsividade

A tela foi feita para funcionar bem em:
- Desktop (navegador grande)
- Tablet (navegador médio)
- Celular (navegador pequeno)

A grade de câmeras se adapta automaticamente ao tamanho da tela.

## Tratamento de Erros

### **Sem conexão com o backend**

Frontend mostra:
```
Sem conexão com o backend. Tentando reconectar...
```

E continua tentando reconectar.

### **Câmera caiu enquanto estava ao vivo**

Backend envia erro, frontend mostra:
```
A câmera ao vivo ficou offline. Escolha outra câmera para o ar.
```

Você precisa clicar em outra câmera.

### **OBS desconectou**

Backend envia status atualizado, você vê na barra:
```
[OBS: 🔴] (está desconectado)
```

Reconecta automaticamente.

## Signals (Reatividade Angular)

O frontend usa "signals" que são variáveis especiais que:
- Notificam todos os componentes quando mudam
- Não precisa de subscriptions manuais
- Atualiza a tela automaticamente

Exemplo:
```typescript
readonly cameras = signal<Camera[]>([]);  // Vazia no início

// Quando algo muda:
this.cameras.set([camera1, camera2]);  // Atualiza

// Componentes veem automaticamente:
@Component({ ... })
export class MyComponent {
  cameras = input.required<Camera[]>();  // Recebe a atualização
}
```

## Estilo e Cores

O frontend usa CSS puro (sem Tailwind):

**Cores principais:**
- Verde (`#4CAF50`) — Online, ativo
- Cinza (`#9E9E9E`) — Offline
- Azul (`#2196F3`) — Destaque, ao vivo
- Vermelho (`#F44336`) — Erro
- Amarelo (`#FF9800`) — Aviso

**Fontes:**
- Sistema padrão (Arial, sans-serif)
- Limpo e legível

## Tecnologias usadas

- **Angular 21** — framework para a tela
- **TypeScript** — linguagem (JavaScript tipado)
- **RxJS** — para requisições HTTP
- **CSS** — estilo puro

## Dicas importantes

### **O frontend não salva nada**

Quando você recarrega a página, tudo volta ao normal. Não há "memória" local.

### **Cada navegador é independente**

Se você abrir dois navegadores, ambos veem a mesma informação, mas são conexões separadas. Se o backend cair, ambos desconectam.

### **O front precisa do backend para funcionar**

Sem o backend ligado, você verá "Conectando..." eternamente. Nada funcionará.

### **Token é obrigatório**

Se mudar o token no backend (variável `API_TOKEN`), precisa mudar também no frontend (`environment.ts`).
