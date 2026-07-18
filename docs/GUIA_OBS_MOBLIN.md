# Guia: OBS + Moblin — Transmitindo com Câmera do Celular

## O que é Moblin?

**Moblin** é um aplicativo para Android que transforma seu smartphone em uma câmera de transmissão. Ele envia o vídeo da câmera do seu celular para um servidor RTMP (como o MediaMTX).

## O que é RTMP?

**RTMP** (Real Time Messaging Protocol) é um "padrão de comunicação" para enviar vídeo ao vivo. Pense como um "endereço" onde você diz "envie o vídeo para aqui".

Formato: `rtmp://endereço-do-servidor/nome-da-camera`

Exemplo: `rtmp://192.168.1.100:1935/camera-celular`

## Setup: Passo a Passo

### **Passo 1: Preparar a rede**

Você precisa que o celular e o computador estejam na mesma rede (WiFi ou LAN).

1. Anota o **IP do seu computador** na rede:
   - **Windows:** Abre PowerShell e digita:
     ```
     ipconfig
     ```
     Procura por "IPv4 Address" (algo como `192.168.1.50`)
   
   - **Linux/Mac:** Abre terminal e digita:
     ```
     ifconfig
     ```
     Procura por `inet 192.168.x.x`

2. Anota o IP, vai precisar em breve.

### **Passo 2: Subir o MediaMTX (se ainda não estiver)**

```bash
make mediamtx-up
```

MediaMTX fica escutando na **porta 1935** (RTMP).

### **Passo 3: Instalar Moblin no celular**

1. Abra a **Google Play Store** no seu Android
2. Procura por **"Moblin"**
3. Instala o app da Moblin (o "oficial")

### **Passo 4: Configurar Moblin no celular**

1. Abre o **Moblin**
2. Você verá várias opções. Procura por:
   - **Configurações** (ícone de engrenagem)
   - Ou **Settings**

3. Dentro de Configurações, procura por:
   - **RTMP Server** ou **Streaming URL**

4. Digita o endereço RTMP. Exemplo:
   ```
   rtmp://192.168.1.50:1935/camera-celular
   ```
   
   (Substitui `192.168.1.50` pelo IP do seu computador)

5. **Salva** as configurações

6. Volta para a tela principal do Moblin

### **Passo 5: Iniciar transmissão pelo Moblin**

1. No Moblin, procura por:
   - **Começar transmissão** ou **Start**
   - Um grande botão de transmissão (play/record)

2. Clica nele

3. Você verá uma confirmação ou notificação que a transmissão começou

4. **Moblin fica transmitindo o vídeo da câmera do celular** para o endereço que você configurou.

### **Passo 6: Verificar se a câmera foi reconhecida**

1. Volta ao navegador
2. Acessa `http://localhost:4200`
3. Espera alguns segundos
4. Você deveria ver a câmera `camera-celular` com status **"Online"**

**Sucesso!** A câmera do seu celular foi reconhecida pelo sistema!

### **Passo 7: Colocar a câmera do celular no ar**

1. Na tela do navegador, procura pelo card da câmera `camera-celular`
2. Clica no botão **"Colocar no ar"**
3. **No OBS Studio**, você verá a câmera do celular aparecer na cena "Program"
4. Agora você pode transmitir o vídeo do celular para a sua audiência!

## Fluxo Completo: Do Celular ao OBS

```
Celular (Moblin)
      ↓
      ↓ (envia RTMP)
      ↓
MediaMTX (recebe e redistribui)
      ↓
      ↓ (verifica a cada 3s)
      ↓
Backend (descobre a câmera)
      ↓
      ↓ (cria input no OBS)
      ↓
OBS Studio (mostra na cena "Program")
      ↓
      ↓ (você transmite)
      ↓
Sua audiência (Twitch/YouTube/etc)
```

## Usando Múltiplas Câmeras

Você pode ter várias câmeras ao mesmo tempo:

### **Cenário: 1 câmera fake + 1 câmera de celular**

**Terminal 1:**
```bash
make mediamtx-up
```

**Terminal 2:**
```bash
make fake-camera NAME=camera-estudio
```

**Celular:**
- Moblin configurado para `rtmp://IP:1935/camera-celular`

**Navegador:**
- Vê 2 câmeras:
  - `camera-estudio` (da máquina, via ffmpeg)
  - `camera-celular` (do celular, via Moblin)

**Frontend:**
```
Câmeras Online:
┌──────────────────┐  ┌──────────────────┐
│ camera-estudio   │  │ camera-celular   │
│ Online           │  │ Online           │
│ [Colocar no ar]  │  │ [Colocar no ar]  │
└──────────────────┘  └──────────────────┘
```

## Parar a Transmissão do Moblin

1. No Moblin, clica no botão de transmissão novamente (para parar)
2. Ou: Fecha o Moblin
3. A câmera desaparecerá da lista após 60 segundos

## Dicas Importantes

### **Não funciona: "Câmera não aparece"**

**Checklist:**
- [ ] Celular e computador na mesma rede?
- [ ] Moblin configurado com o IP correto?
- [ ] MediaMTX está rodando? (`make mediamtx-up`)
- [ ] Backend está rodando? (`make dev-backend`)
- [ ] Clica em "Sincronizar" no navegador?
- [ ] Moblin está transmitindo? (verifica se aparece algo na tela)

