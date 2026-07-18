# Guia Passo a Passo — Como Usar a Aplicação

## Pré-requisitos (O que você precisa ter instalado)

Antes de começar, certifique-se de ter:

- **Go 1.22+** (backend)
- **Node.js 18+** (frontend)
- **Docker + Docker Compose** (MediaMTX em container)
- **OBS Studio** (seu programa de transmissão)
- **ffmpeg** (para simular câmeras em desenvolvimento)
- **make** (opcional — se não tiver, pode copiar os comandos)

## Preparação Inicial (Primeira vez)

### Passo 1: Clonar o Repositório

```bash
git clone <url-do-repositorio>
cd PoC-golive
```

### Passo 2: Habilitar WebSocket no OBS

**⚠️ IMPORTANTE: Faça isso ANTES de ligar o backend**

1. Abra o **OBS Studio**
2. Vá em **Ferramentas** → **WebSocket Server Settings**
3. Marque a opção **Enable WebSocket Server**
4. Anote a **porta** (padrão: `4455`) e a **senha** (se houver)
5. Feche a janela

### Passo 3: Criar arquivo de configuração (opcional)

Se o OBS estiver em outra máquina ou porta diferente, crie um arquivo `.env`:

```bash
# Copie o arquivo de exemplo
cp .env.example .env

# Edite o arquivo .env com suas configurações
# Exemplo:
# OBS_ADDR=192.168.1.100:4455
# OBS_PASSWORD=sua-senha-aqui
```

## Passo a Passo: Rodando Tudo

### **Terminal 1: Subir o MediaMTX (servidor de vídeo)**

```bash
make mediamtx-up
```

Você verá um monte de mensagens. Espere até aparecer:
```
mediamtx  | 2024-07-17T10:00:00Z INFO [RTMP listener] listening on :1935
```

Isso significa que o MediaMTX está pronto para receber câmeras.

### **Terminal 2: Simular câmeras fake (opcional, para teste)**

Abra um novo terminal e digite:

```bash
make fake-camera NAME=camera1
```

Você verá ffmpeg começando a enviar o vídeo em loop. Deixe rodando.

Se quiser adicionar outra câmera, abra **mais um terminal**:

```bash
make fake-camera NAME=camera2
```

Agora você tem 2 câmeras transmitindo para o sistema.

### **Terminal 3: Ligar o Backend**

Abra um novo terminal e digite:

```bash
make dev-backend
```

Espere aparecer:
```
2024-07-17T10:00:00Z INFO http server listening addr=:8080
```

Se ver mensagens de "Connecting to OBS..." é normal, quer dizer que está tentando conectar.

### **Terminal 4: Ligar o Frontend**

Abra um novo terminal e digite:

```bash
make dev-frontend
```

Espere aparecer:
```
✓ ready in 1234 ms
```

O navegador pode abrir automaticamente. Se não abrir, acesse:
```
http://localhost:4200
```

## Usando a Aplicação

### **Tela Principal**

Você verá:

1. **Barra no topo** com informações:
   - 🟢 **OBS conectado** (verde = ligado, vermelho = desligado)
   - 🟢 **MediaMTX conectado** (verde = ligado, vermelho = desligado)
   - **Câmera ao vivo:** mostra qual câmera está transmitindo
   - Botão **Sincronizar** para forçar atualização

2. **Grade de câmeras** mostrando cada câmera:
   - Nome da câmera
   - Status (Online/Offline)
   - Botão **"Colocar no ar"**

### **Operação: Transmitindo uma câmera**

1. Espere as câmeras aparecerem como "Online"
2. Clique no botão **"Colocar no ar"** de uma câmera
3. Observe:
   - A câmera fica com destaque azul
   - A barra topo mostra a câmera ao vivo
   - No OBS, o input `cam_<nome>` aparece na cena "Program"

### **Mudando para outra câmera**

1. Clique em **"Colocar no ar"** de outra câmera
2. A anterior desaparece (fica oculta no OBS)
3. A nova aparece no OBS
4. A tela atualiza em tempo real

### **Se uma câmera cair**

1. Você verá um aviso vermelho: "A câmera ao vivo ficou offline"
2. Escolha outra câmera para transmitir
3. O aviso desaparece

### **Botão Sincronizar**

Normalmente o sistema verifica câmeras a cada 3 segundos. Se quiser forçar uma verificação imediata:

1. Clique no botão **"Sincronizar"**
2. A tela atualiza com as câmeras novas/que desapareceram

## Cenários de Uso

### **Cenário 1: Transmissão com 3 câmeras**

```
Terminal 1:  make mediamtx-up
Terminal 2:  make fake-camera NAME=cam-palco
Terminal 3:  make fake-camera NAME=cam-publico
Terminal 4:  make fake-camera NAME=cam-close
Terminal 5:  make dev-backend
Terminal 6:  make dev-frontend
```

