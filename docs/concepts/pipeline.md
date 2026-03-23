# Pipeline

## Обзор

Pipeline — 13-шаговый конвейер создания ресурсов. Каждый reconcile проходит все шаги с начала. Нет сохранённого `status.currentStep` — состояние определяется неявно по наличию и status ресурсов (implicit state).

## Полный pipeline (infra + client)

```
Step 1:  Application           → Create/Update        (сразу)
Step 2:  CertificateSet[infra] → Create/Update        (сразу)
Step 3:  WAIT                  → CertificateSet[infra] condition Ready=True
Step 4:  Cluster[infra]        → Create/Update        (сразу)
Step 5:  WAIT                  → Cluster[infra] infrastructureProvisioned=true
Step 6:  CertificateSet[client]→ Create/Update
Step 7:  WAIT                  → Cluster[infra] controlPlaneInitialized=true
Step 8:  CcmCsrc               → Create/Update        (client enabled=false)
Step 9:  ConfigMap[infra/client]→ Apply в infra cluster
Step 10: Cluster[client]       → Create/Update
Step 11: WAIT                  → Cluster[client] controlPlaneInitialized=true
Step 12: CcmCsrc (повторно)   → Update, client=true
Step 13: READY
```

## Pipeline без client (client.enabled=false)

Steps 6, 10, 11, 12 пропускаются:

```
Step 1  → Step 2 → Step 3 (WAIT) → Step 4 → Step 5 (WAIT) →
Step 7 (WAIT) → Step 8 → Step 9 → Step 13 (READY)
```

## Pipeline без ConfigMap (configMapTemplateRef не указан)

Step 9 пропускается:

```
Step 1 → ... → Step 8 → Step 10 → ... → Step 13 (READY)
```

## Подробности шагов

### Step 1: Application (ArgoCD)

- Шаблон: `observeTemplateRef`
- Ресурс: `Application` с именем `{claim}`
- Контекст: только `.ClusterClaim`

### Step 2: CertificateSet[infra]

- Шаблон: `certificateSetTemplateRef.infra`
- Ресурс: `CertificateSet` с именем `{claim}-infra`
- Контекст: только `.ClusterClaim`

### Step 3: WAIT CertificateSet Ready

Ожидает condition `Ready=True` на CertificateSet[infra]. Без этого сертификаты не готовы для создания Cluster.

### Step 4: Cluster[infra]

- Шаблон: `clusterTemplateRef.infra`
- Ресурс: `Cluster` (CAPI) с именем `{claim}-infra`
- Контекст: `.ClusterClaim` + `.InfraControlPlaneEndpoint` (если доступен)

### Step 5: WAIT Infrastructure Provisioned

Ожидает `status.initialization.infrastructureProvisioned=true` на Cluster[infra]. Извлекает `controlPlaneEndpoint` (host + port) в TemplateContext.

### Step 6: CertificateSet[client]

- Шаблон: `certificateSetTemplateRef.client`
- Ресурс: `CertificateSet` с именем `{claim}-client`
- **Пропускается** при `client.enabled=false`

### Step 7: WAIT Control Plane Initialized

Ожидает `status.initialization.controlPlaneInitialized=true` на Cluster[infra]. Извлекает replica counts в TemplateContext.

### Step 8: CcmCsrc

- Шаблон: `ccmCsrTemplateRef`
- Ресурс: `CcmCsrc` с именем `{claim}`
- На этом шаге `ClientControlPlaneInitialized=false` → client-компоненты отключены

### Step 9: Remote ConfigMaps

- Шаблоны: `configMapTemplateRef.infra`, `configMapTemplateRef.client`
- Ресурсы: ConfigMap `parameters-infra`, `parameters-system` (если role содержит "system"), `parameters-client`
- Применяются в **infra cluster** через kubeconfig из Secret `{claim}-infra-kubeconfig`
- Namespace: из `spec.remoteNamespace` или флага `--remote-namespace`
- **Пропускается** если `configMapTemplateRef` не указан

### Step 10: Cluster[client]

- Шаблон: `clusterTemplateRef.client`
- Ресурс: `Cluster` (CAPI) с именем `{claim}-client`
- **Пропускается** при `client.enabled=false`

### Step 11: WAIT Client CP Initialized

Ожидает `status.initialization.controlPlaneInitialized=true` на Cluster[client]. Извлекает client endpoint и replica counts.

### Step 12: CcmCsrc (update)

Повторный рендер и update CcmCsrc. Теперь `ClientControlPlaneInitialized=true` → client-компоненты включены.

### Step 13: Ready

Все шаги завершены → Phase = `Ready`, requeue через 10 минут для drift detection.

## Requeue стратегия

| Ситуация | RequeueAfter |
|---|---|
| WAIT шаг, условие не готово | 5m (страховка, основной триггер — watch) |
| Phase = Ready | 10m (drift detection) |
| Ошибка remote (Step 9) | 30s |
| Ошибка рендера / API | 1m |

## Обновление (Update)

При изменении `ClusterClaim.spec` или шаблона — полный проход pipeline. На каждом шаге: render → сравнить с existing → update если отличается. WAIT-шаги пропускаются если условие уже выполнено.

## Удаление (Deletion)

При удалении ClusterClaim — обратный pipeline с финализатором:

```
Phase = Deleting
1. Delete remote ConfigMaps (через kubeconfig, ошибки игнорируются)
2. Delete CcmCsrc
3. Delete Cluster[client] → wait пока не удалён
4. Delete Cluster[infra] → wait пока не удалён
5. Delete CertificateSet'ы
6. Delete Application
7. Remove finalizer
```

## Watches

Pipeline продвигается не только через requeue, но и через watches:

| Ресурс | Механизм | Что триггерит |
|---|---|---|
| `ClusterClaim` | Primary | Полный reconcile |
| `CertificateSet` | ownerRef | Step 3 (Ready condition) |
| `Cluster` (CAPI) | ownerRef | Steps 5, 7, 11 |
| `CcmCsrc` | ownerRef | Drift detection |
| `Application` | ownerRef | Drift detection |
| `Secret` (kubeconfig) | predicate | Step 9 (remote access) |
| `ClusterClaimObserveResourceTemplate` | name ref | Re-render при изменении шаблона |

## Связанные ресурсы

- [ClusterClaim](clusterclaim.md) — входной ресурс pipeline
- [ClusterClaimObserveResourceTemplate](template.md) — шаблоны, рендерящиеся на каждом шаге
