# ClusterClaim Operator — Техническое задание

## 1. Ресурсы, которыми оперирует оператор

### 1.1. CRD оператора (2 штуки)

| CRD | Scope | Назначение |
|-----|-------|------------|
| `ClusterClaim` | Namespaced | Входной ресурс — плоские переменные + ссылки на шаблоны |
| `ClusterClaimObserveResourceTemplate` | Cluster | Универсальный Go-template. Один CRD для всех типов шаблонов |

### 1.2. Что создаёт оператор

| Ресурс | Где создаётся | GVK |
|--------|---------------|-----|
| `Application` | Management cluster | `argoproj.io/v1alpha1/Application` |
| `CertificateSet[infra]` | Management cluster | `in-cloud.io/v1alpha1/CertificateSet` |
| `CertificateSet[client]` | Management cluster | `in-cloud.io/v1alpha1/CertificateSet` |
| `Cluster[infra]` | Management cluster | `cluster.x-k8s.io/v1beta2/Cluster` |
| `Cluster[client]` | Management cluster | `cluster.x-k8s.io/v1beta2/Cluster` |
| `CcmCsrc` | Management cluster | `controller.in-cloud.io/v1alpha1/CcmCsrc` |
| `ConfigMap[infra]` — `parameters-infra` | **Infra cluster** (remote), `beget-system` | `v1/ConfigMap` |
| `ConfigMap[client]` — `parameters-client` | **Infra cluster** (remote), `beget-system` | `v1/ConfigMap` |

ConfigMap'ы `parameters-infra` и `parameters-client` **рендерятся из шаблонов и применяются в infra cluster** через kubeconfig из Secret `{name}-infra-kubeconfig`. Они содержат конфигурацию (сеть, endpoints, credentials), которую используют компоненты внутри infra cluster.

### 1.3. Что читает оператор (не создаёт)

| Ресурс | Зачем |
|--------|-------|
| `Secret` (`{name}-infra-kubeconfig`) | Kubeconfig для подключения к infra cluster (remote операции) |

---

## 2. Маппинг ClusterClaim → шаблоны

### 2.1. Принцип

Каждое поле `*TemplateRef` в ClusterClaim ссылается **по имени** на экземпляр `ClusterClaimObserveResourceTemplate`. Шаблон содержит Go text/template в `spec.value`, который рендерится в произвольный Kubernetes YAML.

**Один и тот же CRD шаблона** — для всех типов ресурсов. Что именно будет создано — определяется содержимым `spec.value`.

### 2.2. Таблица маппинга

| Поле ClusterClaim | Тип | Что рендерит | Обязательное |
|--------------------|-----|--------------|--------------|
| `spec.observeTemplateRef.name` | `string` | → `Application` (ArgoCD) | да |
| `spec.certificateSetTemplateRef.infra.name` | `string` | → `CertificateSet[infra]` | да |
| `spec.certificateSetTemplateRef.client.name` | `*string` | → `CertificateSet[client]` | если `client.enabled` |
| `spec.clusterTemplateRef.infra.name` | `string` | → `Cluster[infra]` | да |
| `spec.clusterTemplateRef.client.name` | `*string` | → `Cluster[client]` | если `client.enabled` |
| `spec.ccmCsrTemplateRef.name` | `string` | → `CcmCsrc` | да |
| `spec.configMapTemplateRef.infra.name` | `*string` | → `ConfigMap[infra]` (remote) | нет |
| `spec.configMapTemplateRef.client.name` | `*string` | → `ConfigMap[client]` (remote) | нет |

### 2.3. Визуализация маппинга

