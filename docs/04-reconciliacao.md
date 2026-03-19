# 04 — Reconciliacao

Esse doc descreve como o reconciler DEVE funcionar. A implementacao ainda nao existe (issue #38) — o controller hoje e um stub vazio. Isso aqui e o mapa.

## O que e reconciliacao no K8s

O Kubernetes funciona com um loop declarativo:

1. Voce declara o **estado desejado** (`.spec`)
2. O controller observa o **estado real** (`.status`)
3. Se tem diferenca, o controller age pra fechar o gap
4. Repete

No nosso caso, o "estado desejado" sao os controladores e disturbios que voce quer na planta, e o "estado real" sao as metricas e controladores que a planta reporta via gRPC.

## Fluxo do reconcile

```
Reconcile(PLCMachine) chamado
  │
  ├─ 1. Conecta na planta (spec.plantAddress) via gRPC
  │     → se falha: phase = Pending, condition = Degraded, requeue
  │
  ├─ 2. GetPlantStatus() — le o estado atual da planta
  │     → preenche .status.plantTime, .status.derivNorm, etc.
  │
  ├─ 3. Diff de controllers: spec.controllers vs planta real
  │     ├─ Controller no spec mas nao na planta? → AddController()
  │     ├─ Controller na planta mas nao no spec? → RemoveController()
  │     └─ Controller existe mas params diferentes? → UpdateController()
  │
  ├─ 4. Diff de disturbances: spec.disturbances vs planta real
  │     ├─ IDV no spec mas inativo na planta? → SetDisturbance(active=true)
  │     └─ IDV ativo na planta mas nao no spec? → SetDisturbance(active=false)
  │
  ├─ 5. Atualiza .status com o estado pos-reconciliacao
  │     ├─ phase = Running (se tudo ok) / Degraded (se alarmes) / Shutdown (se ISD)
  │     ├─ controllers[] com medicoes atuais e erros
  │     ├─ activeDisturbances[]
  │     ├─ alarms[]
  │     └─ lastReconcileTime = now()
  │
  └─ 6. Requeue apos metricsIntervalMs
```

## Deteccao de problemas

A beleza do modelo K8s e que a deteccao de falha e automatica:

| Situacao                        | O que o operator ve                    | O que faz                     |
|---------------------------------|----------------------------------------|-------------------------------|
| Planta normal                   | phase=Running, sem alarmes             | Nada. Requeue no intervalo    |
| Alarme ativo                    | alarms[] nao vazio                     | phase=Degraded, condition     |
| ISD (shutdown)                  | isdActive=true, derivNorm=0            | phase=Shutdown                |
| Conexao gRPC falhou             | erro no dial                           | phase=Pending, retry          |
| Controller sumiu da planta      | diff detecta ausencia                  | Re-adiciona via AddController |
| Parametro mudou no spec         | diff detecta divergencia               | UpdateController              |

## Sobre o requeue

O controller-runtime tem dois modos de requeue:

- **`ctrl.Result{RequeueAfter: 1s}`** — volta daqui a 1 segundo
- **`ctrl.Result{Requeue: true}`** — volta imediatamente (com backoff)

Pro nosso caso, o padrao e `RequeueAfter: spec.metricsIntervalMs` (default 1s). Isso vira efetivamente um polling loop. No futuro, podemos usar `StreamMetrics` pra ser event-driven.

## Idempotencia

Toda acao do reconciler deve ser idempotente. Se voce chamar `AddController` duas vezes pro mesmo ID, a segunda deve ser um no-op (ou um update). O server gRPC da planta ja trata isso — se o controller ja existe, retorna sucesso com mensagem.

## O que NAO e responsabilidade do operator

- **Decisao de controle**: o operator nao calcula saidas PID. Quem faz isso e a planta (o `ControllerBank` no Rust). O operator so configura QUAIS controllers existem e com QUAIS parametros.
- **Criar/destruir a planta**: a planta e um Pod separado. O operator assume que ela ja esta rodando no `plantAddress`.
- **Storage de dados historicos**: o operator lida com o agora. Historico e job de Prometheus/Grafana.
