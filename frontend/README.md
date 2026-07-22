# golive-frontend

Frontend Angular do Live Orchestrator — painel de controle web para o operador
gerenciar câmeras e transmissões ao vivo. Comunica-se com o backend
(`ghcr.io/golivelabs/golive-backend`) via HTTP REST e WebSocket para consumir
dados de câmeras e estado do sistema em tempo real.

> Este repositório é **independente**. Ele contém apenas o código e o histórico
> do frontend. Para executar a stack completa (backend + frontend + MediaMTX +
> OBS), consulte o repositório de infraestrutura
> [`GoLiveLabs/PoC-GoLive`](https://github.com/GoLiveLabs/PoC-GoLive) e use
> o `docker-compose.yml` publicado lá como matriz de compatibilidade entre as
> imagens `ghcr.io/golivelabs/golive-backend` e `ghcr.io/golivelabs/golive-frontend`.
> Não há nenhum subdiretório `frontend/` aqui, e não é necessário clonar nenhum
> outro repositório para construir, testar ou executar apenas este painel.

## Pré-requisitos

- Node.js 22+
- npm 11+ (`packageManager: npm@11.11.0`)
- Docker (opcional, apenas para construir/rodar a imagem publicada)
- Um backend acessível (para conexão com o orquestrador em tempo de execução;
  não é necessário para build/teste locais)

## Instalação

A partir da raiz deste repositório:

```bash
npm ci
```

## Servidor de desenvolvimento

```bash
npm start
# ou: npx ng serve
```

O servidor ficará disponível em `http://localhost:4200/` e recarrega
automaticamente ao modificar arquivos fonte. Para apontar o painel para um
backend diferente do padrão, ajuste a URL base em `src/core/api.service.ts`
(ou a variável de entretenimento esperada pelo serviço de API).

## Testes

```bash
npx ng test --watch=false
```

A suíte de testes é a mesma que rodava dentro do monorepo original e deve passar
de forma idêntica neste repositório isolado. O runner de testes é o
`@angular/build:unit-test` (configurado em `angular.json`, architect `test`).

## Build

```bash
npm run build
```

Os artefatos de produção são gerados em `dist/`.

## Build e publicação

A imagem Docker é publicada automaticamente pelo workflow
[`.github/workflows/release.yml`](.github/workflows/release.yml) quando uma
tag `vMAJOR.MINOR.PATCH` é empurrada para este repositório. O campo `version`
de `package.json` precisa casar com a tag empurrada. Para construir a imagem
localmente (sem depender do pipeline):

```bash
docker build -t ghcr.io/golivelabs/golive-frontend:dev .
```

A imagem produzida expõe a porta `4200` e serve a aplicação com
`npm start -- --host 0.0.0.0 --port 4200 --poll 1000`.

### Tags e releases

- As releases seguem [SemVer](https://semver.org/) a partir de `v0.1.0`.
- O pipeline de release é acionado apenas por tags que casam com
  `^v[0-9]+\.[0-9]+\.[0-9]+$`. Tags malformadas (ex.: `v0.1`) não disparam
  build/publicação.
- O workflow de CI
  [`.github/workflows/ci.yml`](.github/workflows/ci.yml) roda `ng test
  --watch=false` em pushes e PRs contra `main`, sem build/publicação.

## Documentação interna

- [`docs/ARQUITETURA.md`](docs/ARQUITETURA.md) — princípios de design, camadas e estrutura de diretórios
- [`docs/COMUNICACAO.md`](docs/COMUNICACAO.md) — fluxos de comunicação com o backend (REST/WebSocket)
- [`docs/SERVICOS.md`](docs/SERVICOS.md) — descrição por serviço/componente com interfaces públicas

## Estrutura

```
src/                  código-fonte da aplicação Angular
  app/                componentes, serviços e modelos
docs/                 documentação do frontend
.github/workflows/    ci.yml e release.yml
angular.json          config do Angular CLI (build/serve/test)
Dockerfile            build da imagem (node:22-alpine)
package.json          dependências e scripts (version reflete a release atual)
```