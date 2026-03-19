# 03 — CRD PLCMachine

A PLCMachine e o unico recurso customizado desse operator. Ela representa uma conexao entre o K8s e uma instancia da planta TEP rodando como servico gRPC.

## A ideia

Voce escreve um YAML assim:

```yaml
apiVersion: infrastructure.greenlabs.io/v1alpha1
kind: PLCMachine
metadata:
  name: tep-baseline
spec:
  plantAddress: "te-plant.default.svc:50051"
  controllers:
    - id: pressure_reactor
      controllerType: P
      xmeasIndex: 6
      xmvIndex: 5
      kp: 0.1
      setpoint: 2705.0
      bias: 40.06
  disturbances: []
```

E o operator garante que a planta tenha exatamente esses controladores com exatamente esses parametros. Se alguem mudar o `kp` no YAML, o operator atualiza o controlador na planta via gRPC. Se ativar um disturbio, o operator manda o `SetDisturbance`. Reconciliacao continua — o jeito Kubernetes de fazer as coisas.

## Spec (estado desejado)

| Campo              | Tipo              | Obrigatorio | Descricao |
|--------------------|-------------------|-------------|-----------|
| `plantAddress`     | string            | Sim         | Endpoint gRPC da planta (ex: `te-plant:50051`) |
| `controllers`      | []ControllerSpec  | Nao         | Loops de controle desejados |
| `disturbances`     | []int (1-20)      | Nao         | Canais IDV pra ativar. Vazio = sem disturbios |
| `metricsIntervalMs`| int               | Nao         | Intervalo de polling do status (default: 1000ms) |

### ControllerSpec

| Campo            | Tipo    | Obrigatorio | Validacao      | Descricao |
|------------------|---------|-------------|----------------|-----------|
| `id`             | string  | Sim         |                | Identificador unico (ex: `pressure_reactor`) |
| `controllerType` | string | Sim         | P, PI, ou PID  | Tipo do controlador |
| `xmeasIndex`     | int     | Sim         | 0-21           | Qual medicao ler (XMEAS) |
| `xmvIndex`       | int     | Sim         | 0-11           | Qual atuador comandar (XMV) |
| `kp`             | float   | Sim         |                | Ganho proporcional |
| `ki`             | float   | Nao         |                | Ganho integral |
| `kd`             | float   | Nao         |                | Ganho derivativo |
| `setpoint`       | float   | Sim         |                | Valor alvo pra medicao |
| `bias`           | float   | Sim         |                | Offset de saida |
| `enabled`        | bool    | Nao         | default: true  | Liga/desliga o loop |

> **Nota sobre os indices**: `xmeasIndex: 6` mapeia para `XMEAS(7)` na nomenclatura TEP (zero-based no Go, 1-based no paper do Downs & Vogel). Os comentarios no sample YAML deixam isso explicito.

## Status (estado observado)

O operator preenche o status a cada ciclo de reconciliacao, lendo metricas da planta via gRPC.

| Campo                | Tipo                  | Descricao |
|----------------------|-----------------------|-----------|
| `phase`              | string                | Estado de alto nivel (ver abaixo) |
| `plantTime`          | float                 | Relogio de simulacao (horas) |
| `isdActive`          | bool                  | True se a planta deu shutdown de emergencia |
| `derivNorm`          | float                 | Norma da derivada do solver ODE |
| `controllers`        | []ControllerStatus    | Estado real de cada loop |
| `activeDisturbances` | []int                 | Canais IDV ativos na planta |
| `alarms`             | []AlarmStatus         | Alarmes ativos |
| `lastReconcileTime`  | timestamp             | Quando o operator sincronizou por ultimo |
| `conditions`         | []Condition           | Condicoes K8s padrao |

### Phases

| Phase         | Significado |
|---------------|-------------|
| `Pending`     | Operator ainda nao conectou na planta |
| `Connected`   | Conexao gRPC ok, mas controllers nao sincronizados |
| `Running`     | Tudo sincronizado, planta operando normal |
| `Degraded`    | Alarmes ativos ou reconciliacao falhando |
| `Shutdown`    | Planta acionou ISD (parada de emergencia) |

### ControllerStatus

| Campo                | Tipo   | Descricao |
|----------------------|--------|-----------|
| `id`                 | string | Mesmo ID do spec |
| `currentMeasurement` | float  | Valor atual de xmeas[xmeasIndex] |
| `currentOutput`      | float  | Valor atual de xmv[xmvIndex] |
| `error`              | float  | (measurement - setpoint). Positivo = acima do alvo |
| `enabled`            | bool   | Se esta ativo na planta |

## kubectl get

A CRD tem print columns configuradas:

```
$ kubectl get plcmachines
NAME           PHASE     PLANT                             TIME (h)   ISD     AGE
tep-baseline   Running   te-plant.default.svc:50051        12.345     false   2h
```

## Mapeamento CRD ↔ gRPC

O reconciler vai usar a API gRPC da planta pra manter spec e status em sincronia:

| Operacao do reconciler          | RPC gRPC chamada       |
|---------------------------------|------------------------|
| Adicionar controller            | `AddController`        |
| Remover controller              | `RemoveController`     |
| Atualizar parametros            | `UpdateController`     |
| Listar controllers atuais       | `ListControllers`      |
| Ativar/desativar disturbio      | `SetDisturbance`       |
| Ler metricas pra status         | `GetPlantStatus`       |
| Stream continuo (futuro)        | `StreamMetrics`        |
