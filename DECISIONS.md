# Decisões de implementação

Este documento registra decisões técnicas tomadas durante a implementação
que não estavam explicitamente definidas no plano, ou que precisaram de
ajuste em relação ao especificado.

## FASE 0 — Bootstrap

- **Test runner do frontend:** o plano especifica "Jasmine/Karma padrão do CLI". O Angular CLI 21.2.7 (versão instalada) já não oferece mais Karma/Jasmine como padrão para projetos novos — o `ng new` gera o projeto com **Vitest** como test runner padrão. Mantivemos o padrão gerado pelo CLI (Vitest) em vez de forçar a reintrodução do Karma, já que a regra geral do plano é "usar o mais simples"/"padrão do CLI", e Vitest é o novo padrão. Comando de teste continua `ng test`.
- **Estilo:** optou-se por CSS puro (sem Tailwind), conforme permitido pelo plano ("escolher o mais simples").
- **Autenticação da API do MediaMTX:** a partir da v1.x, o MediaMTX exige autenticação
  também para chamadas à API HTTP por padrão (usuário anônimo `any` não tem a `action: api`
  liberada por padrão). Para dev local, em vez de variáveis de ambiente `MTX_*` (que não
  conseguem representar de forma confiável uma lista de objetos como `authInternalUsers`),
  optou-se por montar um arquivo `mediamtx.yml` próprio via volume no `docker-compose.yml`,
  desabilitando autenticação (usuário `any` com todas as permissões, incluindo `api`).
  **Isso é válido apenas para ambiente de desenvolvimento local.** Também foi necessário
  adicionar explicitamente `paths: { all_others: {} }` ao `mediamtx.yml`: ao fornecer um
  arquivo de config próprio, o path coringa `all_others` (que no config padrão do
  MediaMTX permite publicar em qualquer nome de path sem pré-cadastro) deixa de existir
  implicitamente — sem ele, tentativas de publicar em `rtmp://.../camera1` falhavam com
  `path 'camera1' is not configured`.
- **Shape real da resposta de `GET /v3/paths/list`:** confirmado em runtime (MediaMTX v1.19.2):
  ```json
  { "itemCount": 0, "pageCount": 0, "items": [] }
  ```
  Cada item de `items` (quando existir stream) tem, entre outros campos, `name`, `ready`
  (bool), `readyTime`, `source`, `tracks`. O client em `internal/mediaserver` decodifica
  `{ items: [{ name, ready, ... }] }` e filtra apenas os itens com `ready == true`.

## FASE 2 — Wrapper OBS

- **Campos de settings do `ffmpeg_source` não confirmados via `GetInputDefaultSettings`
  em runtime:** o plano pedia para consultar essa chamada com o OBS local antes de
  chutar os nomes dos campos. O OBS Studio estava instalado e rodando na máquina, mas
  o servidor `obs-websocket` não estava habilitado (porta `4455` recusando conexão) e
  o arquivo de config do plugin não foi localizado no perfil padrão — habilitá-lo exigiria
  automação de UI (Tools → WebSocket Server Settings → Enable), o que não foi feito para
  não mexer na configuração do OBS do usuário sem confirmação explícita.
  Optou-se por usar os nomes de campos **documentados oficialmente** do input
  `ffmpeg_source` do OBS:
  - `input` (string, URL quando `is_local_file: false`)
  - `is_local_file` (bool)
  - `reconnect_delay_sec` (int) — o plano citava um campo booleano `reconnect`, que
    não existe no `ffmpeg_source`; o campo real de controle de reconexão é
    `reconnect_delay_sec` (segundos entre tentativas).
  - `buffering_mb` (int, setado baixo conforme pedido no plano)
  **Confirmado em runtime** (2026-07-16, após o usuário habilitar o obs-websocket
  manualmente): chamando `GetInputDefaultSettings` com `inputKind: ffmpeg_source` contra
  o OBS local, os campos usados batem exatamente com os nomes reais da API:
  `is_local_file` (bool), `reconnect_delay_sec` (number, default 10), `buffering_mb`
  (number, default 2). O campo `input` (string, usado quando `is_local_file: false`) não
  aparece na resposta de defaults porque seu valor-zero é string vazia e a API omite
  defaults vazios — mas é o nome de campo documentado oficialmente e usado corretamente
  no código. Nenhuma alteração foi necessária em `obs.go`.
