# Resumo Executivo — O Essencial em 2 Páginas

## O Que É

Uma ferramenta que controla qual câmera está sendo transmitida no OBS quando você tem múltiplas câmeras, sem precisar mexer no OBS durante a transmissão.

## Problema Resolvido

**Antes:** Operador tem que parar a transmissão, mexer no OBS, voltar ao ar.  
**Depois:** Operador clica em um botão no navegador, câmera muda instantaneamente.

## Stack Tecnológico

- **Backend:** Go (maestro que coordena tudo)
- **Frontend:** Angular (painel web simples)
- **Vídeo:** MediaMTX (recebe câmeras)
- **Dados:** WebSocket (tempo real)
- **Deploy:** Docker (MediaMTX)

## Como Funciona

```
Câmeras → MediaMTX → Backend → OBS → Audiência
                        ↑
                    Você controla
                    via navegador
```

**Passos:**
1. Câmeras enviam vídeo via RTMP para MediaMTX
2. Backend descobre câmeras (a cada 3s)
3. Backend cria inputs no OBS automaticamente
4. Você abre navegador e vê as câmeras
5. Você clica "Colocar no ar" em uma câmera
6. Backend torna visível no OBS, oculta as outras
7. Audiência vê a câmera mudar suavemente

## Começar em 10 Minutos

Terminal 1:
```bash
make mediamtx-up
```

Terminal 2-3:
```bash
make fake-camera NAME=camera1
make fake-camera NAME=camera2
```

Terminal 4:
```bash
make dev-backend
```

Terminal 5:
```bash
make dev-frontend
# Abre http://localhost:4200
```

Clica em "Colocar no ar" em qualquer câmera. Pronto!

## Arquitetura

### Backend (Go)
- **Orquestrador:** Descobre câmeras, controla OBS
- **MediaServer Client:** Fala com MediaMTX
- **OBS Controller:** Cria/mostra/esconde inputs
- **HTTP API:** Endpoints REST
- **WebSocket Hub:** Envia atualizações em tempo real

### Frontend (Angular)
- **WebSocket Service:** Conexão permanente com backend
- **API Service:** Requisições HTTP
- **Componentes:** Tela visual (cards de câmeras)
- **Signals:** Reatividade (atualiza quando algo muda)

## Dados Principais

**Câmera:**
```json
{
  "id": "camera1",
  "status": "online",
  "isLive": true,
  "lastSeenAt": "2024-07-17T10:00:00Z"
}
```

**Sistema:**
```json
{
  "obsConnected": true,
  "mediaServerConnected": true,
  "streaming": true,
  "liveCameraId": "camera1"
}
```

## Pontos Chave

✅ **Automático:** Descobre câmeras sozinho  
✅ **Rápido:** Muda câmera em <1 segundo  
✅ **Resiliente:** Reconecta automaticamente se OBS/câmera cair  
✅ **Fácil:** Interface web, sem tocar no OBS  
✅ **Múltiplas Câmeras:** Suporta ilimitadas  

❌ **Sem autenticação:** Token fixo (dev)  
❌ **Sem persistência:** Tudo em memória  
❌ **Sem preview:** Só controle, sem imagem

## Variáveis Importantes

| Variável | Padrão | O quê |
|----------|--------|-------|
| HTTP_ADDR | :8080 | Porta backend |
| OBS_ADDR | localhost:4455 | Endereço OBS |
| MEDIAMTX_API_URL | http://localhost:9997 | API MediaMTX |
| MEDIA_SOURCE_BASE_URL | rtmp://localhost:1935 | Base da URL de vídeo das câmeras |
| SYNC_INTERVAL | 3s | Frequência sincronização |
| PROGRAM_SCENE | Program | Nome cena OBS |

## Fluxo de Uma Ação

```
Você clica "Colocar no ar" em camera2
         ↓
Frontend envia POST /cameras/camera2/live
         ↓
Backend SetLive('camera2')
         ↓
Backend diz ao OBS:
  - Mostra cam_camera2
  - Oculta cam_camera1, cam_camera3
         ↓
OBS muda a cena
         ↓
Backend publica via WebSocket:
  - cameras.updated
  - system.status
         ↓
Frontend atualiza signals
         ↓
Tela redesenha
         ↓
Você vê camera2 em destaque e ao vivo
(tudo em <1 segundo)
```

## Reconexão Automática

**OBS cai:**
- Backend verifica a cada 5s
- Se falhar, espera 1s, depois 2s, 4s, 8s... (exponencial até 30s)
- Quando conectar, volta ao normal

**Frontend perde conexão:**
- Mostra aviso "Reconectando..."
- Tenta reconectar com backoff exponencial
- Quando conecta, carrega estado do backend
- Usuário continua trabalhando

## Câmeras Reais (Moblin)

Instale Moblin no celular:
1. Configure RTMP: `rtmp://seu-ip:1935/camera-celular`
2. Começa transmissão
3. Sistema descobre automaticamente
4. Use no navegador igual às fake cameras

## Troubleshooting Rápido

| Problema | Solução |
|----------|---------|
| Câmeras não aparecem | Verifica se MediaMTX/ffmpeg tá rodando. Clica "Sincronizar" |
| OBS desconectado | Habilita WebSocket em OBS (Ferramentas → WebSocket Server Settings) |
| "Sem conexão backend" | Verifica se terminal 5 (frontend) está rodando |
| Câmera desaparece | Normal! Espera 60s, sistema remove automaticamente |

## Próximas Fases (Roadmap)

- Fase 3: Preview de vídeo (HLS)
- Fase 4: Múltiplas cenas/layouts
- Fase 5: Banco de dados (persistência)
- Fase 6: Deploy (Kubernetes)
- Fase 7: Autenticação de usuários

## Documentação

| Arquivo | Tempo | Descrição |
|---------|-------|-----------|
| QUICK_START.md | 10 min | Setup rápido |
| VISAO_GERAL.md | 5 min | Entendimento geral |
| GUIA_USO.md | 20 min | Como usar |
| GUIA_OBS_MOBLIN.md | 15 min | Câmeras reais |
| DOCUMENTACAO_BACKEND.md | 25 min | Backend Go |
| DOCUMENTACAO_FRONTEND.md | 25 min | Frontend Angular |
| ARQUITETURA_VISUAL.md | 10 min | Diagramas e fluxos |

## Comece Aqui

1. **Super rápido (10 min):** QUICK_START.md
2. **Entender (1h):** VISAO_GERAL.md → GUIA_USO.md → DOCUMENTACAO_BACKEND.md
3. **Com câmeras reais (45 min):** QUICK_START.md → GUIA_OBS_MOBLIN.md

## Resumo

- ✅ Controla múltiplas câmeras no OBS
- ✅ Automático (descobre câmeras)
- ✅ Rápido (<1s para mudar)
- ✅ Resiliente (reconecta sozinho)
- ✅ Fácil (interface web)
- ✅ MVP funcional
- 🚀 Pronto para usar agora
