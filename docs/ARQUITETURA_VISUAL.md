# Arquitetura Visual — Como Tudo Se Conecta

## 1️⃣ Visão Geral: O Grande Quadro

```
┌──────────────────────────────────────────────────────────────────┐
│                         A APLICAÇÃO COMPLETA                     │
│                                                                  │
│        Câmeras Enviando Vídeo → Servidor Recebendo → OBS Transmitindo
│                                                                  │
└──────────────────────────────────────────────────────────────────┘

                             Fluxo Visual:

    📱 Smartphone      📹 Webcam      🎥 Câmera IP      📹 Câmera Fake
    (Moblin)          (USB)           (RTSP)            (ffmpeg)
        │                 │                │                 │
        │                 └─────┬──────────┼─────────────────┘
        │                       │          │
        │              (envia vídeo via RTMP - porta 1935)
        │                       │          │
        ▼                       ▼          ▼
    ┌─────────────────────────────────────────┐
    │     MediaMTX (Servidor de Vídeo)       │
    │         Escuta em :1935 (RTMP)         │
    │         API em :9997 (HTTP)            │
    │     (Recebe, armazena, redistribui)    │
    └─────────────────────────────────────────┘
                       │
         (Backend consulta câmeras)
                       │
    ┌──────────────────▼──────────────────┐
    │   BACKEND (Go) Porta :8080          │
    │   • Descobre câmeras                │
    │   • Controla OBS                    │
    │   • Envia atualizações via WebSocket│
    └──────┬──────────────┬───────────────┘
           │              │
        (HTTP)      (WebSocket)
           │              │
    ┌──────▼──┐    ┌──────▼─────────┐
    │   OBS   │    │   NAVEGADOR    │
    │ Studio  │    │   (Angular)    │
    │ :4455   │    │   :4200        │
    │ (Video) │    │ (Controle)     │
    └─────────┘    └────────────────┘
           │              │
           └──────┬───────┘
                  │
            (Transmissão ao vivo)
                  │
            Sua Audiência
         (Twitch/YouTube/etc)
```

---

## 2️⃣ Camada por Camada

### **Camada 1: Câmeras**

```
Fontes de Vídeo:

    ┌─────────────┐  ┌─────────────┐  ┌─────────────┐  ┌──────────┐
    │  Moblin     │  │  Webcam USB │  │ Câmera IP   │  │ ffmpeg   │
    │ (Celular)   │  │  (real)     │  │ (real)      │  │ (fake)   │
    └─────────────┘  └─────────────┘  └─────────────┘  └──────────┘
           │                │                │               │
           │                └────────────────┼───────────────┘
           │                                 │
           └─────────────────────────────────┘
                        │
              (RTMP - Real Time Messaging Protocol)
                        │
        rtmp://host:1935/camera1
        rtmp://host:1935/camera2
        rtmp://host:1935/camera3
                        │
```

### **Camada 2: MediaMTX (Servidor de Vídeo)**

