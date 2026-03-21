# 04 — Reconciliacao

Esse doc descreve como o reconciler DEVE funcionar. A implementacao ainda nao existe (issue #38) — o controller hoje e um stub vazio. Isso aqui e o mapa.

## O que esse operator e

NAO e um sincronizador de configuracao. E um **controlador supervisorio**. A planta vive sozinha, tem controladores PID rodando, sofre disturbios aleatorios. O operator:

1. **Observa** — le XMEAS da planta via gRPC
2. **Avalia** — compara com faixas aceitaveis e detecta tendencias
3. **Decide** — se precisa intervir, qual regra dispara
4. **Age** — executa a acao via gRPC (ou nao faz nada)
5. **Registra** — grava o estado no .status como memoria pro proximo ciclo

## Fluxo do reconcile

```
Reconcile(PLCMachine) chamado
  |
  +- 1. Fetch PLCMachine CR do cluster
  |
  +- 2. Dial gRPC -> spec.plantAddress
  |     -> se falha: phase = Pending, requeue com backoff
  |
  +- 3. GetPlantStatus() — le XMEAS da planta
  |     -> recebe: xmeas[], plantTime, isdActive, alarms
  |
  +- 4. Checar ISD (parada de emergencia)
  |     -> se isdActive: phase = Shutdown, condition = Degraded, parar
  |
  +- 5. Pra cada OperatingRange no spec:
  |     +- Ler xmeas[range.xmeasIndex]
  |     +- Comparar com .status.variables (valor anterior)
  |     +- Calcular trend: Rising, Falling, ou Stable
  |     +- Checar se ta dentro de [min, max]
  |     +- Gravar em VariableStatus
  |
  +- 6. Avaliar responseRules:
  |     +- Pra cada regra:
  |     |    +- A variavel referenciada (watchRef) saiu da faixa?
  |     |    +- A condition bate? (above_max / below_min)
  |     |    +- Se sim: executar UpdateController(controllerID, parameter, value)
  |     |    +- Gravar em lastAction
  |     +- Se nenhuma regra disparou: nada a fazer
  |
  +- 7. Determinar phase:
  |     +- Todas inRange + trends Stable -> Stable
  |     +- Todas inRange + alguma trend Rising/Falling -> Transient
  |     +- Alguma fora da faixa -> Alarm
  |     +- isdActive -> Shutdown
  |
  +- 8. Gravar .status (memoria)
  |     +- variables[] com valores, trends, inRange
  |     +- phase
  |     +- lastAction (se houve)
  |     +- lastReconcileTime = now()
  |
  +- 9. Requeue adaptativo:
        +- Phase Stable -> RequeueAfter(baseMs)
        +- Phase Transient -> RequeueAfter(transientMs)
        +- Phase Alarm -> RequeueAfter(transientMs)
        +- Phase Shutdown -> nao requeue (nada a fazer)
```

## Deteccao de tendencia

O operator compara o valor atual com o anterior (gravado no .status):

```
delta = atual - anterior
threshold = 0.5% do range (max - min)

se |delta| < threshold -> Stable
se delta > 0           -> Rising
se delta < 0           -> Falling
```

O threshold e relativo a faixa — uma variacao de 0.1 pode ser irrelevante pra pressao (range de 200) mas significativa pra nivel (range de 20).

## Intervalo adaptativo

O operator nao faz polling fixo. Se detecta transitorio:

| Situacao                       | Intervalo             |
|--------------------------------|-----------------------|
| Tudo estavel, tudo em faixa    | baseMs (default 2s)   |
| Transitorio detectado          | transientMs (200ms)   |
| Variavel fora da faixa (Alarm) | transientMs (200ms)   |
| ISD (Shutdown)                 | Para de monitorar     |

## Idempotencia

Se o operator disparar a mesma regra duas vezes seguidas (ex: pressao continua acima do max), a segunda chamada a UpdateController com o mesmo valor e um no-op. O gRPC server da planta trata isso.

## O que NAO e responsabilidade do operator

- **Calculo PID**: quem calcula saida dos controladores e a planta (ControllerBank no Rust). O operator so ajusta PARAMETROS (ganho, setpoint).
- **Criar/destruir controllers**: existem na planta, o operator so ajusta parametros.
- **Controlar disturbios**: disturbios sao aleatorios. O operator reage aos efeitos, nao causa.
- **Criar/destruir a planta**: a planta e um Pod separado. O operator assume que ela ja ta rodando.
- **Historico**: o operator lida com o agora e o "um passo atras" (pro trend). Historico longo e job de Prometheus/Grafana.
- **Sincronizar config**: o operator nao empurra parametros do spec pra planta. Ele le o estado, avalia, e decide. A decisao pode ser "nao fazer nada".