```
ClusterClaim "ec8a00"                 ClusterClaimObserveResourceTemplate
┌──────────────────────────┐
│ observeTemplateRef ──────┼───▶ "default-observe"        ══▶ Application
│                          │
│ certSetTemplateRef       │
│   .infra ────────────────┼───▶ "default-certset-infra"  ══▶ CertificateSet[infra]
│   .client ───────────────┼───▶ "default-certset-client" ══▶ CertificateSet[client]
│                          │
│ clusterTemplateRef       │
│   .infra ────────────────┼───▶ "v1.34.4"                ══▶ Cluster[infra]
│   .client ───────────────┼───▶ "v1.35.2"                ══▶ Cluster[client]
│                          │
│ ccmCsrTemplateRef ───────┼───▶ "default-ccm"            ══▶ CcmCsrc
│                          │
│ configMapTemplateRef     │
│   .infra ────────────────┼───▶ "default-cm-infra"       ══▶ ConfigMap[infra]  (remote)
│   .client ───────────────┼───▶ "default-cm-client"      ══▶ ConfigMap[client] (remote)
└──────────────────────────┘
```

### 2.4. Именование создаваемых ресурсов

`metadata.name` и `metadata.namespace` **задаёт оператор**, а не шаблон. Шаблон содержит только `apiVersion`, `kind`, `metadata.labels`, `metadata.annotations` и `spec`.

Это исключает коллизии имён: разные шаблоны не могут случайно создать ресурсы с одинаковыми именами, и оператор гарантирует уникальность.

**Naming conventions (management cluster)**:

| templateRef поле | `metadata.name` | `metadata.namespace` |
|------------------|-----------------|----------------------|
| `observeTemplateRef` | `{claim.name}` | `{claim.namespace}` |
| `certificateSetTemplateRef.infra` | `{claim.name}-infra` | `{claim.namespace}` |
| `certificateSetTemplateRef.client` | `{claim.name}-client` | `{claim.namespace}` |
| `clusterTemplateRef.infra` | `{claim.name}-infra` | `{claim.namespace}` |
| `clusterTemplateRef.client` | `{claim.name}-client` | `{claim.namespace}` |
| `ccmCsrTemplateRef` | `{claim.name}` | `{claim.namespace}` |

**Naming conventions (remote — infra cluster)**:

| templateRef поле | `metadata.name` | `metadata.namespace` | role |
|------------------|-----------------|----------------------|------|
| `configMapTemplateRef.infra` | `parameters-infra` | `beget-system` | infra |
| `configMapTemplateRef.infra` | `parameters-system` | `beget-system` | system |
| `configMapTemplateRef.client` | `parameters-client` | `beget-system` | — |

Имена и namespace remote-ресурсов задаёт оператор по конвенции, а **не** шаблон. Шаблон не должен содержать `metadata.name` / `metadata.namespace`.

**Что оператор добавляет после рендера**:

Для management cluster ресурсов:
- `metadata.name` — по конвенции выше
- `metadata.namespace` — из ClusterClaim
- `metadata.ownerReferences` — на ClusterClaim (controller=true)
- Стандартные labels (merge с labels из шаблона):
  - `clusterclaim.in-cloud.io/claim-name`
  - `clusterclaim.in-cloud.io/claim-namespace`

Для remote ресурсов (infra cluster):
- `metadata.name` — по конвенции выше (`parameters-infra` / `parameters-client`)
- `metadata.namespace` — `beget-system`
- Стандартные labels (merge с labels из шаблона):
  - `clusterclaim.in-cloud.io/claim-name`
  - `clusterclaim.in-cloud.io/claim-namespace`

### 2.5. Что содержит шаблон

```
┌─ ClusterClaimObserveResourceTemplate.spec ─────────────────────┐
│                                                                       │
│  apiVersion: ...       ← обязательно (GVK ресурса) (immutable)        │
│  kind: ...             ← обязательно (GVK ресурса) (immutable)        │
│  value: ...            ← обязательно                                  │
└───────────────────────────────────────────────────────────────────────┘
┌─ ClusterClaimObserveResourceTemplate.spec.value ─────────────────────┐
│                                                                       │
│  metadata:                                                            │
│    labels: ...         ← опционально (будут merge с operator labels)  │
│    annotations: ...    ← опционально                                  │
│    ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄                   │
│    name: ЗАПРЕЩЕНО ✗   (оператор задаёт по конвенции)                 │
│    namespace: ЗАПРЕЩЕНО ✗                                             │
│    ┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄┄                   │
│  spec: ...             ← основное содержимое                          │
│  data: ...             ← для ConfigMap                                │
└───────────────────────────────────────────────────────────────────────┘
```

