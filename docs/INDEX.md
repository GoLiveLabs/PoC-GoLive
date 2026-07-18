# Documentação Completa — Índice

Bem-vindo! Esta é a documentação completa da aplicação **Live Orchestrator**. Escolha seu caminho abaixo.

---

## 🎯 Começa Aqui

### **1. Quero entender rapidinho o que é isto**
→ Leia **[VISAO_GERAL.md](VISAO_GERAL.md)** (5 min)

Aqui você aprende:
- O que a aplicação faz
- Como funciona em alto nível
- As peças principais
- Por que foi criada

---

## 🚀 Quero Usar Agora

### **2. Quero colocar tudo rodando (setup completo)**
→ Siga **[GUIA_USO.md](GUIA_USO.md)** (15-20 min)

Passo a passo:
- Como instalar pré-requisitos
- Como ligar tudo (MediaMTX, Backend, Frontend)
- Como usar a interface
- Troubleshooting comum

**Resultado:** Você tem 2-3 câmeras fake transmitindo e está controlando qual vai ao ar.

---

### **3. Quero usar câmeras reais do meu celular (Moblin)**
→ Leia **[GUIA_OBS_MOBLIN.md](GUIA_OBS_MOBLIN.md)** (10-15 min)

Saiba como:
- Instalar e configurar Moblin
- Conectar celular ao computador
- Transmitir câmera do celular
- Integrar com o OBS

**Resultado:** Seu smartphone vira uma câmera profissional.

---

## 📚 Quero Entender o Código

### **4. Quero saber como o Backend funciona**
→ Leia **[DOCUMENTACAO_BACKEND.md](DOCUMENTACAO_BACKEND.md)** (20-30 min)

Você aprenderá:
- Arquitetura do backend (Go)
- Cada módulo e sua responsabilidade
- Como se conecta ao OBS
- Como se conecta ao MediaMTX
- O fluxo de sincronização
- Dados que trafegam
- Variáveis de ambiente

**Para quem:** Desenvolvedores, pessoas curiosas sobre a lógica.

### **5. Quero saber como o Frontend funciona**
→ Leia **[DOCUMENTACAO_FRONTEND.md](DOCUMENTACAO_FRONTEND.md)** (20-30 min)

Você aprenderá:
- Arquitetura do frontend (Angular)
- Cada serviço e componente
- Como se conecta ao backend (WebSocket)
- Fluxo de dados
- Signals (reatividade)
- Layout e componentes
- Tratamento de erros

**Para quem:** Desenvolvedores web, pessoas cursosas sobre UI.

---

## 📋 Quick Reference

### **Perguntas Rápidas**

| Pergunta | Resposta Rápida | Arquivo |
|----------|---|---|
| **O que é isto?** | Controla múltiplas câmeras no OBS | VISAO_GERAL.md |
| **Como instalo?** | `make mediamtx-up`, depois backends/frontend | GUIA_USO.md |
| **Como usar?** | Clica no botão "Colocar no ar" na câmera | GUIA_USO.md |
| **Tenho problemas** | Vê a seção "Dicas Úteis" em GUIA_USO.md | GUIA_USO.md |
| **Como usa Moblin?** | Lê GUIA_OBS_MOBLIN.md | GUIA_OBS_MOBLIN.md |
| **Como funciona o backend?** | Lê DOCUMENTACAO_BACKEND.md | DOCUMENTACAO_BACKEND.md |
| **Como funciona o frontend?** | Lê DOCUMENTACAO_FRONTEND.md | DOCUMENTACAO_FRONTEND.md |
| **Tenho dúvidas técnicas** | Vê DECISIONS.md (na raiz) | ../DECISIONS.md |

---

## 🗺️ Mapa Mental da Aplicação

```
┌─────────────────────────────────────────────────────────┐
│                   VOCÊ (Navegador)                      │
│          Abre http://localhost:4200                    │
│          Vê câmeras e clica para mudar                 │
└─────────────────────────────────────────────────────────┘
                           ↑ ↓
                    (WebSocket em tempo real)
                           ↑ ↓
┌─────────────────────────────────────────────────────────┐
│                 BACKEND (Go) :8080                      │
│  • Descobre câmeras no MediaMTX                        │
│  • Cria inputs no OBS                                  │
│  • Envia atualizações ao navegador                     │
└─────────────────────────────────────────────────────────┘
       ↑          ↑                  ↑
       │          │                  │
   CÂMERAS    MEDIAMTX         OBS STUDIO
   (RTMP)     :1935             :4455
              :9997             (controle)
```

---

## 📂 Estrutura de Arquivos

