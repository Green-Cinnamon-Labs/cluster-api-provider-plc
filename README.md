# cluster-api-provider-plc

Operator Kubernetes para supervisao da planta Tennessee Eastman Process (TEP). Funciona como um controlador supervisorio: recebe a descricao do estado desejado via CRD (`PLCMachine`) e reconcilia com a planta via gRPC.

## Contexto

Esse operator faz parte do **TEP Digital Twin Lab** — um laboratorio onde o Kubernetes atua como sistema supervisorio de uma planta quimica simulada. A planta roda como um servico Rust com API gRPC; esse operator conecta nela, configura controladores e monitora metricas.

```
kubectl apply -f plcmachine.yaml
  → operator le o spec
    → conecta via gRPC na planta
      → adiciona/atualiza/remove controllers
        → monitora metricas e atualiza status
```

## Quick start

```bash
# Pre-requisitos: Go 1.25+, Docker, Kind, kubectl

# Gerar codigo e manifests
make generate manifests

# Build da imagem
make docker-build IMG=controller:latest

# Criar cluster Kind e deployar
kind create cluster --name tep-lab
kind load docker-image controller:latest --name tep-lab
make install      # CRDs
make deploy IMG=controller:latest

# Criar um PLCMachine
kubectl apply -f config/samples/infrastructure_v1alpha1_plcmachine.yaml
kubectl get plcmachines
```

## CRD: PLCMachine

Recurso unico do operator. O `.spec` declara o que voce quer; o `.status` reflete o que a planta reporta.

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
      xmeasIndex: 6      # XMEAS(7) — reactor pressure
      xmvIndex: 5        # XMV(6)   — purge valve
      kp: 0.1
      setpoint: 2705.0
      bias: 40.06
  disturbances: []       # baseline — sem disturbios
```

Detalhes completos em [docs/03-crd-plcmachine.md](docs/03-crd-plcmachine.md).

## Documentacao

| Doc                                                        | Conteudo                                                          |
| ---------------------------------------------------------- | ----------------------------------------------------------------- |
| [01 — Visao geral](docs/01-visao-geral.md)                 | O que e esse repo, onde se encaixa, estado atual                  |
| [02 — Anatomia do projeto](docs/02-anatomia-do-projeto.md) | Mapa de arquivos: o que editar vs o que e gerado pelo Kubebuilder |
| [03 — CRD PLCMachine](docs/03-crd-plcmachine.md)           | Spec, status, phases, mapeamento com gRPC                         |
| [04 — Reconciliacao](docs/04-reconciliacao.md)             | Como o reconciler funciona (design, fluxo, idempotencia)          |

## Repositorios do lab

| Repo                                                                                    | O que faz                          |
| --------------------------------------------------------------------------------------- | ---------------------------------- |
| [spec-tennessee-eastman](https://github.com/Green-Cinnamon-Labs/spec-tennessee-eastman) | Issues, specs, decisoes            |
| [fork-tennesseeEastman](https://github.com/Green-Cinnamon-Labs/fork-tennesseeEastman)   | Planta TEP (Rust) + gRPC server    |
| **cluster-api-provider-plc**                                                            | **Este repo** — operator K8s (Go)  |
| [lab-k8s-supervisor](https://github.com/Green-Cinnamon-Labs/lab-k8s-supervisor)         | Infra do cluster (Kind, manifests) |

## Estrutura

```
api/v1alpha1/          ← CRD types (PLCMachine spec/status)
internal/controller/   ← Reconciler (logica de negocio)
cmd/main.go            ← Entry point do manager
config/                ← Kustomize: CRD, RBAC, deployment
docs/                  ← Documentacao do projeto
```

> Para entender o que cada arquivo faz e o que e boilerplate do Kubebuilder, veja [docs/02-anatomia-do-projeto.md](docs/02-anatomia-do-projeto.md).

## Status do desenvolvimento

- [x] Scaffold Kubebuilder
- [x] CRD PLCMachine com campos reais (#37)
- [ ] Reconciler com gRPC client (#38)
- [ ] Setup Kind cluster (#39)
- [ ] Deploy planta + operator (#40)
- [ ] Teste E2E: CR → IDV → reconciliacao (#41)