### 2.6. Пример шаблона (management cluster)

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
        xcluster.in-cloud.io/name: {{ .ClusterClaim.metadata.name }}
      annotations:
        secret-copy.in-cloud.io/dstClusterKubeconfig: {{ .ClusterClaim.metadata.namespace }}/{{ .ClusterClaim.metadata.name }}-infra-kubeconfig
        secret-copy.in-cloud.io/dstNamespace: beget-system
    spec:
      environment: {{ .ClusterClaim.spec.infra.role }}
      issuerRef:
        apiVersion: cert-manager.io/v1
        kind: ClusterIssuer
        name: selfsigned
```

Оператор после рендера добавит:
- `metadata.name: ec8a00-infra` (по конвенции `{claim.name}-infra`)
- `metadata.namespace: dlputi1u` (из ClusterClaim)
- `metadata.ownerReferences: [...]`

### 2.7. Пример шаблона (remote resource — ConfigMap)

ConfigMap'ы рендерятся и **применяются в infra cluster** (не в management cluster). Оператор подключается к infra cluster через kubeconfig из Secret `{name}-infra-kubeconfig`.

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaimObserveResourceTemplate
metadata:
  name: default-cm-infra
spec:
  apiVersion: v1
  kind: ConfigMap
  value: |
    metadata:
      labels:
        xcluster.in-cloud.io/name: {{ .ClusterClaim.metadata.name }}
    data:
      environment: infra
      clusterName: {{ .ClusterClaim.metadata.name }}-infra
      clusterHost: {{ .InfraControlPlaneEndpoint.host }}
      clusterPort: "{{ .ClusterClaim.spec.infra.network.kubeApiserverPort }}"
      controlPlaneAvailableReplicas: "{{ .InfraControlPlaneAvailableReplicas }}"
      controlPlaneDesiredReplicas: "{{ .InfraControlPlaneDesiredReplicas }}"
      customer: {{ .ClusterClaim.metadata.namespace }}
      ServiceCidr: {{ .ClusterClaim.spec.infra.network.serviceCidr }}
      podCidr: {{ .ClusterClaim.spec.infra.network.podCidr }}
      # ... и другие параметры
```

Оператор после рендера добавит:
- `metadata.name: parameters-infra` (по конвенции для `configMapTemplateRef.infra`)
- `metadata.namespace: beget-system`
- Стандартные labels

---

## 3. Использование значений ClusterClaim в шаблонах

### 3.1. Контекст шаблонизации

При рендеринге шаблон получает структуру `TemplateContext`:

```go
type TemplateContext struct {
    // Всегда доступно — полная копия ClusterClaim
    ClusterClaim  ClusterClaimSnapshot

    // Вычисляемые поля — появляются по мере прохождения pipeline
    InfraControlPlaneEndpoint           *ControlPlaneEndpoint  // host + port из Cluster[infra]
    InfraControlPlaneInitialized        bool
    InfraControlPlaneAvailableReplicas  int32
    InfraControlPlaneDesiredReplicas    int32
    ClientControlPlaneInitialized       bool
}
```

### 3.2. Доступ к полям ClusterClaim из шаблона

**Всё, что есть в spec ClusterClaim — доступно в шаблоне через `.ClusterClaim`**:

| Выражение в шаблоне | Что возвращает |
|---------------------|----------------|
| `{{ .ClusterClaim.metadata.name }}` | Имя claim (например `ec8a00`) |
| `{{ .ClusterClaim.metadata.namespace }}` | Namespace (например `dlputi1u`) |
| `{{ .ClusterClaim.spec.replicas }}` | Количество реплик CP |
| `{{ .ClusterClaim.spec.configuration.cpuCount }}` | CPU |
| `{{ .ClusterClaim.spec.configuration.diskSize }}` | Диск |
| `{{ .ClusterClaim.spec.configuration.memory }}` | Память |
| `{{ .ClusterClaim.spec.infra.role }}` | Роль infra-кластера (`system/infra`, `customer/infra`) |
| `{{ .ClusterClaim.spec.infra.paused }}` | Пауза infra |
| `{{ .ClusterClaim.spec.infra.network.serviceCidr }}` | Service CIDR infra |
| `{{ .ClusterClaim.spec.infra.network.podCidr }}` | Pod CIDR infra |
| `{{ .ClusterClaim.spec.infra.network.kubeApiserverPort }}` | API server port infra |
| `{{ .ClusterClaim.spec.infra.componentVersions.kubernetes.version }}` | K8s version infra |
| `{{ .ClusterClaim.spec.client.network.kubeApiserverPort }}` | API server port client |
| `{{ .ClusterClaim.spec.client.componentVersions.kubernetes.version }}` | K8s version client |
| `{{ .ClusterClaim.spec.extraEnvs.beget_cluster_region }}` | Произвольная переменная |

### 3.3. Вычисляемые поля (доступны не на всех шагах)

Эти поля заполняются по мере прохождения pipeline — на ранних шагах они ещё `nil`/`false`.

| Выражение в шаблоне | Откуда берётся | С какого шага доступно |
|---------------------|----------------|----------------------|
| `{{ .InfraControlPlaneEndpoint.host }}` | `Cluster[infra].spec.controlPlaneEndpoint.host` | после Step 5 (Cluster[infra] provisioned) |
| `{{ .InfraControlPlaneEndpoint.port }}` | `Cluster[infra].spec.controlPlaneEndpoint.port` | после Step 5 |
| `{{ .InfraControlPlaneInitialized }}` | `Cluster[infra].status.initialization.controlPlaneInitialized` | после Step 7 |
| `{{ .InfraControlPlaneAvailableReplicas }}` | `max(Cluster[infra].status.controlPlane.availableReplicas, 1)` | после Step 7 |
| `{{ .InfraControlPlaneDesiredReplicas }}` | `max(Cluster[infra].status.controlPlane.desiredReplicas, 1)` | после Step 7 |
| `{{ .ClientControlPlaneInitialized }}` | `Cluster[client].status.initialization.controlPlaneInitialized` | после Step 12 |

### 3.4. Какие вычисляемые поля доступны на каком шаге

| Шаг pipeline | `.ClusterClaim` | `.InfraControlPlaneEndpoint` | `.InfraControlPlaneInitialized` | `.InfraControlPlaneAvailableReplicas` | `.InfraControlPlaneDesiredReplicas` | `.ClientControlPlaneInitialized` |
|-------------|:-:|:-:|:-:|:-:|:-:|:-:|
| Application | ✓ | — | — | — | — | — |
| CertificateSet[infra] | ✓ | — | — | — | — | — |
| Cluster[infra] | ✓ | — | — | — | — | — |
| CertificateSet[client] | ✓ | ✓ | — | — | — | — |
| CcmCsrc (первый раз) | ✓ | ✓ | ✓ | — | — | — |
| ConfigMap[infra] | ✓ | ✓ | ✓ | ✓ | ✓ | — |
| ConfigMap[client] | ✓ | ✓ | ✓ | ✓ | ✓ | — |
| Cluster[client] | ✓ | ✓ | ✓ | ✓ | ✓ | — |
| CcmCsrc (повторный) | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

### 3.5. Пример: шаблон CertificateSet[client] с вычисляемыми полями

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaimObserveResourceTemplate
metadata:
  name: default-certset-client
spec:
  apiVersion: in-cloud.io/v1alpha1
  kind: CertificateSet
  value: |
    metadata:
      labels:
        cluster.x-k8s.io/cluster-name: {{ .ClusterClaim.metadata.name }}-client
        xcluster.in-cloud.io/name: {{ .ClusterClaim.metadata.name }}
    spec:
      environment: client
      kubeconfig: true
      kubeconfigEndpoint: "https://{{ .InfraControlPlaneEndpoint.host }}:{{ .ClusterClaim.spec.client.network.kubeApiserverPort }}"
      issuerRef:
        apiVersion: cert-manager.io/v1
        kind: ClusterIssuer
        name: selfsigned