```
PoC-golive/
├── docs/                          ← VOCÊ ESTÁ AQUI
│   ├── INDEX.md                   ← Você está lendo isto
│   ├── VISAO_GERAL.md             ← Entender o quê
│   ├── GUIA_USO.md                ← Como usar
│   ├── GUIA_OBS_MOBLIN.md         ← Câmeras reais
│   ├── DOCUMENTACAO_BACKEND.md    ← Como funciona Go
│   └── DOCUMENTACAO_FRONTEND.md   ← Como funciona Angular
│
├── backend/                       ← Código Go
│   ├── cmd/server/main.go         ← Entrada principal
│   └── internal/
│       ├── orchestrator/          ← Lógica principal
│       ├── mediaserver/           ← Conexão com MediaMTX
│       ├── obs/                   ← Conexão com OBS
│       ├── httpapi/               ← API REST + WebSocket
│       └── config/                ← Configuração
│
├── frontend/                      ← Código Angular
│   ├── src/
│   │   ├── app/
│   │   │   ├── app.ts             ← Componente principal
│   │   │   ├── core/              ← Serviços
│   │   │   │   ├── api.service.ts
│   │   │   │   ├── websocket.service.ts
│   │   │   │   └── models.ts
│   │   │   └── features/          ← Componentes
│   │   │       ├── camera-grid/
│   │   │       └── control-bar/
│   │   └── environments/          ← Configuração
│   └── package.json
│
├── Makefile                       ← Comandos rápidos
├── docker-compose.yml             ← MediaMTX
├── mediamtx.yml                   ← Configuração MediaMTX
└── README.md                      ← Readme original

```

---

## 🎓 Caminhos de Aprendizado

### **Caminho 1: "Quero só usar"** (⏱️ 20 min)
1. VISAO_GERAL.md (5 min)
2. GUIA_USO.md (15 min)
3. Pronto! Começa a usar.

### **Caminho 2: "Quero entender"** (⏱️ 1h)
1. VISAO_GERAL.md (5 min)
2. DOCUMENTACAO_BACKEND.md (25 min)
3. DOCUMENTACAO_FRONTEND.md (25 min)
4. GUIA_USO.md (5 min)
5. Pronto! Entende a lógica.

### **Caminho 3: "Quero desenvolver/modificar"** (⏱️ 1,5h)
1. Caminho 2 completo
2. GUIA_OBS_MOBLIN.md (15 min) — entender integração
3. DECISIONS.md (../DECISIONS.md) (15 min) — decisões técnicas
4. Explorar o código (`backend/` e `frontend/`)
5. Pronto! Pode modificar.

---

## 🆘 Troubleshooting Rápido

### **"Instalei tudo, mas não funciona"**

Checklist:

- [ ] MediaMTX está rodando? (`docker ps` deve mostrar `mediamtx`)
- [ ] Backend está rodando? (deve ver `listening on :8080`)
- [ ] Frontend está rodando? (deve ver `ready in Xms`)
- [ ] OBS tem WebSocket habilitado? (Ferramentas → WebSocket Server Settings)
- [ ] Navegador consegue acessar `http://localhost:4200`?

Se passou em todos, vê **GUIA_USO.md → Seção "Dicas Úteis"**.

### **"Câmera não aparece"**

- Vê se está com MediaMTX rodando
- Clica em "Sincronizar" no navegador
- Vê GUIA_USO.md → "Problema: Câmeras offline"

### **"OBS desconectado"**

- OBS ligado?
- WebSocket habilitado?
- Porta correta? (padrão 4455)
- Espera alguns segundos, backend reconecta automaticamente

---

## 📞 Informações de Contato

Para questões técnicas específicas, abra uma issue no repositório GitHub.

---

## 📄 Documentos Relacionados

Na raiz do repositório:

- **README.md** — Arquivo original (mais técnico)
- **DECISIONS.md** — Decisões técnicas tomadas
- **Makefile** — Comandos disponíveis
- **.env.example** — Template de variáveis

---

## ✅ Checklist: Você está pronto quando...

- [ ] Entende o que a aplicação faz
- [ ] Consegue ligar tudo (MediaMTX, Backend, Frontend)
- [ ] Consegue ver câmeras no navegador
- [ ] Consegue mudar qual câmera está ao vivo
- [ ] OBS mostra a câmera na cena "Program"
- [ ] Consegue usar com Moblin (opcional)
- [ ] Entende como o Backend funciona
- [ ] Entende como o Frontend funciona

Se passou em tudo: **Parabéns! Você domina a aplicação! 🎉**

---

## 🚀 Próximos Passos

Após dominar:

1. **Quer modificar?** Explore o código nos diretórios `backend/` e `frontend/`
2. **Quer customizar?** Edita `.env` para variáveis, `mediamtx.yml` para configuração
3. **Quer fazer deploy?** Vê como containerizar (futuro)
4. **Quer contribuir?** Faz um fork, cria uma branch, envia um PR

---

**Última atualização:** Julho 2024  
**Versão:** 1.0 PoC  
**Status:** MVP Funcional ✅