```
┌──────────────────────────────────────────────────────────┐
│                      MEDIAMTX                            │
│              (Servidor de Vídeo em Container)            │
│                                                          │
│   Ports:                                                │
│   • :1935 - RTMP (câmeras enviam vídeo aqui)           │
│   • :8554 - RTSP                                       │
│   • :8890 - SRT                                        │
│   • :9997 - API HTTP (backend consulta aqui)          │
│                                                          │
│   Funciona:                                            │
│   ┌─────────────────────────────────────────────────┐  │
│   │ Câmera 1 [vídeo] ──┐                           │  │
│   │ Câmera 2 [vídeo] ──┼─→ Recebe e Redistribui   │  │
│   │ Câmera 3 [vídeo] ──┘                           │  │
│   │                    (elas ficam aqui em loop)   │  │
│   └─────────────────────────────────────────────────┘  │
│                    │                                    │
│           Lista: /v3/paths/list (API)                 │
│                    │                                    │
│        Retorna: [ camera1, camera2, camera3 ]         │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### **Camada 3: Backend (Go)**

```
┌─────────────────────────────────────────────────────────────────┐
│                      BACKEND (Go)                              │
│              Porta :8080 (HTTP + WebSocket)                    │
│                                                                 │
│  ┌─────────────────────────────────────────────────────────┐   │
│  │  ORQUESTRADOR (o maestro)                              │   │
│  │  • Mantém lista de câmeras em memória                  │   │
│  │  • Sincroniza a cada 3 segundos                        │   │
│  │  • Controla qual câmera vai ao ar                      │   │
│  │  • Publica eventos (câmera nova, câmera caiu, etc)     │   │
│  └─────────────────────────────────────────────────────────┘   │
│           │              │              │                      │
│    (pergunta)    (controla)    (publica)                       │
│           │              │              │                      │
│  ┌────────▼──┐  ┌────────▼──┐  ┌───────▼──────┐                │
│  │ MediaMTX  │  │    OBS    │  │  WebSocket   │                │
│  │  Client   │  │ Controller│  │     Hub      │                │
│  │           │  │           │  │              │                │
│  │ "Quais    │  │ "Cria     │  │ "Publica:    │                │
│  │  câmeras  │  │  input"   │  │  câmera nova"│                │
│  │  têm?"    │  │ "Mostra   │  │  "câmera     │                │
│  │           │  │  câmera"  │  │   ao vivo"   │                │
│  └───────────┘  └───────────┘  └──────────────┘                │
│                                      │                         │
│                                 Para navegador                 │
│                                                                 │
└─────────────────────────────────────────────────────────────────┘
```

### **Camada 4: Frontend (Angular)**

```
┌──────────────────────────────────────────────────────────┐
│              FRONTEND (Angular)                          │
│              Porta :4200 (Navegador)                     │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  App Component (Tela Principal)                    │ │
│  │  • Mostra barra de controle (topo)                │ │
│  │  • Mostra grade de câmeras (centro)               │ │
│  │  • Mostra erros/avisos (topo/banner)              │ │
│  └────────────────────────────────────────────────────┘ │
│         │                     │                         │
│  ┌──────▼────────┐    ┌───────▼──────────────┐          │
│  │ Control Bar   │    │ Camera Grid         │          │
│  │               │    │                     │          │
│  │ Mostra:       │    │ Mostra:             │          │
│  │ • OBS: 🟢/🔴 │    │ • Cartão da câmera  │          │
│  │ • MediaMTX    │    │ • Status (online)   │          │
│  │ • Câmera ao   │    │ • Botão "Colocar    │          │
│  │   vivo        │    │   no ar"            │          │
│  │ • WebSocket   │    │ • Destaque se está  │          │
│  │   status      │    │   ao vivo           │          │
│  └───────────────┘    └─────────────────────┘          │
│                                                          │
│  ┌────────────────────────────────────────────────────┐ │
│  │  Serviços                                          │ │
│  │  • WebSocket Service (conexão permanente)         │ │
│  │  • API Service (requisições HTTP)                 │ │
│  │  • Models (tipos de dados)                        │ │
│  └────────────────────────────────────────────────────┘ │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

### **Camada 5: OBS Studio**

```
┌──────────────────────────────────────────────────────────┐
│              OBS STUDIO (Programa de Transmissão)        │
│              Porta :4455 (WebSocket)                     │
│                                                          │
│  ┌───────────────────────────────────────────────────┐  │
│  │  Cena "Program"                                  │  │
│  │  ┌─────────────────────────────────────────────┐ │  │
│  │  │  Inputs (Câmeras Criadas Automaticamente)   │ │  │
│  │  │                                             │ │  │
│  │  │  cam_camera1  [visível]  ← MOSTRANDO       │ │  │
│  │  │  cam_camera2  [oculto]                     │ │  │
│  │  │  cam_camera3  [oculto]                     │ │  │
│  │  │  manual_camera [oculto]  (você criou)     │ │  │
│  │  │                                             │ │  │
│  │  │  (um de cada vez visível, resto oculto)   │ │  │
│  │  └─────────────────────────────────────────────┘ │  │
│  │                 │                                 │  │
│  │          (o que aparece aqui)                    │  │
│  │                 │                                 │  │
│  │        Transmite para sua audiência              │  │
│  │                                                  │  │
│  └──────────────────────────────────────────────────┘  │
│                                                          │
└──────────────────────────────────────────────────────────┘
```

---

## 3️⃣ O Fluxo de Uma Ação

### **Ação: Você clica "Colocar no ar" em Camera 2**