Navegador:
1. Vê as 3 câmeras
2. Clica em "cam-palco" → transmite
3. Clica em "cam-publico" → muda para câmera do público
4. Clica em "cam-close" → close

### **Cenário 2: Câmeras reais (Moblin/Smartphone)**

Em vez de `make fake-camera`, você faria:

1. Abrir app **Moblin** no smartphone
2. Configurar para enviar para `rtmp://seu-ip:1935/camera-phone`
3. Aplicação reconhece automaticamente
4. Usa normalmente no navegador

(Ver guia `GUIA_OBS_MOBLIN.md` para detalhes)

## Parando Tudo

### **Para uma câmera fake:**
- No terminal onde está rodando, pressione `Ctrl+C`

### **Para o backend:**
- No terminal, pressione `Ctrl+C`
- Verá: "shutdown complete"

### **Para o frontend:**
- No terminal, pressione `Ctrl+C`

### **Para o MediaMTX:**
```bash
make mediamtx-down
```

## Dicas Úteis

### **Problema: "Sem conexão com o backend"**

**Solução:**
1. Verifica se o backend está rodando (Terminal com `make dev-backend`)
2. Verifica se a URL está correta (deve ser `http://localhost:8080`)
3. Tenta atualizar a página do navegador (F5)

### **Problema: "OBS desconectado" na barra**

**Solução:**
1. OBS está desligado — ligue-o
2. WebSocket não está habilitado — vai em Ferramentas → WebSocket Server Settings
3. Senha incorreta — verifica se a senha no `.env` está certa
4. Espera alguns segundos, backend reconecta automaticamente

### **Problema: "Câmeras offline"**

**Solução:**
1. Verifica se a câmera fake ainda está rodando (`make fake-camera`)
2. Verifica se o MediaMTX está rodando (`make mediamtx-up`)
3. Clica em **Sincronizar** para forçar atualização

### **Problema: Nada aparece no OBS**

**Solução:**
1. Verifica se a cena **"Program"** existe no OBS (backend cria automaticamente)
2. Verifica se o WebSocket está habilitado
3. Verifica os logs do backend procurando por erros

### **Câmera não desaparece depois de desligar**

**Esperado!** O sistema espera 60 segundos antes de remover a câmera do OBS. Isso absorve pequenos "piscos" de conexão.

## Testando via API (avançado)

Se quiser testar a API direto (sem interface web):

```bash
# Verifica se backend está vivo
curl http://localhost:8080/api/v1/health

# Pega lista de câmeras
curl -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/cameras

# Pega status do sistema
curl -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/status

# Coloca camera1 ao vivo
curl -X POST -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/cameras/camera1/live

# Força sincronização
curl -X POST -H "X-Api-Token: dev-token" http://localhost:8080/api/v1/sync
```

## Dados que o Sistema Conhece

Cada câmera que o sistema vê tem essas informações:

```json
{
  "id": "camera1",                              // Identificador único
  "name": "camera1",                            // Nome legível
  "sourceUrl": "rtmp://localhost:1935/camera1", // Endereço do vídeo (base configurável via MEDIA_SOURCE_BASE_URL)
  "status": "online",                           // online ou offline
  "obsSourceCreated": true,                     // Já foi criada no OBS?
  "isLive": true,                               // Está transmitindo agora?
  "lastSeenAt": "2024-07-17T10:00:00Z"         // Quando foi vista
}
```

## Quando Desligar o Computador

Se precisar desligar tudo:

```bash
# Terminal onde está o backend
Ctrl+C

# Terminal onde está o frontend  
Ctrl+C

# Terminal onde está o MediaMTX
Ctrl+C

# Ou pelo terminal
make mediamtx-down
```

Quando for usar novamente, repete o processo.

## Variáveis de Ambiente (Customização Avançada)

Se quiser mudar o comportamento padrão, edite o `.env`:

```bash
# Porta do backend (padrão: 8080)
HTTP_ADDR=:8080

# Endereço do OBS (padrão: localhost:4455)
OBS_ADDR=localhost:4455

# Senha do OBS (padrão: vazio)
OBS_PASSWORD=123456

# URL do MediaMTX (padrão: http://localhost:9997)
MEDIAMTX_API_URL=http://localhost:9997

# Base da URL de vídeo das câmeras (padrão: rtmp://localhost:1935)
MEDIA_SOURCE_BASE_URL=rtmp://localhost:1935

# Token de segurança (padrão: dev-token)
API_TOKEN=dev-token

# Frequência de sincronização (padrão: 3s)
SYNC_INTERVAL=3s

# Nome da cena no OBS (padrão: Program)
PROGRAM_SCENE=Program

# Verbosidade dos logs (padrão: info)
LOG_LEVEL=info  # pode ser: debug, info, warn, error
```

## Próximos Passos

Quando quiser usar com câmeras reais:

1. Leia o arquivo **`GUIA_OBS_MOBLIN.md`** para entender como configurar Moblin
2. Use a mesma interface — funciona igual com câmeras reais!