```

Оператор после рендера добавит `metadata.name: {claim}-client`, `metadata.namespace: {claim-ns}`.
Вычисляемое поле `.InfraControlPlaneEndpoint` доступно, т.к. этот шаг идёт после Cluster[infra] provisioned.

### 3.6. Go text/template — рендер YAML

Шаблоны используют стандартный Go `text/template` + все функции из библиотеки [Masterminds/sprig](https://masterminds.github.io/sprig/) + `toYaml`/`indent`. Рендер выполняется с опцией `missingkey=error` — обращение к несуществующему полю вызывает ошибку.

Доступен **весь** функционал Go-шаблонов: подстановки, условия (`if`/`else`), циклы (`range`), переменные (`$var := ...`), pipe-конвейеры, а также все sprig-функции (строки, math, кодирование, `default`, `required`, `hasPrefix`, `quote`, `b64enc`, и т.д.).

---

## 4. Граф выполнения pipeline

### 4.1. Полный граф (infra + client)

```
                          ┌─────────────────────┐
                          │     ClusterClaim     │
                          │   создан / изменён   │
                          └──────────┬──────────┘
                                     │
                          ┌──────────▼──────────┐
                   Step 1 │    Application      │  Рендер шаблона → Create/Update
                          │    (ArgoCD)         │
                          └──────────┬──────────┘
                                     │ сразу
                          ┌──────────▼──────────┐
                   Step 2 │  CertificateSet     │  Рендер шаблона → Create/Update
                          │    [infra]          │
                          └──────────┬──────────┘
                                     │
                          ┌──────────▼──────────┐
                   Step 3 │       WAIT          │  condition Ready=True
                          │ CertificateSet[i]   │  на CertificateSet[infra]
                          │     ready?          │
                          └──────────┬──────────┘
                                     │ Ready=True  (event от watch)
                          ┌──────────▼──────────┐
                   Step 4 │   Cluster[infra]    │  Рендер шаблона → Create/Update
                          └──────────┬──────────┘
                                     │
                          ┌──────────▼──────────┐
                   Step 5 │       WAIT          │  status.initialization
                          │ Cluster[infra]      │    .infrastructureProvisioned
                          │   provisioned?      │    == true
                          └──────────┬──────────┘
                                     │ provisioned  (event от watch)
                          ┌──────────▼──────────┐
                   Step 6 │  CertificateSet     │  Рендер с .InfraControlPlaneEndpoint
                          │    [client]         │  [skip если client.enabled=false]
                          └──────────┬──────────┘
                                     │
                          ┌──────────▼──────────┐
                   Step 7 │       WAIT          │  status.initialization
                          │ Cluster[infra]      │    .controlPlaneInitialized
                          │ CP initialized?     │    == true
                          └──────────┬──────────┘
                                     │ CP initialized  (event от watch)
                          ┌──────────▼──────────┐
                   Step 8 │      CcmCsrc        │  Рендер, client enabled=false
                          └──────────┬──────────┘
                                     │
                          ┌──────────▼──────────┐
                   Step 9 │ ConfigMap "parameters-     │  Рендер → Create/Update
                          │    infra" / "parameters-   │  в INFRA CLUSTER (remote)
                          │          client"          │  через Secret {name}-infra-kubeconfig
                          │                           │  [skip если configMapTemplateRef не указан]
                          └──────────┬──────────┘
                                     │ сразу
                          ┌──────────▼──────────┐
                  Step 10 │  Cluster[client]    │  Рендер с .InfraControlPlaneEndpoint,
                          │                     │  .InfraControlPlaneReplicas
                          │                     │  [skip если client.enabled=false]
                          └──────────┬──────────┘
                                     │
                          ┌──────────▼──────────┐
                  Step 11 │       WAIT          │  status.initialization
                          │ Cluster[client]     │    .controlPlaneInitialized
                          │ CP initialized?     │    == true
                          │                     │  [skip если client.enabled=false]
                          └──────────┬──────────┘
                                     │ CP initialized  (event от watch)
                          ┌──────────▼──────────┐
                  Step 12 │  CcmCsrc (повторно) │  Повторный рендер,
                          │                     │  client enabled=true
                          │                     │  [skip если client.enabled=false]
                          └──────────┬──────────┘
                                     │
                          ┌──────────▼──────────┐
                  Step 13 │     READY           │  Phase = Ready
                          └─────────────────────┘