**Dica:** No terminal onde está o MediaMTX, procura por logs de conexão (algo como "camera-celular connected").

### **Câmera aparece, mas vídeo é muito lento**

**Razão:** Problema de rede ou qualidade do Wi-Fi.

**Solução:**
1. Verifica a força do sinal Wi-Fi do celular (procura estar perto do roteador)
2. Verifica a qualidade da conexão no Moblin
3. Reduz a resolução no Moblin (se houver opção)

### **Moblin consome muita bateria**

**Esperado!** Está transmitindo vídeo ao vivo. Use o celular ligado na tomada.

### **Quero mudara câmera que está ao vivo**

1. Clica em outra câmera no navegador
2. A anterior é ocultada
3. A nova é mostrada no OBS
4. Instantâneo!

### **Qual é a qualidade do vídeo?**

Depende de:
- Resolução do Moblin
- Velocidade da internet (upload do celular)
- Velocidade da rede local (Wi-Fi)

Em redes locais de boa qualidade, geralmente é bastante bom (HD ou próximo).

## Integração com OBS Studio

O **OBS Studio** vai ter inputs assim no sistema:

- `cam_camera-estudio` (fake camera)
- `cam_camera-celular` (Moblin)

Você **não precisa tocar** no OBS! O sistema cria tudo automaticamente.

Mas se quiser saber o que está acontecendo:

1. Abre OBS Studio
2. Na cena "Program", procura pelos inputs `cam_*`
3. Cada um é uma câmera que o sistema descobriu
4. O sistema liga/desliga a visibilidade (mostra/esconde) automáticamente

## Cenários de Uso Prático

### **Cenário 1: Transmissão Tradicional (Desktop + Celular)**

```
Câmera 1: Webcam ou câmera USB (fake ou real via câmera física)
Câmera 2: Celular com Moblin (câmera móvel)

Operador no navegador:
- Começa com webcam
- Muda para celular quando precisa de ângulo diferente
- Volta para webcam
```

### **Cenário 2: Multi-ângulos com Múltiplos Celulares**

```
Câmera 1: Celular 1 (Moblin) — ângulo frontal
Câmera 2: Celular 2 (Moblin) — ângulo lateral
Câmera 3: Celular 3 (Moblin) — ângulo aéreo

Operador vai alternando entre elas
Audiência vê uma "transmissão cinematográfica" com múltiplos ângulos
```

### **Cenário 3: Câmeras Fixas + Móvel**

```
Câmera 1: Câmera fixa (fake ou real) — palco
Câmera 2: Celular com Moblin — repórter se movimentando

Operador alterna:
- Palco em geral (câmera 1)
- Close no repórter (câmera 2)
- Volta para palco
```

## Configurações Avançadas de Moblin

Dependendo da versão do Moblin, você pode ter:

- **Resolução:** Qualidade do vídeo (360p, 720p, 1080p)
- **Bitrate:** Velocidade de envio (mais = melhor, mas mais lento)
- **FPS:** Quadros por segundo (60fps = suave, 30fps = normal)
- **Orientação:** Deitado (landscape) ou em pé (portrait)

**Dica:** Começa com configurações padrão. Se o vídeo ficar lento, reduz resolução/bitrate.

## Se Perder a Conexão

Se o celular perder Wi-Fi:

1. Moblin pausa a transmissão
2. A câmera fica "Offline" no navegador
3. Quando o Wi-Fi volta, Moblin reconecta automaticamente
4. A câmera volta para "Online" em segundos

## Diferença: Câmeras Fake vs. Reais

| | Câmera Fake | Câmera Real (Moblin) |
|--|--|--|
| **O que é** | ffmpeg enviando um arquivo MP4 | Vídeo ao vivo do celular |
| **Qualidade** | Sempre a mesma (vídeo gravado) | Depende da rede |
| **Uso** | Testes, demonstração | Transmissões reais |
| **Latência** | Mínima | Um pouco mais (rede) |
| **Bateria** | Não gasta (computador) | Gasta bateria (celular) |
| **Comando** | `make fake-camera NAME=x` | App Moblin no celular |

## Resumo Rápido

1. **Computador:** `make mediamtx-up` + `make dev-backend` + `make dev-frontend`
2. **Celular:** Instala Moblin → configura RTMP → começa transmissão
3. **Navegador:** Vê a câmera appear → clica "Colocar no ar"
4. **OBS:** Vê o vídeo aparecer na cena "Program"
5. **Transmissão:** Já está saindo para sua audiência!

## Suporte/Troubleshooting

Se algo não funciona:

1. Verifica se a rede está ok (celular conectado ao Wi-Fi)
2. Verifica se o IP está correto no Moblin
3. Verifica se MediaMTX está rodando
4. Clica em "Sincronizar" no navegador
5. Tenta reabrir o Moblin
6. Verifica os logs do terminal (procura por erros)

Se continuar com problema, ativa logs de debug:

```bash
# Terminal onde o backend está rodando
# Pressiona Ctrl+C para parar
# Liga com debug:
LOG_LEVEL=debug make dev-backend
```

Você verá mais informações sobre o que está acontecendo.
