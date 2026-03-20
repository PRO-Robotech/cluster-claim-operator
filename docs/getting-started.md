# Быстрый старт

Руководство по установке оператора и созданию первого кластера.

## Предварительные требования

- Kubernetes-кластер 1.31+ (management cluster)
- [ClusterAPI](https://cluster-api.sigs.k8s.io/) v1beta2 установлен в management-кластере
- [Argo CD](https://argo-cd.readthedocs.io/) установлен
- [cert-manager](https://cert-manager.io/) установлен (для CertificateSet)
- `kubectl` настроен на management-кластер

## 1. Установка оператора

### Установка CRD

```bash
make install
```

### Развёртывание контроллера

```bash
make deploy IMG=<your-registry>/cluster-claim-operator:latest
```

Или для локальной разработки:

```bash
make run
```

### Проверка установки

```bash
kubectl get pods -n cluster-claim-operator-system
kubectl get crd | grep clusterclaim.in-cloud.io
```

Ожидаемый результат:

```
NAME                                                              CREATED AT
clusterclaims.clusterclaim.in-cloud.io                            2026-03-20T00:00:00Z
clusterclaimobserveresourcetemplates.clusterclaim.in-cloud.io     2026-03-20T00:00:00Z
```

## 2. Создание шаблонов

Шаблоны — cluster-scoped ресурсы `ClusterClaimObserveResourceTemplate`, общие для всех namespace.

Создайте минимальный набор шаблонов для infra-only кластера:

**Шаблон Application (ArgoCD):**

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaimObserveResourceTemplate
metadata:
  name: default-observe
spec:
  apiVersion: argoproj.io/v1alpha1
  kind: Application
  value: |
    metadata:
      labels:
        xcluster.in-cloud.io/name: {{ .ClusterClaim.metadata.name }}
    spec:
      destination:
        name: in-cluster
        namespace: {{ .ClusterClaim.metadata.namespace }}
      project: common
      source:
        helm:
          releaseName: {{ .ClusterClaim.metadata.name }}
        path: .
        repoURL: https://gitlab.beget.ru/cloud/k8s/charts/cluster
        targetRevision: HEAD
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
```

**Шаблон CertificateSet[infra]:**

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaimObserveResourceTemplate
metadata:
  name: default-certset-infra
spec:
  apiVersion: in-cloud.io/v1alpha1
  kind: CertificateSet
  value: |
    metadata:
      annotations:
        secret-copy.in-cloud.io/dstClusterKubeconfig: {{ .ClusterClaim.metadata.namespace }}/{{ .ClusterClaim.metadata.name }}-infra-kubeconfig
        secret-copy.in-cloud.io/dstNamespace: beget-system
      labels:
        cluster.x-k8s.io/cluster-name: {{ .ClusterClaim.metadata.name }}-infra
    spec:
      environment: {{ .ClusterClaim.spec.infra.role }}
      issuerRef:
        kind: ClusterIssuer
        name: selfsigned
```

**Шаблон Cluster[infra]** и **CcmCsrc** — см. [примеры](examples/).

Примените шаблоны:

```bash
kubectl apply -f templates/
```

## 3. Создание ClusterClaim

Создайте файл `clusterclaim.yaml` для infra-only кластера:

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaim
metadata:
  name: ec8a00
  namespace: dlputi1u
spec:
  observeTemplateRef:
    name: "default-observe"
  certificateSetTemplateRef:
    infra:
      name: "default-certset-infra"
  clusterTemplateRef:
    infra:
      name: "v1.34.4"
  ccmCsrTemplateRef:
    name: "default-ccm"

  replicas: 1
  configuration:
    cpuCount: 6
    diskSize: 51200
    memory: 12288

  infra:
    role: "customer/infra"
    paused: false
    network:
      serviceCidr: "10.96.0.0/12"
      podCidr: "10.244.0.0/16"
      kubeApiserverPort: 6443
      clusterDNS: "10.96.0.10"
    componentVersions:
      kubernetes: { version: "v1.34.4" }
      containerd: { version: "1.7.19" }
      runc: { version: "1.1.12" }
      etcd: { version: "3.5.12" }

  client:
    enabled: false
```

Примените:

```bash
kubectl apply -f clusterclaim.yaml
```

## 4. Проверка

```bash
# Статус ClusterClaim
kubectl get clusterclaim ec8a00 -n dlputi1u

# Детальный статус
kubectl get clusterclaim ec8a00 -n dlputi1u -o jsonpath='{range .status.conditions[*]}{.type}: {.status} ({.reason}){"\n"}{end}'
```

Ожидаемый результат после завершения pipeline:

```
Ready: True (Ready)
ApplicationCreated: True (Created)
InfraCertificateReady: True (Ready)
InfraProvisioned: True (Provisioned)
InfraCPReady: True (Ready)
CcmCsrcCreated: True (Created)
```

### Проверка созданных ресурсов

```bash
# Application (ArgoCD)
kubectl get application ec8a00 -n dlputi1u

# CertificateSet[infra]
kubectl get certificateset ec8a00-infra -n dlputi1u

# Cluster[infra] (CAPI)
kubectl get cluster ec8a00-infra -n dlputi1u

# CcmCsrc
kubectl get ccmcsrc ec8a00 -n dlputi1u
```

## Понимание процесса

```
ClusterClaim (ec8a00)
    │
    │ ─── Шаблоны: ────────────────────────────────────────
    │
    ├──► observeTemplateRef         → Application (ArgoCD)
    ├──► certificateSetTemplateRef  → CertificateSet[infra]
    │                                   │
    │                                   ▼ WAIT Ready
    ├──► clusterTemplateRef         → Cluster[infra] (CAPI)
    │                                   │
    │                                   ▼ WAIT Provisioned, CP Initialized
    ├──► ccmCsrTemplateRef          → CcmCsrc
    └──► configMapTemplateRef       → ConfigMaps (remote, опционально)
```

1. **ClusterClaim** определяет что создать (ссылки на шаблоны, параметры)
2. **ClusterClaimObserveResourceTemplate** определяет как создать (Go template → YAML)
3. Контроллер выполняет pipeline: рендерит шаблоны, создаёт ресурсы, ждёт условий
4. При завершении pipeline — Phase = `Ready`, requeue каждые 10 минут для drift detection

## Очистка

```bash
kubectl delete clusterclaim ec8a00 -n dlputi1u
```

Оператор автоматически удалит все managed ресурсы в обратном порядке pipeline.

## Следующие шаги

- [Концепции](concepts/clusterclaim.md) — понимание ClusterClaim, шаблонов и pipeline
- [Руководство пользователя](user-guide/) — пошаговые руководства
- [Примеры](examples/) — готовые конфигурации кластеров
- [Устранение неполадок](troubleshooting.md) — типичные проблемы и решения
