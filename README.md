# ClusterClaim Operator

Kubernetes-оператор для декларативного управления жизненным циклом кластеров (infra + client) через единый ресурс `ClusterClaim`.

## Что это?

ClusterClaim Operator автоматизирует создание и оркестрацию полного набора зависимых ресурсов для кластера: ArgoCD Application, CertificateSet'ы, CAPI Cluster'ы, CcmCsrc и ConfigMap'ы в remote кластерах. Вместо ручного управления десятками ресурсов, вы описываете желаемое состояние через один `ClusterClaim`, а оператор выполняет 13-шаговый pipeline для его достижения.

## Ключевые возможности

- **Единая точка входа** — один `ClusterClaim` определяет полный набор ресурсов кластера
- **Pipeline из 13 шагов** — автоматическая оркестрация с wait-условиями между шагами
- **Шаблонизация** — Go text/template + Sprig для рендеринга ресурсов из `ClusterClaimObserveResourceTemplate`
- **Infra + Client режимы** — поддержка двух pipeline'ов: полный (infra + client) и сокращённый (infra-only)
- **Remote операции** — apply ConfigMap'ов в infra cluster через kubeconfig
- **Event-driven watches** — автоматическое продвижение pipeline по событиям от managed ресурсов

## Архитектура

```
┌─────────────────┐     ┌──────────────┐     ┌─────────────────────────┐
│  ClusterClaim   │────▶│  ClusterClaim│────▶│  Pipeline (13 шагов)    │
│     CRD         │     │  Controller  │     │  1.  Application        │
└─────────────────┘     └──────────────┘     │  2.  CertificateSet[i]  │
        ▲                      │             │  3.  WAIT CertSet Ready │
        │                      ▼             │  4.  Cluster[infra]     │
┌─────────────────┐     ┌──────────────┐     │  5.  WAIT Provisioned   │
│ ClusterClaim    │     │   Template   │     │  6.  CertificateSet[c]  │
│ ObserveResource │────▶│  Rendering   │     │  7.  WAIT CP Ready      │
│ Template (CRD)  │     │  (Go + Sprig)│     │  8.  CcmCsrc            │
└─────────────────┘     └──────────────┘     │  9.  ConfigMaps (remote)│
                                             │  10. Cluster[client]    │
                                             │  11. WAIT Client CP     │
                                             │  12. CcmCsrc (update)   │
                                             │  13. READY              │
                                             └─────────────────────────┘
```

## Быстрый старт

### Установка CRD

```bash
make install
```

### Развёртывание оператора

```bash
make deploy IMG=<your-registry>/cluster-claim-operator:latest
```

### Создание первого кластера

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
      kubernetes:
        version: "v1.34.4"
  client:
    enabled: false
```

```bash
kubectl apply -f clusterclaim.yaml
```

### Проверка статуса

```bash
kubectl get clusterclaim ec8a00 -n dlputi1u
```

## Документация

Подробная документация доступна в директории [docs/](docs/):

| Раздел | Описание |
|--------|----------|
| [Быстрый старт](docs/getting-started.md) | Установка и первые шаги |
| [Концепции](docs/concepts/) | ClusterClaim, шаблоны, pipeline |
| [Руководство](docs/user-guide/) | Пошаговые инструкции |
| [Справочник API](docs/reference/) | Спецификации CRD |
| [Примеры](docs/examples/) | Готовые конфигурации |
| [Устранение неполадок](docs/troubleshooting.md) | Решение проблем |

## Требования

- Kubernetes 1.31+
- ClusterAPI v1beta2
- Argo CD
- cert-manager (CertificateSet)
- Go 1.24+ (для сборки)

## Разработка

```bash
# Запуск локально
make run

# Тесты
make test

# Линтер
make lint

# Генерация CRD манифестов
make manifests

# Генерация кода (DeepCopy)
make generate
```

## Лицензия

Apache License 2.0. См. [LICENSE](LICENSE).
