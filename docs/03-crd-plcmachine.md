# 03 — CRD PLCMachine

A PLCMachine e o unico recurso customizado desse operator. Ela representa um controlador supervisorio conectado a uma planta TEP via gRPC.

## A ideia

A planta vive sozinha. Tem controladores PID ja rodando, sofre disturbios aleatorios que ninguem controla. O operator nao empurra configuracao — ele **observa**, **avalia**, **decide** e **age**.

O spec define a **politica supervisoria**: quais variaveis monitorar, quais sao os limites aceitaveis, e o que fazer quando um limite e violado. O status e a **memoria** do operator: o que ele viu por ultimo, pra onde os valores estao indo, e o que ele fez.

```yaml
apiVersion: infrastructure.greenlabs.io/v1alpha1
kind: PLCMachine
metadata:
  name: tep-baseline
spec:
  plantAddress: "te-plant.default.svc:50051"
  operatingRanges:
    - name: reactor_pressure
      xmeasIndex: 6        # XMEAS(7)
      min: 2600.0
      max: 2800.0
  responseRules:
    - name: pressure_high
      watchRef: reactor_pressure
      condition: above_max
      controllerID: pressure_reactor
      parameter: kp
      adjustValue: 0.15
  monitoringInterval:
    baseMs: 2000
    transientMs: 200
```

Isso diz: "monitore a pressao do reator. Se passar de 2800, aumente o ganho do controlador de pressao pra 0.15. Em regime estavel, cheque a cada 2s. Se a coisa estiver mudando rapido, cheque a cada 200ms."

## Spec (politica supervisoria)

O spec NAO e uma lista de parametros a sincronizar. E um conjunto de regras que o operator usa pra decidir SE e COMO intervir.

| Campo               | Tipo                | Obrigatorio | Descricao |
|---------------------|---------------------|-------------|-----------|
| `plantAddress`      | string              | Sim         | Endpoint gRPC da planta |
| `operatingRanges`   | []OperatingRange    | Nao         | Faixas aceitaveis pra XMEAS |
| `responseRules`     | []ResponseRule      | Nao         | Regras de resposta a violacoes |
| `monitoringInterval`| MonitoringInterval  | Nao         | Frequencia de polling adaptativa |

### OperatingRange

Define os limites aceitaveis pra uma variavel medida da planta.

| Campo        | Tipo    | Obrigatorio | Descricao |
|--------------|---------|-------------|-----------|
| `name`       | string  | Sim         | ID legivel (referenciado por responseRules) |
| `xmeasIndex` | int     | Sim         | Qual XMEAS monitorar (0-based, 0-40) |
| `min`        | float   | Sim         | Limite inferior aceitavel |
| `max`        | float   | Sim         | Limite superior aceitavel |

> **Nota sobre indices**: `xmeasIndex: 6` = XMEAS(7) na nomenclatura TEP (zero-based no Go, 1-based no paper do Downs & Vogel).

### ResponseRule

Define o que o operator faz quando uma variavel sai da faixa.

| Campo          | Tipo   | Obrigatorio | Descricao |
|----------------|--------|-------------|-----------|
| `name`         | string | Sim         | Nome legivel da regra |
| `watchRef`     | string | Sim         | Nome do OperatingRange que dispara |
| `condition`    | string | Sim         | `above_max` ou `below_min` |
| `controllerID` | string | Sim        | Qual controller ajustar na planta |
| `parameter`    | string | Sim         | Qual parametro: `kp`, `ki`, `kd`, `setpoint`, `bias`, `enabled` |
| `adjustValue`  | float  | Sim         | Valor a setar. Pra `enabled`, 1.0=ligar, 0.0=desligar |

> **Importante**: a decisao de agir vem da leitura de XMEAS vs faixas, NAO da comparacao spec vs parametros atuais do controller. O operator le o estado da planta e reage. Nao sincroniza config.

### MonitoringInterval

| Campo         | Tipo | Obrigatorio | Default | Descricao |
|---------------|------|-------------|---------|-----------|
| `baseMs`      | int  | Nao         | 2000    | Intervalo quando estavel (ms) |
| `transientMs` | int  | Nao         | 200     | Intervalo durante transitorios (ms) |

O operator detecta transitorio quando os valores de XMEAS estao mudando rapido entre leituras consecutivas. Nesse caso, ele encurta o polling pra monitorar mais de perto.

## Status (memoria do operator)

O status NAO e um espelho pra `kubectl get`. E a memoria interna do operator. Ele grava:

- O que viu por ultimo (valores de XMEAS)
- O que viu antes disso (pra calcular tendencia)
- Se ta subindo, descendo, ou estavel
- A ultima acao que tomou

| Campo               | Tipo               | Descricao |
|---------------------|--------------------|-----------|
| `phase`             | string             | Estado percebido da planta (ver abaixo) |
| `plantTime`         | float              | Relogio de simulacao (horas) |
| `isdActive`         | bool               | True se ISD (parada de emergencia) |
| `variables`         | []VariableStatus   | Memoria de cada variavel monitorada |
| `lastAction`        | *ActionTaken       | Ultima acao supervisoria executada |
| `lastReconcileTime` | timestamp          | Quando o operator leu por ultimo |
| `conditions`        | []Condition        | Condicoes K8s padrao |

### Phases

| Phase        | Significado |
|--------------|-------------|
| `Pending`    | Operator ainda nao conectou na planta |
| `Stable`     | Todas as variaveis dentro da faixa, tendencias estaveis |
| `Transient`  | Valores mudando rapido — operator monitora com frequencia alta |
| `Alarm`      | Uma ou mais variaveis fora da faixa aceitavel |
| `Shutdown`   | Planta acionou ISD (parada de emergencia) |

### VariableStatus

| Campo           | Tipo   | Descricao |
|-----------------|--------|-----------|
| `name`          | string | Nome da variavel (mesmo do OperatingRange) |
| `xmeasIndex`    | int    | Qual XMEAS |
| `value`         | float  | Leitura atual |
| `previousValue` | float  | Leitura anterior (pra trend) |
| `trend`         | string | `Rising`, `Falling`, ou `Stable` |
| `inRange`       | bool   | Dentro da faixa? |

### ActionTaken

| Campo          | Tipo      | Descricao |
|----------------|-----------|-----------|
| `ruleName`     | string    | Qual ResponseRule disparou |
| `controllerID` | string   | Qual controller foi ajustado |
| `parameter`    | string    | Qual parametro mudou |
| `value`        | float     | Valor que foi setado |
| `timestamp`    | timestamp | Quando |

## kubectl get

```
$ kubectl get plcmachines
NAME           PHASE     PLANT                             TIME (h)   ISD     AGE
tep-baseline   Stable    te-plant.default.svc:50051        12.345     false   2h
```

## Mapeamento CRD <> gRPC

| O que o operator faz                     | RPC gRPC             |
|------------------------------------------|----------------------|
| Le XMEAS da planta                       | `GetPlantStatus`     |
| Stream continuo de metricas              | `StreamMetrics`      |
| Descobre controllers existentes          | `ListControllers`    |
| Ajusta parametro de um controller        | `UpdateController`   |

> **Nota**: nao tem `AddController`, `RemoveController`, nem `SetDisturbance`. O operator nao cria controllers e nao controla disturbios. Disturbios sao aleatorios — o operator reage aos efeitos.
