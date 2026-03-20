# Документация ClusterClaim Operator

ClusterClaim Operator автоматизирует создание и оркестрацию кластерных ресурсов через декларативные CRD и 13-шаговый pipeline.

## Обзор

```
┌─────────────────┐     ┌──────────────┐     ┌─────────────────────────┐
│  ClusterClaim   │────▶│  ClusterClaim│────▶│  Pipeline (13 шагов)    │
│     CRD         │     │  Controller  │     │                         │
└─────────────────┘     └──────────────┘     │  Application            │
        ▲                      │             │  CertificateSet[infra]  │
        │                      ▼             │  Cluster[infra]         │
┌─────────────────┐     ┌──────────────┐     │  CertificateSet[client] │
│ ClusterClaim    │     │   Template   │     │  CcmCsrc                │
│ ObserveResource │────▶│   Renderer   │     │  ConfigMaps (remote)    │
│ Template        │     │  (Go + Sprig)│     │  Cluster[client]        │
└─────────────────┘     └──────────────┘     └─────────────────────────┘
```

## Быстрые ссылки

| Раздел | Описание |
|--------|----------|
| [Быстрый старт](getting-started.md) | Установка оператора и создание первого кластера |
| [Концепции](concepts/) | Основные ресурсы |
| [Руководство пользователя](user-guide/) | Пошаговые инструкции |
| [Справочник](reference/) | Спецификации API |
| [Примеры](examples/) | Готовые конфигурации кластеров |
| [Устранение неполадок](troubleshooting.md) | Частые проблемы и решения |

## Концепции

Оператор использует два Custom Resource Definition (CRD):

### [ClusterClaim](concepts/clusterclaim.md)

Основной namespaced-ресурс, описывающий кластер — набор ссылок на шаблоны, параметры инфраструктуры, сети и компонентов:

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
  client:
    enabled: false
```

### [ClusterClaimObserveResourceTemplate](concepts/template.md)

Cluster-scoped шаблон, определяющий GVK и Go template для генерации Kubernetes-ресурса:

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
      labels:
        cluster.x-k8s.io/cluster-name: {{ .ClusterClaim.metadata.name }}-infra
    spec:
      environment: {{ .ClusterClaim.spec.infra.role }}
      issuerRef:
        kind: ClusterIssuer
        name: selfsigned
```

### [Pipeline](concepts/pipeline.md)

13-шаговый конвейер создания ресурсов с wait-шагами между ними:

```
Step 1:  Application           → Create/Update
Step 2:  CertificateSet[infra] → Create/Update
Step 3:  WAIT                  → CertificateSet[infra] Ready=True
Step 4:  Cluster[infra]        → Create/Update
Step 5:  WAIT                  → Cluster[infra] infrastructureProvisioned
Step 6:  CertificateSet[client]→ Create/Update        [skip если client.enabled=false]
Step 7:  WAIT                  → Cluster[infra] controlPlaneInitialized
Step 8:  CcmCsrc               → Create/Update
Step 9:  ConfigMap (remote)    → Apply в infra cluster [skip если configMapTemplateRef нет]
Step 10: Cluster[client]       → Create/Update        [skip если client.enabled=false]
Step 11: WAIT                  → Cluster[client] CP    [skip если client.enabled=false]
Step 12: CcmCsrc (update)     → Update client=true    [skip если client.enabled=false]
Step 13: READY
```

## Возможности

- **Декларативное управление** — определяйте кластеры как Kubernetes-ресурсы
- **Шаблонизация** — Go text/template + Sprig для генерации любых ресурсов
- **Два режима** — infra-only (`client.enabled=false`) и infra+client (`client.enabled=true`)
- **Remote операции** — apply ConfigMap'ов в infra cluster через kubeconfig
- **Event-driven** — watches на managed ресурсы ускоряют продвижение pipeline
- **Drift detection** — periodic requeue (10 мин) для проверки состояния Ready-кластеров

## Status Conditions

ClusterClaim сообщает своё состояние через стандартные Kubernetes conditions:

| Condition | Описание |
|-----------|----------|
| `Ready` | Кластер полностью создан, pipeline завершён |
| `ApplicationCreated` | Application (ArgoCD) создан |
| `InfraCertificateReady` | CertificateSet[infra] в состоянии Ready |
| `InfraProvisioned` | Cluster[infra] инфраструктура создана |
| `InfraCPReady` | Cluster[infra] control plane инициализирован |
| `CcmCsrcCreated` | CcmCsrc создан |
| `RemoteConfigApplied` | Remote ConfigMaps применены |
| `ClientCPReady` | Cluster[client] control plane инициализирован |
| `Paused` | Reconciliation приостановлен |

## Помощь

- [Устранение неполадок](troubleshooting.md) — частые проблемы и решения
- [GitHub Issues](https://github.com/PRO-Robotech/cluster-claim-operator/issues) — сообщить об ошибке

## Лицензия

Apache License 2.0