```
1. NAVEGADOR
   └─→ Você clica no botão
       │
       ▼
2. FRONTEND (Angular)
   └─→ Camera Card Component emite evento
       │
       ▼
3. APP COMPONENT (Angular)
   └─→ Chama api.setLive('camera2')
       │
       ▼
4. API SERVICE (Angular)
   └─→ POST /api/v1/cameras/camera2/live
       │ (com header X-Api-Token)
       ▼
5. BACKEND HTTP API (Go)
   └─→ Recebe requisição
       │
       ▼
6. ORQUESTRADOR (Go)
   └─→ SetLive('camera2')
       │
       ├─→ Valida: câmera existe? está online?
       │
       ▼
7. OBS CONTROLLER (Go)
   └─→ SetOnlyVisibleSource('Program', 'cam_camera2')
       │
       ├─→ Esconde cam_camera1
       ├─→ Esconde cam_camera3
       │
       ▼
8. OBS STUDIO (WebSocket)
   └─→ Scene item visibility muda
       │
       ├─→ cam_camera2 fica visível
       ├─→ outras ficam ocultas
       │
       ▼
9. BACKEND (Go)
   └─→ Publica eventos via WebSocket:
       │
       ├─→ "cameras.updated" (lista atualizada)
       ├─→ "system.status" (câmera ao vivo mudou)
       │
       ▼
10. FRONTEND (Angular)
    └─→ WebSocket Service recebe eventos
        │
        ├─→ Atualiza signal 'cameras'
        ├─→ Atualiza signal 'systemStatus'
        │
        ▼
11. COMPONENTES (Angular)
    └─→ Percebem mudança e redesenham
        │
        ├─→ Camera Grid atualiza visual
        ├─→ Control Bar atualiza informações
        │
        ▼
12. NAVEGADOR
    └─→ Você vê:
        ├─→ Camera 2 com destaque azul
        ├─→ Barra mostra "Ao vivo: camera2"
        
        E no OBS, você vê:
        └─→ Camera 2 aparecendo na transmissão! 🎉

        Tudo isto em <1 segundo!
```

---

## 4️⃣ Loop de Sincronização (A cada 3 segundos)

```
Backend:

┌─────────────────────────────────────────┐
│  Ticker: A cada 3 segundos              │
└─────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│  Pergunta ao MediaMTX:                  │
│  "GET /v3/paths/list"                  │
│  Quais câmeras estão transmitindo?      │
└─────────────────────────────────────────┘
              │
              ▼ (Resposta)
         [camera1, camera2, camera3]
              │
              ▼
┌─────────────────────────────────────────┐
│  Compara com o que tínhamos:            │
│  Antes: [camera1, camera2]              │
│  Agora: [camera1, camera2, camera3]     │
│                                         │
│  Diferença: camera3 é nova!             │
└─────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│  Cria input no OBS:                     │
│  "cam_camera3"                          │
│  Aponta para:                           │
│  {MEDIA_SOURCE_BASE_URL}/camera3        │
└─────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│  Publica eventos:                       │
│  - "cameras.updated"                    │
│  - "system.status"                      │
└─────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│  Frontend recebe e redesenha            │
│  Navegador mostra camera3 como Online   │
└─────────────────────────────────────────┘
```

---

## 5️⃣ Reconexão Automática

### **Se OBS desconectar:**

```
Backend (a cada 5 segundos):

┌─────────────────────────────────────────┐
│  Health Check                           │
│  Tenta: General.GetVersion()            │
└─────────────────────────────────────────┘
              │
              ▼
      ❌ Falha!
              │
              ▼
┌─────────────────────────────────────────┐
│  Backoff Exponencial                    │
│  Espera: 1s, 2s, 4s, 8s, 16s, 30s...   │
└─────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│  Tenta reconectar a cada intervalo      │
└─────────────────────────────────────────┘
              │
              ▼
      ✅ Conectou!
              │
              ▼
┌─────────────────────────────────────────┐
│  Volta ao normal (health check a cada 5s)
└─────────────────────────────────────────┘
```

### **Se Frontend perder conexão WebSocket:**

```
Frontend (Angular):

┌─────────────────────────────────────────┐
│  WebSocket.close()                      │
│  Tela mostra:                           │
│  "Sem conexão com o backend"            │
└─────────────────────────────────────────┘
              │
              ▼
┌─────────────────────────────────────────┐
│  Reconexão Automática                   │
│  Espera: 1s, 2s, 4s, 8s... (até 30s)  │
└─────────────────────────────────────────┘
              │
              ▼
      ✅ Conectou!
              │
              ▼
┌─────────────────────────────────────────┐
│  Tela volta ao normal                   │
│  Recebe lista de câmeras                │
└─────────────────────────────────────────┘
```

---

## 6️⃣ Estrutura de Dados

### **Uma Câmera (JSON)**

```json
{
  "id": "camera1",                            // Identificador
  "name": "camera1",                          // Nome legível
  "sourceUrl": "rtmp://localhost:1935/camera1",  // Link da câmera (base configurável via MEDIA_SOURCE_BASE_URL)
  "status": "online",                         // online ou offline
  "obsSourceCreated": true,                   // Já criou no OBS?
  "isLive": true,                             // Está transmitindo?
  "lastSeenAt": "2024-07-17T10:00:00Z"       // Último avistamento
}
```

### **Status do Sistema (JSON)**

```json
{
  "obsConnected": true,                   // OBS ligado?
  "mediaServerConnected": true,           // MediaMTX ligado?
  "streaming": true,                      // Tem transmissão?
  "activeSceneName": "Program",           // Nome da cena
  "liveCameraId": "camera1"               // Qual câmera ao vivo
}
```

