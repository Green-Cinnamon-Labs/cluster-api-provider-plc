# 04 — Reconciliacao

Esse doc descreve como o reconciler DEVE funcionar. A implementacao ainda nao existe (issue #38) — o controller hoje e um stub vazio. Isso aqui e o mapa.

## O que e reconciliacao no K8s

O Kubernetes funciona com um loop declarativo:

1. Voce declara o **estado desejado** (`.spec`)
2. O controller observa o **estado real** (`.status`)
3. Se tem diferenca, o controller age pra fechar o gap
4. Repete

No nosso caso, o "estado desejado" e a politica de controle — parametros que os controladores devem ter. O "estado real" e o que a planta reporta via gRPC. O operator NAO cria nem remove controladores. Eles ja existem na planta. O operator so ajusta parametros.

## Fluxo do reconcile

```
Reconcile(PLCMachine) chamado
  |
  +- 1. Conecta na planta (spec.plantAddress) via gRPC
  |     -> se falha: phase = Pending, condition = Degraded, requeue
  |
  +- 2. GetPlantStatus() — le o estado atual da planta
  |     -> preenche .status.plantTime, .status.derivNorm, etc.
  |
  +- 3. ListControllers() — descobre quais controllers existem na planta
  |
  +- 4. Diff de parametros: spec.controllers vs parametros reais
  |     +- Controller no spec E na planta, mas params diferentes?
  |     |    -> UpdateController() com os params do spec
  |     +- Controller no spec mas NAO na planta?
  |     |    -> Warning no log (controller esperado nao existe)
  |     +- Controller na planta mas NAO no spec?
  |          -> Nao faz nada (o operator nao gerencia esse controller)
  |
  +- 5. Diff de disturbances: spec.disturbances vs planta real
  |     +- IDV no spec mas inativo? -> SetDisturbance(active=true)
  |     +- IDV ativo mas nao no spec? -> SetDisturbance(active=false)
  |
  +- 6. Atualiza .status com o estado pos-reconciliacao
  |     +- phase = Running (se tudo ok) / Degraded (se alarmes) / Shutdown (se ISD)
  |     +- controllers[] com medicoes atuais e erros
  |     +- activeDisturbances[]
  |     +- alarms[]
  |     +- lastReconcileTime = now()
  |
  +- 7. Requeue apos metricsIntervalMs
```

## Deteccao de problemas

A beleza do modelo K8s e que a deteccao de falha e automatica:

| Situacao                        | O que o operator ve                    | O que faz                              |
|---------------------------------|----------------------------------------|----------------------------------------|
| Planta normal                   | phase=Running, sem alarmes             | Nada. Requeue no intervalo             |
| Alarme ativo                    | alarms[] nao vazio                     | phase=Degraded, condition              |
| ISD (shutdown)                  | isdActive=true, derivNorm=0            | phase=Shutdown                         |
| Conexao gRPC falhou             | erro no dial                           | phase=Pending, retry                   |
| Parametro divergiu              | diff detecta divergencia               | UpdateController com params do spec    |
| Controller esperado nao existe  | spec pede ID que a planta nao tem      | Warning no log, condition = Degraded   |

## Sobre o requeue

O controller-runtime tem dois modos de requeue:

- **`ctrl.Result{RequeueAfter: 1s}`** — volta daqui a 1 segundo
- **`ctrl.Result{Requeue: true}`** — volta imediatamente (com backoff)

Pro nosso caso, o padrao e `RequeueAfter: spec.metricsIntervalMs` (default 1s). Isso vira efetivamente um polling loop. No futuro, podemos usar `StreamMetrics` pra ser event-driven.

## Idempotencia

Toda acao do reconciler deve ser idempotente. Se voce chamar `UpdateController` duas vezes com os mesmos parametros, a segunda deve ser um no-op. O server gRPC da planta ja trata isso.

## O que NAO e responsabilidade do operator

- **Decisao de controle**: o operator nao calcula saidas PID. Quem faz isso e a planta (o `ControllerBank` no Rust). O operator so ajusta parametros (ganhos, setpoints) dos controllers que ja existem.
- **Criar/destruir controllers**: os controllers sao criados no codigo Rust. O operator assume que eles ja existem. Se um ID do spec nao bater com nenhum controller da planta, e um warning — nao tenta criar.
- **Criar/destruir a planta**: a planta e um Pod separado. O operator assume que ela ja esta rodando no `plantAddress`.
- **Storage de dados historicos**: o operator lida com o agora. Historico e job de Prometheus/Grafana.
