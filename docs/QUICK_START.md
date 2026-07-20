# Quick Start — 10 Minutos para Começar

**Não tem tempo? Segue isto e em 10 minutos você tá testando.**

## O que é?

Uma ferramenta pra trocar qual câmera está sendo transmitida no OBS (seu programa de streaming) usando um painel web simples.

## Antes de Começar

Tem instalado?
- [ ] Docker
- [ ] Go
- [ ] Node.js
- [ ] OBS Studio
- [ ] ffmpeg

Se não, volta em `docs/GUIA_USO.md` para instruções de instalação.

---

## Começar (5 terminais abertos em paralelo)

### Terminal 1: MediaMTX (servidor de vídeo) + Postgres
```bash
make mediamtx-up
```
Espera por: `listening on :1935` (sobe também o Postgres usado pelos
cadastros de clients/ingests/streaming-platforms/live-ids — o backend não
inicia sem ele)

### Terminal 2: Câmera Fake 1
```bash
make fake-camera NAME=camera1
```
Deixa rodando

### Terminal 3: Câmera Fake 2
```bash
make fake-camera NAME=camera2
```
Deixa rodando

### Terminal 4: Backend (maestro)
```bash
make dev-backend
```
Espera por: `listening addr=:8080`

### Terminal 5: Frontend (tela)
```bash
make dev-frontend
```
Espera por: `ready in`

---

## Preparar OBS (1 minuto)

1. Abre **OBS Studio**
2. Vai em **Ferramentas** → **WebSocket Server Settings**
3. Marca **Enable WebSocket Server** ✓
4. Clica OK

---

## Usar (1 minuto)

1. Abre navegador: `http://localhost:4200`
2. Espera aparecer 2 câmeras como "Online"
3. Clica em **"Colocar no ar"** em qualquer câmera
4. No OBS, vê a câmera aparecer automaticamente na cena "Program"
5. Clica em outra câmera no navegador
6. OBS muda automaticamente

**Pronto! 🎉**

---

## Próximas Coisas Para Explorar

- Adiciona mais câmeras (repete Terminal 2/3)
- Desliga uma câmera (Ctrl+C no terminal dela)
- Desliga o OBS e vê backend reconectar
- Lê `docs/GUIA_OBS_MOBLIN.md` para câmeras reais
- Lê `docs/DOCUMENTACAO_BACKEND.md` se quer entender o código

---

## Parar Tudo

Em cada terminal:
```bash
Ctrl+C
```

Para o MediaMTX:
```bash
make mediamtx-down
```

---

## Problemas?

| Problema | Solução |
|----------|---------|
| "Sem conexão com backend" | Verifica se terminal 5 (frontend) tá rodando |
| "Câmeras não aparecem" | Clica "Sincronizar" no navegador. Se continuar, verifica se ffmpeg/MediaMTX tá rodando |
| "OBS desconectado" | Habilita WebSocket em Tools → WebSocket Server Settings |
| "Porta já em uso" | Outra aplicação tá usando a porta. Muda no `.env` |

Mais ajuda em `docs/GUIA_USO.md`.

---

## Próximo Passo

Quer usar câmeras reais? Lê `docs/GUIA_OBS_MOBLIN.md` em 5 minutos.

Quer entender como funciona? Lê `docs/VISAO_GERAL.md` em 5 minutos.

---

**That's it! Enjoy! 🚀**
