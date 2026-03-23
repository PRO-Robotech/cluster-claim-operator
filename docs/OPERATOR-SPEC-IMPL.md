# ClusterClaim Operator: Полное описание реализации

> Цель документа: детальное описание каждого этапа работы оператора с привязкой к секциям ТЗ (OPERATOR-SPEC.md). Описывает что реализовано, как работает, и где реализация отличается от оригинального ТЗ.

---

## Содержание

1. [Обзор архитектуры](#1-обзор-архитектуры)
2. [CRD: ClusterClaimObserveResourceTemplate](#2-crd-clusterclaimobserveresourcetemplate)
3. [CRD: ClusterClaim](#3-crd-clusterclaim)
4. [Пайплайн рендеринга и шаблонизация](#4-пайплайн-рендеринга-и-шаблонизация)
5. [Pipeline: 13 шагов](#5-pipeline-13-шагов)
6. [WAIT-шаги и извлечение данных](#6-wait-шаги-и-извлечение-данных)
7. [Remote операции (Step 9)](#7-remote-операции-step-9)
8. [Reconcile: основной цикл](#8-reconcile-основной-цикл)
9. [Обновление (Update)](#9-обновление-update)
10. [Pause / Resume](#10-pause--resume)
11. [Удаление (Finalizer)](#11-удаление-finalizer)
12. [Watches: стратегия наблюдения](#12-watches-стратегия-наблюдения)
13. [Статус и Conditions](#13-статус-и-conditions)
14. [Events (наблюдаемость)](#14-events-наблюдаемость)
15. [Сводная таблица отклонений от ТЗ](#15-сводная-таблица-отклонений-от-тз)

---

## 1. Обзор архитектуры

### Что говорит ТЗ

Kubernetes-оператор, создающий по одному `ClusterClaim` полный набор зависимых ресурсов: ArgoCD Application, CertificateSet'ы, CAPI Cluster'ы, CcmCsrc и ConfigMap'ы в remote кластерах. Pipeline из 13 шагов с wait-условиями.

Два CRD:

| CRD | Scope | Роль |
|-----|-------|------|
| `ClusterClaimObserveResourceTemplate` | Cluster | Go-template для генерации любого ресурса |
| `ClusterClaim` | Namespaced | Ссылки на шаблоны + параметры кластера |

Все зависимости (Application, CertificateSet, Cluster, CcmCsrc) — через `unstructured.Unstructured`. Никаких typed imports внешних API groups.

### Что реализовано

Полностью совпадает с ТЗ. Два CRD в группе `clusterclaim.in-cloud.io/v1alpha1`. Все внешние ресурсы через unstructured-клиент.

**Отклонений нет.**

---

## 2. CRD: ClusterClaimObserveResourceTemplate

### Что говорит ТЗ

Cluster-scoped шаблон с тремя полями: `apiVersion`, `kind` (GVK создаваемого ресурса, immutable) и `value` (Go text/template → YAML). Один шаблон для одного типа ресурса.

Шаблон **не содержит** `metadata.name`, `metadata.namespace`, `metadata.ownerReferences` — оператор добавляет их при создании.

### Что реализовано

Три поля `spec.apiVersion`, `spec.kind`, `spec.value`. Metadata добавляется оператором при создании ресурса.

**Отклонений нет.**

---

## 3. CRD: ClusterClaim

### Что говорит ТЗ

Namespaced ресурс с:
- Ссылки на шаблоны (observeTemplateRef, certificateSetTemplateRef, clusterTemplateRef, ccmCsrTemplateRef, configMapTemplateRef)
- Параметры: replicas, configuration, extraEnvs
- Infra/Client спецификации с network и componentVersions
- `client.enabled` — immutable, задаёт тип pipeline

### 3.1. Spec — отклонения от ТЗ

| Поле | ТЗ | Реализация | Причина изменения |
|------|-----|------------|-------------------|
| `remoteNamespace` | Не в ТЗ | `string`, optional | Per-claim override namespace для remote ConfigMap'ов. Приоритет: spec > флаг `--remote-namespace` |
| `extraEnvs` | `map[string]любые` | `map[string]apiextensionsv1.JSON` | CRD schema не поддерживает `map[string]any` напрямую. `apiextensionsv1.JSON` хранит raw JSON bytes |
| `configuration` | Числовые поля | `int32` с validation (min: 1) | Единицы: cpuCount — ядра, diskSize/memory — MB |
| `componentVersions` | `map` | `map[string]ComponentVersion` | Обёртка `ComponentVersion { Version string }` для возможного расширения |

### 3.2. Status — отклонения от ТЗ

| Поле | ТЗ | Реализация | Причина изменения |
|------|-----|------------|-------------------|
| `phase` | Не определён в ТЗ | Определён по референсу WGC-operator: Provisioning, WaitingDependency, Ready, Failed, Degraded, Paused, Deleting | ТЗ не определял status schema |
| `observedGeneration` | Не упомянут | `int64` | Стандарт controller-runtime |
| `conditions` | Не определены в ТЗ | 9 condition types | ТЗ не определял status schema |

### 3.3. extraEnvs — ключевое решение

**ТЗ**: `extraEnvs` — произвольные переменные для шаблонов.

**Реализация**: `map[string]apiextensionsv1.JSON` с `x-kubernetes-preserve-unknown-fields: true`.

**Почему**: значения могут быть любого типа — string, number, boolean. Шаблоны обращаются к ним через `.ClusterClaim.spec.extraEnvs.<key>`.

---

## 4. Пайплайн рендеринга и шаблонизация

### Что говорит ТЗ

Каждый шаг pipeline: загрузить шаблон по templateRef → рендер Go text/template с TemplateContext → создать/обновить ресурс. Доступны Sprig-функции.

### Что реализовано

### 4.1. Процесс рендеринга

```
┌─────────────────┐     ┌──────────────────────┐     ┌─────────────────────┐
│  ClusterClaim   │     │ ClusterClaimObserve   │     │  Результат          │
│  (typed)        │     │ ResourceTemplate      │     │                     │
│                 │     │                        │     │                     │
│  Преобразование │     │  spec.apiVersion      │     │  apiVersion (шаблон)│
│  в Unstructured │     │  spec.kind            │     │  kind (шаблон)      │
│       │         │     │  spec.value: |        │     │  metadata (оператор)│
│       ▼         │     │    {{ .ClusterClaim   │     │  spec (рендер)      │
│  TemplateContext│────▶│      .metadata.name }}│────▶│  labels (merge)     │
│                 │     │    ...                │     │  ownerReferences    │
└─────────────────┘     └──────────────────────┘     └─────────────────────┘
```

Шаги:
1. Преобразовать typed `ClusterClaim` в `unstructured.Unstructured` (для доступа через `.ClusterClaim.spec.field.subfield`)
2. Построить `TemplateContext` (ClusterClaim + вычисляемые поля с предыдущих WAIT-шагов)
3. Загрузить `ClusterClaimObserveResourceTemplate` по имени из templateRef
4. Выполнить `template.Execute(spec.value, TemplateContext)` → rendered YAML
5. Распарсить rendered YAML → `map[string]interface{}`
6. Построить desired ресурс: GVK из шаблона + metadata от оператора + spec/data из рендера
7. Merge labels: rendered labels + standard labels (`claim-name`, `claim-namespace`)
8. Сравнить с existing → Create или Update

### 4.2. Template Context

| Поле | Тип | Доступно с шага | Описание |
|------|-----|:---------------:|----------|
| `.ClusterClaim` | Unstructured (map) | Все | Полный объект ClusterClaim |
| `.InfraControlPlaneEndpoint` | `{Host, Port}` | Step 5+ | Из `Cluster[infra].spec.controlPlaneEndpoint` |
| `.InfraControlPlaneInitialized` | `bool` | Step 7+ | Из `Cluster[infra].status.initialization.controlPlaneInitialized` |
| `.InfraControlPlaneAvailableReplicas` | `int32` | Step 7+ | Из `Cluster[infra].status.controlPlane.availableReplicas` |
| `.InfraControlPlaneDesiredReplicas` | `int32` | Step 7+ | Из `Cluster[infra].status.controlPlane.desiredReplicas` |
| `.ClientControlPlaneInitialized` | `bool` | Step 11+ | Из `Cluster[client].status.initialization.controlPlaneInitialized` |
| `.ClientControlPlaneEndpoint` | `{Host, Port}` | Step 11+ | Из `Cluster[client].spec.controlPlaneEndpoint` |
| `.ClientControlPlaneAvailableReplicas` | `int32` | Step 11+ | Из `Cluster[client].status.controlPlane.availableReplicas` |
| `.ClientControlPlaneDesiredReplicas` | `int32` | Step 11+ | Из `Cluster[client].status.controlPlane.desiredReplicas` |

**Совпадает с ТЗ.**

### 4.3. Template-функции

Стандартные Go template функции + библиотека Sprig. Шаблоны выполняются с `missingkey=error` — обращение к несуществующему полю вызывает ошибку рендера (а не `<no value>`).

**Совпадает с ТЗ.**

### 4.4. Update-семантика ресурсов (merge)

При update existing ресурса оператор использует **merge-семантику**: обновляются только ключи, присутствующие в rendered шаблоне. Поля, добавленные внешними контроллерами, сохраняются.

Это означает: шаблон определяет «минимальный desired state», а не полный. Это критично для ресурсов типа CcmCsrc, где Helm-operator добавляет поля в status и metadata.

**Совпадает с ТЗ.**

---

## 5. Pipeline: 13 шагов

### Что говорит ТЗ

```
Step 1:  Application           → Create/Update
Step 2:  CertificateSet[infra] → Create/Update
Step 3:  WAIT CertificateSet[infra] Ready=True
Step 4:  Cluster[infra]        → Create/Update
Step 5:  WAIT Cluster[infra] infrastructureProvisioned
Step 6:  CertificateSet[client]→ Create/Update        [skip если client.enabled=false]
Step 7:  WAIT Cluster[infra] controlPlaneInitialized
Step 8:  CcmCsrc               → Create/Update
Step 9:  ConfigMap (remote)    → Apply в infra cluster [skip если configMapTemplateRef нет]
Step 10: Cluster[client]       → Create/Update        [skip если client.enabled=false]
Step 11: WAIT Cluster[client] controlPlaneInitialized  [skip если client.enabled=false]
Step 12: CcmCsrc (update)     → Update client=true    [skip если client.enabled=false]
Step 13: READY
```

Pipeline без client: Steps 1→2→3→4→5→7→8→9→13.
Pipeline без ConfigMap: Step 9 пропускается.

### Что реализовано

Pipeline реализован как массив `pipelineStep`, каждый reconcile проходит все шаги с начала (implicit state — нет `status.currentStep`).

```
┌─ stepApplication              (Step 1)
├─ stepCertificateSetInfra      (Step 2)
├─ stepWaitCertSetReady         (Step 3 — WAIT)
├─ stepClusterInfra             (Step 4)
├─ stepWaitInfraProvisioned     (Step 5 — WAIT, extracts endpoint)
├─ stepCertificateSetClient     (Step 6 — skip if !client.enabled)
├─ stepWaitInfraCPReady         (Step 7 — WAIT, extracts replicas)
├─ stepCcmCsrc                  (Step 8)
├─ stepRemoteConfigMaps         (Step 9 — skip if no configMapTemplateRef)
├─ stepClusterClient            (Step 10 — skip if !client.enabled)
├─ stepWaitClientCPReady        (Step 11 — skip if !client.enabled, extracts client info)
└─ stepCcmCsrcUpdate            (Step 12 — skip if !client.enabled)

→ Все шаги прошли → Phase = Ready, requeue 10m
```

Каждый шаг возвращает `Proceed` (продолжить) или `Wait` (pipeline останавливается, requeue 5m).

**Совпадает с ТЗ.**

### 5.1. Маппинг создаваемых ресурсов

| Step | TemplateRef | GVK | metadata.name | metadata.namespace |
|------|-------------|-----|---------------|-------------------|
| 1 | `observeTemplateRef` | `argoproj.io/v1alpha1/Application` | `{claim}` | `{claim-ns}` |
| 2 | `certificateSetTemplateRef.infra` | `in-cloud.io/v1alpha1/CertificateSet` | `{claim}-infra` | `{claim-ns}` |
| 4 | `clusterTemplateRef.infra` | `cluster.x-k8s.io/v1beta2/Cluster` | `{claim}-infra` | `{claim-ns}` |
| 6 | `certificateSetTemplateRef.client` | `in-cloud.io/v1alpha1/CertificateSet` | `{claim}-client` | `{claim-ns}` |
| 8 | `ccmCsrTemplateRef` | `controller.in-cloud.io/v1alpha1/CcmCsrc` | `{claim}` | `{claim-ns}` |
| 10 | `clusterTemplateRef.client` | `cluster.x-k8s.io/v1beta2/Cluster` | `{claim}-client` | `{claim-ns}` |
| 12 | `ccmCsrTemplateRef` (повторно) | `controller.in-cloud.io/v1alpha1/CcmCsrc` | `{claim}` | `{claim-ns}` |

**Совпадает с ТЗ.**

### 5.2. Owner References

Все management cluster ресурсы получают `ownerReference` на `ClusterClaim` с `controller: true`, `blockOwnerDeletion: true`. Remote ConfigMap'ы (Step 9) — **без** ownerReferences (cross-cluster).

**Совпадает с ТЗ.**

---

## 6. WAIT-шаги и извлечение данных

### Что говорит ТЗ

WAIT-шаги проверяют status managed ресурсов через `unstructured.NestedBool`, `NestedString`. При невыполненном условии — requeue 5m (страховка, основной триггер — watch).

### Что реализовано

### 6.1. Step 3: Wait CertificateSet[infra] Ready

Проверяет condition `Ready=True` на CertificateSet через helper `isConditionTrue()`:
- Загрузить `CertificateSet` по имени `{claim}-infra`
- Пройти `status.conditions[]` → найти `type: Ready` → проверить `status: "True"`

**Совпадает с ТЗ.**

### 6.2. Step 5: Wait Infrastructure Provisioned

```
Cluster[infra].status.initialization.infrastructureProvisioned == true
```

При выполнении извлекает `controlPlaneEndpoint` (host + port) из `Cluster.spec.controlPlaneEndpoint` → заполняет `TemplateContext.InfraControlPlaneEndpoint`.

**Совпадает с ТЗ.**

### 6.3. Step 7: Wait Control Plane Initialized

```
Cluster[infra].status.initialization.controlPlaneInitialized == true
```

Извлекает из `Cluster.status.controlPlane`:
- `availableReplicas` → `InfraControlPlaneAvailableReplicas`
- `desiredReplicas` → `InfraControlPlaneDesiredReplicas`

Устанавливает `InfraControlPlaneInitialized = true`.

**Совпадает с ТЗ.**

### 6.4. Step 11: Wait Client CP Initialized

```
Cluster[client].status.initialization.controlPlaneInitialized == true
```

Извлекает client endpoint и replica counts аналогично Step 5 + Step 7.

**Совпадает с ТЗ.**

---

## 7. Remote операции (Step 9)

### Что говорит ТЗ

ConfigMap'ы применяются в infra cluster через kubeconfig из Secret `{claim}-infra-kubeconfig`. Три ConfigMap'а: `parameters-infra`, `parameters-system` (если role содержит "system"), `parameters-client` (если client enabled). Namespace — `beget-system`.

### Что реализовано

### 7.1. Remote client

Паттерн `ClusterManager` (по референсу secret-copy-operator):
- Парсит kubeconfig из Secret key `value`
- Кеширует REST client per-cluster (по UID Secret'а)
- Cleanup при Stop менеджера

### 7.2. Процесс Apply

```
1. Проверить configMapTemplateRef != nil (иначе skip)
2. Загрузить Secret {claim}-infra-kubeconfig
   └─ NotFound → Wait (requeue 5m)
3. Получить remote client через ClusterManager
4. Определить target namespace:
   └─ spec.remoteNamespace (если задан) > флаг --remote-namespace
5. Рендер и apply:
   a) configMapTemplateRef.infra → ConfigMap "parameters-infra"
   b) Если role содержит "system" → ConfigMap "parameters-system" (тот же шаблон)
   c) Если client.enabled && configMapTemplateRef.client → ConfigMap "parameters-client"
```

Каждый ConfigMap:
- Рендерится из шаблона с текущим TemplateContext
- Labels: merge rendered + standard (`claim-name`, `claim-namespace`)
- Create если не существует, Update если отличается (DeepEqual на Labels, Annotations, Data)

### 7.3. Отклонения от ТЗ

| Аспект | ТЗ | Реализация | Причина |
|--------|-----|------------|---------|
| Namespace | Хардкод `beget-system` | Конфигурируемый: `spec.remoteNamespace` > `--remote-namespace` (обязательный флаг) | Per-claim override для multi-tenant и mixed-environment сценариев |
| `parameters-system` | При `role: system/infra` | При `strings.Contains(role, "system")` | Более гибкое условие |

**Совпадает с ТЗ в остальном.**

---

## 8. Reconcile: основной цикл

### Что говорит ТЗ

Implicit state pipeline. Каждый reconcile проверяет все шаги с начала. Нет `status.currentStep`.

### Что реализовано

Полный порядок:

```
┌─ 1. Загрузить ClusterClaim (exit если NotFound)
├─ 2. Проверить DeletionTimestamp → reconcileDelete()
├─ 3. Проверить pause аннотацию → reconcilePaused()
├─ 4. Ensure finalizer (clusterclaim.in-cloud.io/finalizer)
├─ 5. executePipeline():
│   ├─ Phase → Provisioning
│   ├─ Конвертировать Claim в Unstructured
│   ├─ Построить начальный TemplateContext
│   ├─ Выполнить все 12 pipelineStep:
│   │   ├─ Step returns Proceed → следующий шаг
│   │   ├─ Step returns Wait → Phase=WaitingDependency, requeue 5m, STOP
│   │   └─ Step returns Error → Phase=Failed, requeue 1m, STOP
│   └─ Все шаги Proceed → Phase=Ready, requeue 10m
└─ 6. Обновить status (только при изменении)
```

### 8.1. Маппинг шагов ТЗ → реализация

| ТЗ | Реализация | Комментарий |
|----|------------|-------------|
| Implicit state pipeline | Массив `pipelineStep` в `executePipeline()` | Каждый reconcile — полный проход с начала |
| WAIT → requeue 5m | Step returns `Wait` → `RequeueAfter: 5m` | Страховка, основной триггер — watch |
| Error → Phase=Failed | Step returns error → `setFailed()`, `RequeueAfter: 1m` | Event `StepFailed` |
| Ready → requeue 10m | Все шаги Proceed → `setReady()`, `RequeueAfter: 10m` | Drift detection |

**Совпадает с ТЗ.**

---

## 9. Обновление (Update)

### Что говорит ТЗ

При изменении `ClusterClaim.spec` — полный проход pipeline (Step 1→13). На каждом шаге: render → сравнить с existing → update если отличается. WAIT-шаги пропускаются если условие уже выполнено.

### Что реализовано

Полностью совпадает. `ensureResource()` на каждом create/update шаге:
1. Загрузить шаблон
2. Рендер с текущим TemplateContext
3. Построить desired ресурс
4. GET existing
   - NotFound → Create
   - Exists → `resourceNeedsUpdate()` (сравнить labels, annotations, spec) → Update если отличается

WAIT-шаги: GET ресурс → проверить condition/status → если выполнено → `Proceed` (pipeline продолжается без ожидания).

**Совпадает с ТЗ.**

---

## 10. Pause / Resume

### Что говорит ТЗ

Два механизма:
- `spec.infra.paused` — только value для шаблонов (CAPI Cluster paused). Не останавливает оператор.
- Аннотация `clusterclaim.in-cloud.io/paused: "true"` — останавливает reconcile. Phase → Paused, no requeue.

### Что реализовано

**Pause (аннотация)**:
1. Detect: аннотация `clusterclaim.in-cloud.io/paused` == `"true"`
2. Phase → `Paused`
3. Condition: `Paused=True`
4. Без requeue — reconcile только при изменении объекта (watch event)

**Resume**:
1. При следующем reconcile: аннотация снята
2. Pipeline запускается с начала
3. Если за время паузы spec изменился → re-render → update ресурсов

**spec.infra.paused**: прозрачно передаётся в шаблоны через `.ClusterClaim.spec.infra.paused`. Оператор не интерпретирует это поле — оно влияет только на rendered CAPI Cluster.

**Совпадает с ТЗ.**

---

## 11. Удаление (Finalizer)

### Что говорит ТЗ

Обратный pipeline:
```
1. Delete remote ConfigMaps
2. Delete CcmCsrc
3. Delete Cluster[client] → wait
4. Delete Cluster[infra] → wait
5. Delete CertificateSet'ы
6. Delete Application
7. Remove finalizer
```

### Что реализовано

```
Step 1: Phase → Deleting

Step 2: Delete remote ConfigMaps
  └─ Через kubeconfig Secret (если доступен)
  └─ Ошибки логируются, но НЕ блокируют удаление
  └─ ConfigMap'ы: parameters-infra, parameters-system, parameters-client

Step 3: Delete CcmCsrc
  └─ NotFound → ok (уже удалён)

Step 4: Delete Cluster[client] (если client.enabled)
  ├─ Exists → Delete + wait
  │   └─ Requeue 5s до исчезновения
  └─ NotFound → продолжить

Step 5: Delete Cluster[infra]
  ├─ Exists → Delete + wait
  │   └─ Requeue 5s до исчезновения
  └─ NotFound → продолжить

Step 6: Delete CertificateSet[client] (если client.enabled)
Step 7: Delete CertificateSet[infra]
Step 8: Delete Application

Step 9: Remove finalizer
  └─ Event: DeletionComplete
```

**Совпадает с ТЗ.**

**Нюанс**: remote ConfigMaps удаляются **первыми** (Step 2) и ошибки игнорируются — infra cluster может быть недоступен в момент удаления. Это graceful degradation: оставшиеся ConfigMaps в remote кластере не влияют на management cluster.

**Нюанс**: Cluster'ы удаляются с ожиданием (`deleteAndWait`) — requeue каждые 5 секунд, пока ресурс не исчезнет. Это необходимо, потому что CAPI выполняет cascade delete (Cluster → Machine → Node → Infrastructure).

---

## 12. Watches: стратегия наблюдения

### Что говорит ТЗ

| Ресурс | Механизм | Что триггерит |
|---|---|---|
| `ClusterClaim` | Primary | Полный reconcile |
| `CertificateSet` | ownerRef | Step 3 |
| `Cluster` (CAPI) | ownerRef | Steps 5, 7, 11 |
| `CcmCsrc` | ownerRef | Drift detection |
| `Application` | ownerRef | Drift detection |
| `Secret` (kubeconfig) | predicate | Step 9 |
| `ClusterClaimObserveResourceTemplate` | name ref + indexer | Re-render при изменении шаблона |

### Что реализовано

| # | Ресурс | Тип | Реализация |
|---|--------|-----|------------|
| 1 | `ClusterClaim` | Primary (For) | Predicate: `GenerationChanged \|\| AnnotationChanged` |
| 2 | `Application` (unstructured) | Owns | Автоматический enqueue владельца |
| 3 | `CertificateSet` (unstructured) | Owns | Автоматический enqueue владельца |
| 4 | `Cluster` (unstructured) | Owns | Автоматический enqueue владельца |
| 5 | `CcmCsrc` (unstructured) | Owns | Автоматический enqueue владельца |
| 6 | `ClusterClaimObserveResourceTemplate` | Watch + EnqueueRequestsFromMapFunc | Fan-out: находит все Claims, ссылающиеся на шаблон |
| 7 | `Secret` (kubeconfig) | Watch + EnqueueRequestsFromMapFunc + predicate | Только Secrets с суффиксом `-infra-kubeconfig` |

**Fan-out для шаблонов**: при изменении `ClusterClaimObserveResourceTemplate` оператор через field indexer'ы находит все ClusterClaim'ы, ссылающиеся на этот шаблон, и enqueue каждый для reconcile.

**Field indexer'ы**: по всем templateRef полям (observeTemplateRef, certificateSetTemplateRef.infra, certificateSetTemplateRef.client, clusterTemplateRef.infra, clusterTemplateRef.client, ccmCsrTemplateRef, configMapTemplateRef.infra, configMapTemplateRef.client). Поддержка до 1000 ClusterClaim'ов.

**MaxConcurrentReconciles: 5** — ограничение параллелизма.

**Совпадает с ТЗ.**

---

## 13. Статус и Conditions

### Фазы (state machine)

```
                      ┌──────────────┐
        создание ────►│ Provisioning │──────────────────► Ready
                      └──────┬───────┘                     │
                             │                              │ spec changed
                             │ WAIT step                    │
                             ▼                              ▼
                      ┌──────────────────┐          Provisioning (re-enter)
                      │ WaitingDependency│──────────────────► Ready
                      └──────────────────┘   (condition met)

        render/API error ─────► Failed ──────────► Provisioning (после исправления, requeue 1m)

   Любая фаза + pause annotation → Paused → (resume) → pipeline с начала
   Любая фаза + DeletionTimestamp → Deleting → (cleanup) → удалён
```

### Conditions

| Condition | True | False |
|-----------|------|-------|
| `Ready` | Pipeline завершён | Ошибка или pipeline не завершён |
| `ApplicationCreated` | Step 1 пройден | — |
| `InfraCertificateReady` | Step 3 пройден | Ожидание CertificateSet Ready |
| `InfraProvisioned` | Step 5 пройден | Ожидание provisioning |
| `InfraCPReady` | Step 7 пройден | Ожидание CP initialization |
| `CcmCsrcCreated` | Step 8 пройден | — |
| `RemoteConfigApplied` | Step 9 пройден | Ошибка remote |
| `ClientCPReady` | Step 11 пройден | Ожидание client CP |
| `Paused` | Аннотация paused | Активен |

### Requeue стратегия

| Ситуация | RequeueAfter | Комментарий |
|----------|-------------|-------------|
| WAIT шаг, условие не готово | 5m | Страховка, основной триггер — watch |
| Phase = Ready | 10m | Drift detection |
| Ошибка шага pipeline | 1m | Retry (пользователь мог исправить шаблон) |
| Remote ошибка (Step 9) | — | Шаг возвращает error → 1m (общая обработка) |
| Deletion, Cluster ещё существует | 5s | Polling cascade delete |
| Paused | — | Без requeue, только watch event |

**Совпадает с ТЗ.**

---

## 14. Events (наблюдаемость)

### Что реализовано

| Event | Type | Reason | Когда |
|-------|------|--------|-------|
| Application создан | Normal | CreatedApplication | Step 1 |
| CertificateSet[infra] Ready | Normal | CertificateSetInfraReady | Step 3 |
| Cluster[infra] создан | Normal | CreatedClusterInfra | Step 4 |
| Infra provisioned | Normal | InfraProvisioned | Step 5 |
| Infra CP ready | Normal | InfraCPReady | Step 7 |
| CcmCsrc создан | Normal | CreatedCcmCsrc | Step 8 |
| Remote ConfigMaps applied | Normal | AppliedRemoteConfigMaps | Step 9 |
| Cluster[client] создан | Normal | CreatedClusterClient | Step 10 |
| Client CP ready | Normal | ClientCPReady | Step 11 |
| CcmCsrc updated | Normal | UpdatedCcmCsrc | Step 12 |
| Pipeline ready | Normal | ClusterClaimReady | Step 13 |
| Ошибка шага | Warning | StepFailed | Любой шаг |
| Начало удаления | Normal | DeletingResources | Deletion start |
| Удаление завершено | Normal | DeletionComplete | Finalizer removed |

---

## 15. Сводная таблица отклонений от ТЗ

### Дополнения (отсутствуют в ТЗ, добавлены при реализации)

| # | Что добавлено | Зачем |
|---|---------------|-------|
| 1 | `spec.remoteNamespace` | Per-claim override namespace для remote ConfigMap'ов |
| 2 | Флаг `--remote-namespace` (обязательный) | Глобальный default вместо хардкода `beget-system` |
| 3 | `--enable-webhook` флаг | Возможность запуска без cert-manager (для разработки) |

### Принятые решения (отличия от изначального ТЗ)

| # | Решение | Область | Комментарий |
|---|---------|---------|-------------|
| 1 | Status schema: Phase + Conditions определены по референсу WGC-operator | Status | ТЗ не определял status schema |
| 2 | Deletion: обратный pipeline с finalizer | Deletion | ТЗ не описывал deletion |
| 3 | Implicit state (нет currentStep) | Pipeline | Каждый reconcile — полный проход |
| 4 | Нет drift recovery для Cluster/CertificateSet | Pipeline | Только re-render шаблонов (update existing) |
| 5 | client.enabled — immutable | client spec | Webhook validation |
| 6 | Field indexer'ы для template watches | Watches | До 1000 ClusterClaim'ов |
| 7 | Всё через unstructured | Dependencies | Никаких typed imports внешних CRDs |
| 8 | Пауза: аннотация vs spec.infra.paused | Pause | Два разных механизма |
| 9 | Template validation — зона ответственности пользователя | Templates | Оператор не валидирует семантику |

### Принятые ограничения

| Ограничение | Статус | Комментарий |
|-------------|--------|-------------|
| Нет drift recovery | По решению | Pipeline не откатывается и не пересоздаёт удалённые ресурсы |
| Нет версионирования шаблонов | Не реализовано | Claim всегда использует текущую версию шаблона |
| Нет MutatingWebhook | Не реализовано | Defaults в коде оператора |

---

## Приложение A: Полный flow — от создания ClusterClaim до Ready

```
Пользователь:
  kubectl apply -f clusterclaim.yaml

┌─────────────────────────────────────────────────────────────────────┐
│ 1. API Server принимает ClusterClaim                                │
│    - CRD schema validation (OpenAPI v3)                             │
│    - CEL: client.enabled immutable (self == oldSelf)                │
│    - Enqueue в workqueue оператора                                  │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 2. Reconcile() начинается                                           │
│    Добавить finalizer clusterclaim.in-cloud.io/finalizer            │
│    Phase: → Provisioning                                            │
│    Конвертировать Claim в Unstructured                               │
│    Построить TemplateContext {ClusterClaim: unstructured}            │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 3. Step 1: Application                                              │
│    Загрузить шаблон "default-observe"                                │
│    Render: .ClusterClaim.metadata.name → "ec8a00"                   │
│    Создать Application ec8a00 в namespace dlputi1u                  │
│    Event: CreatedApplication                                        │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 4. Step 2: CertificateSet[infra]                                    │
│    Загрузить шаблон "default-certset-infra"                          │
│    Render: secret-copy annotations, issuerRef, role                  │
│    Создать CertificateSet ec8a00-infra                              │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 5. Step 3: WAIT CertificateSet Ready                                │
│    GET CertificateSet ec8a00-infra                                  │
│    Проверить condition Ready=True                                   │
│    ├─ True → Proceed                                                │
│    └─ False → Phase=WaitingDependency, requeue 5m, STOP            │
│       (Watch на CertificateSet ускорит re-reconcile)                │
└──────────────────────────────┬──────────────────────────────────────┘
                               │ (condition True)
┌──────────────────────────────▼──────────────────────────────────────┐
│ 6. Step 4: Cluster[infra]                                           │
│    Загрузить шаблон "v1.34.4"                                        │
│    Render: network config, componentVersions, extraEnvs              │
│    Создать Cluster ec8a00-infra                                     │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 7. Steps 5-7: WAIT Provisioned → WAIT CP Initialized                │
│    Извлечь controlPlaneEndpoint, replica counts                      │
│    Заполнить TemplateContext: InfraControlPlaneEndpoint, replicas    │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 8. Step 8: CcmCsrc                                                  │
│    Render с .ClientControlPlaneInitialized = false                   │
│    → ccmClient.enabled: false, csrcClient.enabled: false             │
│    Создать CcmCsrc ec8a00                                           │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 9. Step 9: Remote ConfigMaps (если configMapTemplateRef задан)       │
│    Загрузить Secret ec8a00-infra-kubeconfig                         │
│    Получить remote client                                           │
│    Namespace: spec.remoteNamespace || --remote-namespace             │
│    Apply: parameters-infra, parameters-system (если system role),    │
│           parameters-client (если client enabled)                    │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 10. Steps 10-12 (если client.enabled = true):                       │
│     Step 10: Cluster[client]                                         │
│     Step 11: WAIT Client CP Initialized                              │
│     Step 12: CcmCsrc update с .ClientControlPlaneInitialized = true  │
│              → ccmClient.enabled: true, csrcClient.enabled: true     │
│                                                                      │
│     (если client.enabled = false → skip, сразу Step 13)              │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 11. Step 13: READY                                                   │
│     Phase → Ready                                                    │
│     Condition Ready=True                                             │
│     Event: ClusterClaimReady                                         │
│     Requeue 10m (drift detection)                                    │
└─────────────────────────────────────────────────────────────────────┘
```

## Приложение B: Flow обновления spec

```
Пользователь:
  kubectl patch clusterclaim ec8a00 -n dlputi1u \
    --type merge -p '{"spec":{"configuration":{"cpuCount":8}}}'

┌─────────────────────────────────────────────────────────────────────┐
│ 1. Reconcile() запускается (generation changed)                      │
│    Phase: Ready → Provisioning (re-enter pipeline)                   │
│                                                                      │
│ 2. Step 1: Application                                               │
│    Render → сравнить → cpuCount не используется в Application → skip │
│                                                                      │
│ 3. Steps 2-3: CertificateSet → WAIT → уже Ready → Proceed          │
│                                                                      │
│ 4. Step 4: Cluster[infra]                                            │
│    Render с cpuCount=8 → rendered отличается от existing             │
│    → Update Cluster ec8a00-infra                                     │
│                                                                      │
│ 5. Steps 5-7: WAIT conditions → уже True → Proceed                  │
│                                                                      │
│ 6. Steps 8-12: Render → сравнить → без изменений → skip             │
│                                                                      │
│ 7. Phase → Ready, requeue 10m                                        │
│                                                                      │
│ Итого: обновлён только Cluster[infra] (где cpuCount используется),  │
│ остальные ресурсы без изменений                                      │
└─────────────────────────────────────────────────────────────────────┘
```

## Приложение C: Flow удаления ClusterClaim

```
Пользователь:
  kubectl delete clusterclaim ec8a00 -n dlputi1u

┌─────────────────────────────────────────────────────────────────────┐
│ 1. API Server устанавливает DeletionTimestamp                        │
│    Finalizer clusterclaim.in-cloud.io/finalizer блокирует удаление  │
│    Enqueue в workqueue                                               │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 2. Reconcile() → DeletionTimestamp != nil → reconcileDelete()        │
│    Phase → Deleting                                                  │
│    Event: DeletingResources                                          │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 3. Delete remote ConfigMaps                                          │
│    GET Secret ec8a00-infra-kubeconfig                                │
│    ├─ NotFound → skip (кластер уже недоступен)                       │
│    └─ Found → получить remote client                                 │
│        ├─ Ошибка подключения → log, skip (graceful degradation)      │
│        └─ OK → Delete:                                               │
│            - parameters-infra  (namespace: remoteNamespace)          │
│            - parameters-system (namespace: remoteNamespace)          │
│            - parameters-client (namespace: remoteNamespace)          │
│            Ошибки каждого Delete логируются, но НЕ блокируют         │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 4. Delete CcmCsrc ec8a00                                            │
│    ├─ NotFound → ok                                                  │
│    └─ Found → Delete                                                 │
│       (Helm-operator удалит CCM/CSR Deployments)                     │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 5. Delete Cluster[client] (если client.enabled = true)               │
│    GET Cluster ec8a00-client                                         │
│    ├─ NotFound → ok, продолжить                                      │
│    └─ Found:                                                         │
│        ├─ DeletionTimestamp == nil → Delete                           │
│        └─ DeletionTimestamp != nil → уже удаляется                   │
│        return false (ресурс ещё существует)                          │
│        → Requeue 5s                                                  │
│                                                                      │
│    (повторные reconcile каждые 5s пока Cluster не исчезнет)          │
│    (CAPI cascade: Cluster → MachineDeployment → Machine → Node)      │
│                                                                      │
│    GET → NotFound → ok, продолжить                                   │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 6. Delete Cluster[infra]                                             │
│    Аналогично Step 5:                                                │
│    GET Cluster ec8a00-infra                                          │
│    ├─ NotFound → ok, продолжить                                      │
│    └─ Found → Delete + Requeue 5s до исчезновения                    │
│                                                                      │
│    (CAPI cascade delete может занять минуты)                         │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 7. Delete CertificateSet[client] (если client.enabled = true)        │
│    Delete CertificateSet ec8a00-client                               │
│    ├─ NotFound → ok                                                  │
│    └─ Found → Delete                                                 │
│                                                                      │
│ 8. Delete CertificateSet[infra]                                      │
│    Delete CertificateSet ec8a00-infra                                │
│    ├─ NotFound → ok                                                  │
│    └─ Found → Delete                                                 │
│                                                                      │
│ 9. Delete Application ec8a00                                         │
│    ├─ NotFound → ok                                                  │
│    └─ Found → Delete                                                 │
└──────────────────────────────┬──────────────────────────────────────┘
                               │
┌──────────────────────────────▼──────────────────────────────────────┐
│ 10. Remove finalizer                                                 │
│     controllerutil.RemoveFinalizer()                                 │
│     Update ClusterClaim                                              │
│     Event: DeletionComplete                                          │
│     → API Server удаляет объект                                      │
└─────────────────────────────────────────────────────────────────────┘

Итого:
  - Remote ConfigMaps удаляются первыми (best-effort, ошибки не блокируют)
  - CcmCsrc удаляется до Cluster'ов (зависит от kubeconfig секретов)
  - Cluster'ы удаляются с ожиданием (CAPI cascade delete)
  - Cluster[client] удаляется ДО Cluster[infra] (client зависит от infra)
  - CertificateSet'ы и Application удаляются последними (не блокируют)
  - Весь процесс может занять минуты из-за CAPI cascade delete
```

## Приложение D: Flow обновления шаблона (fan-out)

```
Платформенный инженер:
  kubectl edit clusterclaimobserveresourcetemplate v1.34.4
  → Изменил spec.value (добавил новое поле в Cluster template)

┌─────────────────────────────────────────────────────────────────────┐
│ 1. Watch на ClusterClaimObserveResourceTemplate срабатывает          │
│    Оператор через field indexer находит все Claims с                 │
│    clusterTemplateRef.infra.name == "v1.34.4"                        │
│    Результат: [claim-A, claim-B, claim-C]                            │
│    → Enqueue все 3 в workqueue                                       │
│                                                                      │
│ 2. MaxConcurrentReconciles: 5 — все 3 обрабатываются параллельно    │
│                                                                      │
│ 3. Для каждого Claim:                                                │
│    - executePipeline() с обновлённым шаблоном                        │
│    - Step 4: Cluster[infra] → render → новый rendered ≠ existing     │
│    - → Update Cluster                                                │
│    - Остальные шаги → без изменений                                  │
│    - Phase → Ready                                                   │
│                                                                      │
│ Результат: все 3 Cluster'а обновлены параллельно                     │
└─────────────────────────────────────────────────────────────────────┘
```