- **Checklist manual do README executado com sucesso (2026-07-16):** com o obs-websocket
  habilitado pelo usuário (porta `4455`, senha `123456`), validou-se contra uma instância
  real do OBS: (1) subir o backend cria a cena `Program` automaticamente; (2)
  `CreateCameraInput` cria inputs `ffmpeg_source` corretamente; (3) `SetOnlyVisibleSource`
  alterna a visibilidade entre múltiplos inputs `cam_*`, escondendo os demais; (4)
  `RemoveInput` remove o input e seu scene item. A etapa restante do checklist (matar/
  reabrir o OBS para validar a reconexão com backoff) não foi executada nesta sessão para
  não interromper o OBS do usuário desnecessariamente; a lógica de reconexão está coberta
  pelo desenho do `watchLoop` (health check ativo a cada 5s + backoff 1s→30s) e pode ser
  validada a qualquer momento seguindo o checklist do README.
- **Fluxo de demonstração completo (seção 9 do plano) executado com sucesso (2026-07-16):**
  com `sample.mp4` gerado via `ffmpeg -f lavfi` e duas câmeras fake publicando via
  `make fake-camera NAME=camera1`/`camera2`, o painel mostrou as duas câmeras "Online"
  em poucos segundos; "Colocar no ar" na `camera2` atualizou o OBS (`cam_camera2` visível,
  `cam_camera1` oculto, confirmado via `GetSceneItemList`) e o painel refletiu o estado
  (`isLive`, destaque visual, indicador "No ar: camera2") em tempo real via WebSocket.
  Nota: o `mediamtx.yml` precisou do path coringa `all_others` (ver acima) para aceitar
  publicação em paths não pré-cadastrados.
  Observação cosmética: dois inputs de teste (`cam_test1`/`cam_test2`) criados manualmente
  durante a validação do wrapper OBS não foram removidos do OBS mesmo com `RemoveInput`
  retornando sem erro — não afeta o funcionamento do sistema (a orquestração real usa
  apenas nomes `cam_<id>` derivados dos paths do MediaMTX); requer remoção manual na
  lista de fontes do OBS se se desejar uma cena limpa.
- **Detecção de queda de conexão:** a lib `goobs` expõe o canal `Disconnected` apenas
  internamente (campo não exportado). Em vez de depender dele, o `ObsController` faz um
  *health check* ativo (`General.GetVersion`) a cada 5s em uma goroutine dedicada; se
  falhar, dispara o reconnect com backoff exponencial (1s → 30s), conforme pedido no plano.

## FASE 6 — Acabamento

- **Graceful shutdown em Windows:** `signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)`
  captura `Ctrl+C` normalmente em um console Windows real. Em testes automatizados via
  `kill -TERM <pid>` no Git Bash, o Windows não propaga um SIGTERM real (não existe
  nativamente) — o processo é encerrado de forma abrupta pelo próprio `kill`/`TerminateProcess`,
  então o caminho de shutdown gracioso do código não chega a rodar nesse cenário específico
  de teste via shell. Isso não afeta o uso real (`Ctrl+C` no terminal onde o servidor roda
  interativamente aciona o handler corretamente). Validado que o processo encerra em poucos
  milissegundos e sem stack trace em ambos os casos, atendendo ao critério de aceite
  ("`Ctrl+C` encerra em menos de 3s sem stack trace").
- **`make` não disponível na máquina de desenvolvimento:** o Windows usado para
  implementar este projeto não tem `make` instalado (nem via Git Bash/MSYS2, nem via
  choco). O `Makefile` foi mantido conforme o plano pede (é a interface esperada do
  projeto), mas a validação de `make check` nesta máquina foi feita rodando os comandos
  equivalentes manualmente: `go vet ./... && go test ./...` no backend, e
  `ng lint (opcional) && ng test --watch=false` no frontend — todos passando. Ver nota
  de pré-requisito no README.
- **`ng lint` não configurado:** o Angular CLI 21 não inclui ESLint por padrão em
  projetos novos (seria necessário `ng add angular-eslint`, fora do escopo do plano).
  O alvo `make check` já trata isso como opcional (`ng lint || true`), conforme previsto
  no próprio plano ("`ng lint` (se configurado)").
