# 02 — Anatomia do projeto

O Kubebuilder gera um monte de arquivo quando voce roda o scaffold. A maioria e boilerplate que voce nunca mexe. Esse doc separa o que importa do que e ruido.

## Mapa de arquivos

### Arquivos que voce EDITA

Esses sao os que carregam a logica do projeto:

```
api/v1alpha1/plcmachine_types.go    ← O CONTRATO. Define spec e status da CRD.
internal/controller/
  plcmachine_controller.go          ← O CORACAO. Logica de reconciliacao.
config/samples/
  infrastructure_v1alpha1_plcmachine.yaml  ← Exemplo de CR (como o usuario usa a CRD).
cmd/main.go                         ← Entry point do manager. Ja esta ok.
Makefile                            ← Build, test, deploy. Ja esta ok.
Dockerfile                          ← Build da imagem do operator. Ja esta ok.
```

### Arquivos AUTO-GERADOS (nao edite)

Esses sao regenerados toda vez que voce roda `make manifests` ou `make generate`:

```
api/v1alpha1/zz_generated.deepcopy.go     ← DeepCopy. Gerado a partir dos types.
config/crd/bases/
  infrastructure.greenlabs.io_plcmachines.yaml  ← O YAML da CRD. Gerado dos markers +kubebuilder.
config/rbac/role.yaml                      ← Permissoes do controller. Gerado dos markers +kubebuilder:rbac.
config/rbac/role_binding.yaml              ← Binding. Auto-gerado.
config/rbac/service_account.yaml           ← SA. Auto-gerado.
```

**Regra de ouro**: se o arquivo tem `DO NOT EDIT` no topo ou comeca com `zz_`, nao mexe nele. Edita o source (types.go ou controller.go) e roda `make manifests generate`.

### Outros arquivos que ficaram

```
hack/boilerplate.go.txt     ← Header de licenca pros arquivos gerados. controller-gen precisa dele.
PROJECT                     ← Metadados do Kubebuilder. Nao edite — o CLI usa isso internamente.
.gitignore / .dockerignore  ← Ignore rules.
go.mod / go.sum             ← Dependencias Go.
```

## Os markers `+kubebuilder:`

Esses comentarios magicos nos arquivos Go nao sao decoracao — o `controller-gen` le eles e gera o CRD YAML, RBAC, validacoes, etc.

Exemplos no `plcmachine_types.go`:

```go
// +kubebuilder:validation:Enum=P;PI;PID       → gera enum no OpenAPI schema
// +kubebuilder:validation:Minimum=0            → gera min no schema
// +kubebuilder:validation:Maximum=21           → gera max no schema
// +kubebuilder:default=true                    → valor default no schema
// +kubebuilder:subresource:status              → habilita /status subresource
// +kubebuilder:printcolumn:name="Phase",...    → colunas no kubectl get
```

E no `plcmachine_controller.go`:

```go
// +kubebuilder:rbac:groups=infrastructure.greenlabs.io,resources=plcmachines,...
```

**Ciclo de vida**: edita os markers → roda `make manifests` → YAML atualizado.

## Fluxo de build

```
make generate     → gera zz_generated.deepcopy.go
make manifests    → gera CRD YAML + RBAC YAML a partir dos markers
make build        → compila o binario do manager
make docker-build → imagem Docker
make install      → aplica CRDs no cluster
make deploy       → deploy completo (CRD + RBAC + Deployment)
```

No Windows sem `make`, os comandos equivalentes sao:

```bash
# generate
controller-gen object:headerFile="hack/boilerplate.go.txt" paths="./..."

# manifests
controller-gen rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases

# build
go build -o bin/manager cmd/main.go
```

## O que foi removido do scaffold original

O Kubebuilder gera um projeto pronto pra producao com CI, monitoring, linting pesado, etc.
Pra um lab, isso e peso morto. Removemos em marco/2026:

| Removido                 | O que era                                      | Porque saiu                                                     |
| ------------------------ | ---------------------------------------------- | --------------------------------------------------------------- |
| `.github/workflows/`     | CI (lint, test, e2e)                           | Nao esta configurado pra esse repo. Recria quando tiver CI real |
| `.devcontainer/`         | Config de Codespace/devcontainer               | Nao usamos Codespaces                                           |
| `config/prometheus/`     | ServiceMonitor pro Prometheus                  | Monitoring vem depois, se precisar                              |
| `config/network-policy/` | NetworkPolicy K8s                              | Desnecessario num Kind local                                    |
| `.golangci.yml`          | Config do golangci-lint (24 linters)           | Overkill pro lab. `go vet` resolve                              |
| `.custom-gcl.yml`        | Plugin custom do linter (logcheck)             | Overkill                                                        |
| `AGENTS.md`              | Guia generico do Kubebuilder pra agentes de IA | Substituido pelos nossos `docs/`                                |
| `bin/`                   | Binarios baixados (controller-gen)             | Regenera com `go install` quando quiser                         |

Tambem simplificamos `config/default/kustomization.yaml` — era 235 linhas de boilerplate
comentado (webhooks, cert-manager, etc). Ficou com ~15 linhas, so o que esta ativo.

**Se precisar de volta**: tudo que foi removido e padrao do Kubebuilder. Basta rodar
`kubebuilder init` num repo limpo pra ver os templates, ou consultar o
[Kubebuilder Book](https://book.kubebuilder.io/).