```

### 4.2. Сокращённый граф (только infra, client.enabled=false)

```
Step 1:  Application           → сразу
Step 2:  CertificateSet[infra] → сразу
Step 3:  WAIT Ready            → event: CertificateSet condition Ready=True
Step 4:  Cluster[infra]        → сразу
Step 5:  WAIT provisioned      → event: Cluster status.initialization.infrastructureProvisioned
Step 7:  WAIT CP initialized   → event: Cluster status.initialization.controlPlaneInitialized
Step 8:  CcmCsrc               → сразу (client enabled=false)
Step 9:  ConfigMap (remote)    → сразу (если configMapTemplateRef указан)
Step 13: READY
```

Steps 6, 10–12 — полностью пропускаются.

---

## 5. Условия перехода между шагами (триггеры)

### 5.1. Таблица триггеров

| Step | Действие | Условие перехода к следующему | Как оператор узнаёт | Что делает если не готов |
|------|----------|------|------|------|
| 1 | Рендер `Application` | Create/Update успешно | Результат API-вызова | Ошибка → retry |
| 2 | Рендер `CertificateSet[infra]` | Create/Update успешно | Результат API-вызова | Ошибка → retry |
| **3** | **WAIT** | **`CertificateSet[infra]` condition `Ready=True`** | **Watch на CertificateSet (ownerRef)** | **Return, ждёт event** |
| 4 | Рендер `Cluster[infra]` | Create/Update успешно | Результат API-вызова | Ошибка → retry |
| **5** | **WAIT** | **`Cluster[infra].status.initialization.infrastructureProvisioned == true`** | **Watch на Cluster (ownerRef)** | **Return, ждёт event** |
| 6 | Рендер `CertificateSet[client]` | Create/Update успешно | Результат API-вызова | Ошибка → retry |
| **7** | **WAIT** | **`Cluster[infra].status.initialization.controlPlaneInitialized == true`** | **Watch на Cluster (ownerRef)** | **Return, ждёт event** |
| 8 | Рендер `CcmCsrc` | Create/Update успешно | Результат API-вызова | Ошибка → retry |
| 9 | Рендер `parameters-infra` + `parameters-client` → apply в infra cluster | Create/Update в infra cluster успешно | Результат API-вызова через remote client | **Requeue 30s** (cross-cluster) |
| 10 | Рендер `Cluster[client]` | Create/Update успешно | Результат API-вызова | Ошибка → retry |
| **11** | **WAIT** | **`Cluster[client].status.initialization.controlPlaneInitialized == true`** | **Watch на Cluster (ownerRef)** | **Return, ждёт event** |
| 12 | Повторный рендер `CcmCsrc` | Create/Update успешно | Результат API-вызова | Ошибка → retry |
| 13 | Финализация | — | — | Phase = Ready |

**Жирным** выделены шаги ожидания.

### 5.2. Reconcile: events + периодический интервал

Reconcile срабатывает по **двум** причинам:

1. **Event** — изменился зависимый ресурс (watch). Основной механизм продвижения pipeline.
2. **Периодический requeue** — каждый reconcile завершается с `RequeueAfter`, чтобы гарантировать консистентность.

```
Reconcile завершается всегда с RequeueAfter:

  WAIT шаг, условие не готово  → RequeueAfter = 5m   (страховка, основной триггер — watch)
  Все шаги ОК, Phase = Ready   → RequeueAfter = 10m  (drift detection)
  Ошибка remote (Step 9)       → RequeueAfter = 30s  (retry)
  Ошибка рендера               → RequeueAfter = 1m   (retry)
