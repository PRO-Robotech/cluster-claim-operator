# ClusterClaim

## Обзор

- Основной namespaced-ресурс, через который пользователь описывает кластер
- Содержит ссылки на шаблоны (`ClusterClaimObserveResourceTemplate`) и параметры кластера
- Оператор создаёт и оркестрирует полный набор зависимых ресурсов по 13-шаговому pipeline
- Поддерживает два режима: infra+client (`client.enabled=true`) и infra-only (`client.enabled=false`)

## Поля Spec

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `observeTemplateRef.name` | `string` | Да | Имя шаблона для Application (ArgoCD) |
| `certificateSetTemplateRef.infra.name` | `string` | Да | Имя шаблона для CertificateSet[infra] |
| `certificateSetTemplateRef.client.name` | `string` | Нет | Имя шаблона для CertificateSet[client] |
| `clusterTemplateRef.infra.name` | `string` | Да | Имя шаблона для Cluster[infra] |
| `clusterTemplateRef.client.name` | `string` | Нет | Имя шаблона для Cluster[client] |
| `ccmCsrTemplateRef.name` | `string` | Да | Имя шаблона для CcmCsrc |
| `configMapTemplateRef.infra.name` | `string` | Нет | Имя шаблона для remote ConfigMap[infra] |
| `configMapTemplateRef.client.name` | `string` | Нет | Имя шаблона для remote ConfigMap[client] |
| `replicas` | `int32` | Да | Количество control plane реплик (min: 1) |
| `configuration` | `ConfigurationSpec` | Да | Compute-ресурсы: cpuCount, diskSize, memory |
| `remoteNamespace` | `string` | Нет | Override namespace в remote кластере (default из флага `--remote-namespace`) |
| `extraEnvs` | `map[string]JSON` | Нет | Произвольные переменные для шаблонов |
| `infra` | `InfraSpec` | Да | Конфигурация infra-кластера |
| `client` | `ClientSpec` | Да | Конфигурация client-кластера |

### InfraSpec

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `role` | `string` | Да | Роль кластера: `customer/infra`, `system/infra` |
| `paused` | `bool` | Да | Передаётся в шаблон (CAPI Cluster paused) |
| `network` | `NetworkConfig` | Да | Сетевые настройки (CIDR, DNS, порт API) |
| `componentVersions` | `map[string]ComponentVersion` | Да | Версии компонентов (kubernetes, etcd, containerd и т.д.) |

### ClientSpec

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `enabled` | `bool` | Да | Включить client-кластер (**immutable** после создания) |
| `paused` | `*bool` | Нет | Передаётся в шаблон |
| `network` | `*NetworkConfig` | Нет | Сетевые настройки client-кластера |
| `componentVersions` | `map[string]ComponentVersion` | Нет | Версии компонентов client-кластера |

### NetworkConfig

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `serviceCidr` | `string` | Да | CIDR для сервисов |
| `podCidr` | `string` | Да | CIDR для подов |
| `podCidrMaskSize` | `*int32` | Нет | Размер маски подсети подов |
| `clusterDNS` | `string` | Да | IP-адрес cluster DNS |
| `kubeApiserverPort` | `int32` | Да | Порт API server (1-65535) |

## Создаваемые ресурсы

### Management cluster

| TemplateRef | Создаваемый ресурс | metadata.name | metadata.namespace |
|---|---|---|---|
| `observeTemplateRef` | Application | `{claim}` | `{claim-ns}` |
| `certificateSetTemplateRef.infra` | CertificateSet | `{claim}-infra` | `{claim-ns}` |
| `certificateSetTemplateRef.client` | CertificateSet | `{claim}-client` | `{claim-ns}` |
| `clusterTemplateRef.infra` | Cluster | `{claim}-infra` | `{claim-ns}` |
| `clusterTemplateRef.client` | Cluster | `{claim}-client` | `{claim-ns}` |
| `ccmCsrTemplateRef` | CcmCsrc | `{claim}` | `{claim-ns}` |

### Remote (infra cluster)

| TemplateRef | Создаваемый ресурс | metadata.name | metadata.namespace |
|---|---|---|---|
| `configMapTemplateRef.infra` | ConfigMap | `parameters-infra` | `{remoteNamespace}` |
| `configMapTemplateRef.infra` | ConfigMap | `parameters-system` | `{remoteNamespace}` |
| `configMapTemplateRef.client` | ConfigMap | `parameters-client` | `{remoteNamespace}` |

`{remoteNamespace}` — из `spec.remoteNamespace` или флага `--remote-namespace`.

## Status

| Поле | Описание |
|------|----------|
| `phase` | Текущая фаза: `Provisioning`, `WaitingDependency`, `Ready`, `Failed`, `Degraded`, `Paused`, `Deleting` |
| `observedGeneration` | Последнее обработанное `metadata.generation` |
| `conditions` | Стандартные conditions (см. ниже) |

### Conditions

| Condition | True | False |
|-----------|------|-------|
| `Ready` | Pipeline завершён | Есть проблемы |
| `ApplicationCreated` | Application создан | — |
| `InfraCertificateReady` | CertificateSet[infra] Ready | Ожидание Ready |
| `InfraProvisioned` | Cluster[infra] provisioned | Ожидание provisioning |
| `InfraCPReady` | Control plane [infra] инициализирован | Ожидание CP |
| `CcmCsrcCreated` | CcmCsrc создан | — |
| `RemoteConfigApplied` | Remote ConfigMaps применены | Ошибка apply |
| `ClientCPReady` | Control plane [client] инициализирован | Ожидание client CP |
| `Paused` | Reconcile приостановлен | — |

## Жизненный цикл фаз

```
                      ┌──────────────┐
        создание ────►│ Provisioning │
                      └──────┬───────┘
                             │
                ┌────────────┼────────────┐
                ▼            ▼            ▼
        ┌──────────────┐ ┌───────┐ ┌──────────────────┐
        │    Failed     │ │ Ready │ │ WaitingDependency │
        │ (ошибка)     │ │       │ │                    │
        └──────────────┘ └───┬───┘ └────────┬───────────┘
                             │              │ условие выполнено
                             │              └───────────────►Ready
                             │
                        10m requeue
                        (drift detection)
```

Аннотация `clusterclaim.in-cloud.io/paused: "true"` → Phase = `Paused`, no requeue.

Удаление → Phase = `Deleting` → обратный pipeline → finalizer removed.

## Пауза

Два механизма паузы:

| Механизм | Назначение |
|----------|-----------|
| `spec.infra.paused` | Только value для шаблонов (CAPI Cluster paused). **Не останавливает** оператор |
| Аннотация `clusterclaim.in-cloud.io/paused: "true"` | **Останавливает reconcile** оператора. Phase → Paused |

## Immutable-поля

- `client.enabled` — задаётся при создании, webhook отклоняет изменения

## Лучшие практики

1. **Один ClusterClaim = один кластер.** Не объединяйте разные кластеры в один Claim
2. **Используйте `extraEnvs`** для произвольных переменных шаблонов, а не кастомные поля
3. **Batch-обновления через pause.** Поставьте ClusterClaim на паузу, внесите изменения, снимите паузу — один проход pipeline вместо нескольких
4. **Проверяйте шаблоны перед применением.** Оператор не валидирует семантику — ошибки рендера → Phase = Failed

## Связанные ресурсы

- [ClusterClaimObserveResourceTemplate](template.md) — шаблоны ресурсов
- [Pipeline](pipeline.md) — как работает 13-шаговый конвейер
