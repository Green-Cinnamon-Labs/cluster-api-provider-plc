# 03 — CRD PLCMachine

A PLCMachine e o unico recurso customizado desse operator. Ela representa uma conexao supervisoria entre o K8s e uma instancia da planta TEP rodando como servico gRPC.

## A ideia

Os controladores ja existem na planta — sao criados no codigo Rust (main.rs). O operator nao cria nem remove controladores. Ele **observa** metricas via gRPC streaming e **ajusta** parametros dos controladores existentes pra manter a planta no estado desejado.

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
      kp: 0.1
      setpoint: 2705.0
      bias: 40.06
    - id: level_separator
      kp: 1.0
      setpoint: 50.0
  disturbances: []
```

E o operator garante que esses controladores operem com esses parametros. Se alguem mudar o `kp` no YAML, o operator atualiza via gRPC. Campos ausentes (nil) = "nao mexe nesse parametro".

## Spec (politica de controle)

| Campo              | Tipo               | Obrigatorio | Descricao |
|--------------------|---------------------|-------------|-----------|
| `plantAddress`     | string              | Sim         | Endpoint gRPC da planta (ex: `te-plant:50051`) |
| `controllers`      | []ControllerPolicy  | Nao         | Parametros desejados pra controllers existentes |
| `disturbances`     | []int (1-20)        | Nao         | Canais IDV pra ativar. Vazio = sem disturbios |
| `metricsIntervalMs`| int                 | Nao         | Intervalo de polling do status (default: 1000ms) |

### ControllerPolicy

Diferente de um ControllerSpec que "cria" — aqui voce so diz quais parametros quer ajustar. Tudo e opcional exceto o `id` (precisa saber QUAL controller ajustar).

| Campo      | Tipo    | Obrigatorio | Descricao |
|------------|---------|-------------|-----------|
| `id`       | string  | Sim         | ID do controller na planta (ex: `pressure_reactor`) |
| `kp`       | *float  | Nao         | Ganho proporcional desejado |
| `ki`       | *float  | Nao         | Ganho integral desejado |
| `kd`       | *float  | Nao         | Ganho derivativo desejado |
| `setpoint` | *float  | Nao         | Valor alvo desejado |
| `bias`     | *float  | Nao         | Offset de saida desejado |
| `enabled`  | *bool   | Nao         | Liga/desliga o loop. Nil = nao mexe |

> **Porque ponteiros?** Campos nil significam "nao alterar esse parametro". Se voce so quer mudar o `setpoint` de um controller, coloca so `id` e `setpoint` — o resto fica como esta na planta.

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
| `Running`     | Parametros sincronizados, planta operando normal |
| `Degraded`    | Alarmes ativos ou reconciliacao falhando |
| `Shutdown`    | Planta acionou ISD (parada de emergencia) |

### ControllerStatus

| Campo                | Tipo   | Descricao |
|----------------------|--------|-----------|
| `id`                 | string | Mesmo ID da policy |
| `currentMeasurement` | float  | Valor atual da medicao |
| `currentOutput`      | float  | Valor atual da saida |
| `error`              | float  | (measurement - setpoint). Positivo = acima do alvo |
| `enabled`            | bool   | Se esta ativo na planta |

## kubectl get

A CRD tem print columns configuradas:

```
$ kubectl get plcmachines
NAME           PHASE     PLANT                             TIME (h)   ISD     AGE
tep-baseline   Running   te-plant.default.svc:50051        12.345     false   2h
```

## Mapeamento CRD <> gRPC

O reconciler usa a API gRPC da planta pra manter spec e status em sincronia:

| Operacao do reconciler          | RPC gRPC chamada       |
|---------------------------------|------------------------|
| Ler controllers atuais          | `ListControllers`      |
| Ajustar parametros              | `UpdateController`     |
| Ativar/desativar disturbio      | `SetDisturbance`       |
| Ler metricas pra status         | `GetPlantStatus`       |
| Stream continuo (futuro)        | `StreamMetrics`        |

> **Nota:** nao tem `AddController` nem `RemoveController` no fluxo do operator. Os controllers sao criados no codigo Rust da planta. O operator so ajusta parametros dos que ja existem.