```

**Зачем периодический reconcile если есть watches:**
- **Drift detection** — проверка что созданные ресурсы не изменились извне (кто-то удалил/поменял)
- **Страховка от пропущенных events** — watch может пропустить event при перезапуске контроллера
- **Remote ресурсы** — ConfigMap в infra cluster не покрыты watches (cross-cluster)
- **Consistency check** — пересчёт status, conditions, mirror cluster statuses

**WAIT шаги** при этом работают так:

1. Проверить условие.
2. **Не готово** → обновить step status = `WaitingDependency`, return с `RequeueAfter = 5m`.
3. **Быстрый путь**: когда зависимый ресурс изменится — watch триггерит reconcile раньше, чем сработает таймер.
4. При следующем reconcile — проверить условие снова.

| Тип ожидания | Быстрый путь (event) | Страховка (periodic) |
|--------------|----------------------|----------------------|
| CertificateSet ready | Watch + ownerRef | RequeueAfter 5m |
| Cluster provisioned / CP init | Watch + ownerRef | RequeueAfter 5m |
| Secret (kubeconfig) | Watch + predicate | RequeueAfter 5m |
| ConfigMap в remote (Step 9) | Нет (cross-cluster) | RequeueAfter 30s |
| Phase = Ready | Watch на owned resources | RequeueAfter 10m |

### 5.3. Граф зависимостей «ресурс → что разблокирует»

```
CertificateSet[infra]
  │ condition Ready=True
  ▼
Cluster[infra] создаётся (Step 4)
  │
  ├─ status.initialization.infrastructureProvisioned == true
  │  ▼
  │  CertificateSet[client] получает InfraControlPlaneEndpoint (Step 6)
  │
  └─ status.initialization.controlPlaneInitialized == true
     ▼
     CcmCsrc создаётся (Step 8)
     ConfigMap[infra/client] создаётся в remote (Step 9)
     │
     ▼
     Cluster[client] создаётся (Step 10)
       │ status.initialization.controlPlaneInitialized == true
       ▼
       CcmCsrc обновляется, client enabled=true (Step 12)
         │
         ▼
         READY
```

### 5.4. Пример: ClusterClaim без client-кластера

```yaml
spec:
  client:
    enabled: false
```

Pipeline: Steps 1→2→3→4→5→7→8→9→13 (Ready).
Steps 6, 10–12 — полностью пропускаются.

### 5.5. Пример: ClusterClaim с system-ролью

```yaml
spec:
  infra:
    role: "system/infra"       # ← определяет system-режим
  extraEnvs:
    systemIstioGwVip: "10.200.0.1"
    systemVmInsertVIP: "10.200.0.2"
    # ... остальные переменные
```

System-режим определяется по `infra.role`: если роль начинается с `system` — ConfigMap'ы на Step 9 рендерятся с `systemEnabled: "true"` (через `{{ hasPrefix "system" .ClusterClaim.spec.infra.role }}`), и system-специфичные `extraEnvs` подставляются в `data`.

Pipeline идентичен базовому — отличие только в данных ConfigMap.

Полный пример: `docs/examples/clusterclaim-system.yaml`.

### 5.6. Пример: ClusterClaim без ConfigMap

```yaml
spec:
  # configMapTemplateRef не указан
```

Pipeline: Steps 1→2→3→4→5→6→7→8→10→11→12→13 (Ready).
Step 9 — пропускается.

---

## Примеры

Полные примеры ClusterClaim и всех шаблонов — в `docs/examples/`:

```
docs/examples/
├── clusterclaim.yaml                        # ClusterClaim (role: customer/infra)
├── clusterclaim-system.yaml                 # ClusterClaim (role: system/infra)
└── templates/
    ├── default-observe.yaml                 # Step 1:  Application (ArgoCD)
    ├── default-certset-infra.yaml           # Step 2:  CertificateSet[infra]
    ├── default-certset-client.yaml          # Step 6:  CertificateSet[client]
    ├── v1.34.4.yaml                         # Step 4:  Cluster[infra]
    ├── v1.35.2.yaml                         # Step 10: Cluster[client]
    ├── default-ccm.yaml                     # Step 8/12: CcmCsrc
    ├── default-cm-infra.yaml                # Step 9:  ConfigMap "parameters-infra" (remote)
    └── default-cm-client.yaml               # Step 9:  ConfigMap "parameters-client" (remote)
```