### **Mensagem WebSocket**

```json
{
  "type": "cameras.updated|system.status|error",
  "payload": { ... dados específicos ... }
}
```

---

## 7️⃣ Conexões de Rede

```
Seu Computador (localhost):

┌─────────────────────────────────────────┐
│  Frontend     :4200                     │
│  Backend      :8080                     │
│  OBS          :4455  (WebSocket)        │
│  MediaMTX     :1935  (RTMP)             │
│  MediaMTX     :9997  (API HTTP)         │
└─────────────────────────────────────────┘

Se Câmeras Reais (mesmo WiFi):

┌──────────────────────────┐
│  Seu Smartphone          │
│  Moblin App              │
│  Envia para:             │
│  rtmp://seu-ip:1935/...  │
└──────────────────────────┘
         │
         │ (WiFi local)
         │
         ▼
    ┌──────────────┐
    │ MediaMTX     │
    │ :1935 (RTMP) │
    └──────────────┘
```

---

## 8️⃣ Linha do Tempo: Startup

```
Tempo    |  Ação
---------|----------------------------------------------------------
0s       | Você digita: make dev-backend
         |
1s       | Backend inicia
         | ├─ Conecta ao MediaMTX
         | ├─ Conecta ao OBS (WebSocket)
         | └─ Cria cena "Program" no OBS
         |
2s       | Backend começa loop de sincronização
         | ├─ Pergunta ao MediaMTX: quais câmeras?
         | └─ Resposta: nenhuma ainda (câmeras fake não foram ligadas)
         |
2.5s     | Você digita em outro terminal: make fake-camera NAME=camera1
         |
3s       | ffmpeg começa a enviar vídeo para:
         | rtmp://localhost:1935/camera1
         |
4s       | Backend sincroniza (a cada 3s)
         | ├─ Pergunta ao MediaMTX
         | ├─ Resposta: camera1 está lá!
         | └─ Cria input "cam_camera1" no OBS
         |
5s       | Frontend abre: http://localhost:4200
         | ├─ Angular inicia
         | └─ Conecta WebSocket ao backend
         |
6s       | Backend envia via WebSocket:
         | ├─ "cameras.updated": [camera1]
         | └─ "system.status": { ... }
         |
7s       | Tela do navegador mostra:
         | └─ Camera1 "Online" com botão "Colocar no ar"
         |
         | 🎉 Pronto para usar!
```

---

## 9️⃣ Estados das Câmeras

```
               ┌─────────────────┐
               │  NÃO EXISTE     │
               │  (não viu ainda)│
               └────────┬────────┘
                        │
         (aparece no MediaMTX)
                        │
                        ▼
               ┌─────────────────┐
               │    ONLINE       │
               │  (transmitindo) │ ◄── Aqui você pode
               │  OBS: criando   │     "Colocar no ar"
               └────────┬────────┘
                        │
        (desaparece por >60s)
                        │
                        ▼
               ┌─────────────────┐
               │   OFFLINE       │
               │ (internet caiu) │ ◄── Aqui NÃO pode
               │ (estamos aguard-│     "Colocar no ar"
               │  ando reconexão)│
               └────────┬────────┘
                        │
      (reconecta ou >60s sem sinal)
                        │
                        ▼
               ┌─────────────────┐
               │  REMOVIDO       │
               │  (do OBS)       │
               │  (da memória)   │
               └─────────────────┘
```

---

## 🔟 Fluxo de Dados Completo

```
📱 CÂMERAS
   │
   ├─ Moblin (smartphone)
   ├─ ffmpeg (câmera fake)
   ├─ Webcam (real)
   └─ Câmera IP (real)
        │
        │ (RTMP :1935)
        │
        ▼
📺 MEDIAMTX
   │ Recebe todos os streams
   │ Lista por API: /v3/paths/list
        │
        │ (HTTP :9997)
        │
        ▼
🖥️ BACKEND (Go)
   │ ├─ Descobre câmeras
   │ ├─ Cria inputs no OBS
   │ └─ Publica eventos
        │
        ├─ REST API (:8080)
        └─ WebSocket (:8080)
                │
        ┌───────┼───────┐
        │               │
        ▼               ▼
    🎬 OBS         🌐 FRONTEND
    (WebSocket)    (Angular)
         │              │
         │ (transmite)  │ (mostra)
         │              │
         ▼              ▼
    📊 LIVE        👁️ VOCÊ
   (Twitch/        (navegador
    YouTube)       vendo câmeras)
```

---

**Resumo:** Tudo é conectado e funciona em tempo real (WebSocket). Quando algo muda em um lugar, todos os outros lugares sabem em milissegundos. 🚀
