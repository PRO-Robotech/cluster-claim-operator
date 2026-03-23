# Руководство пользователя

## Содержание

| Раздел | Описание |
|--------|----------|
| [Установка](installation.md) | Установка оператора в management-кластер |
| [Создание шаблонов](creating-templates.md) | Написание ClusterClaimObserveResourceTemplate |
| [Развёртывание кластеров](deploying-clusters.md) | Создание ClusterClaim и управление ресурсами |
| [Pause / Resume](pause-resume.md) | Приостановка и возобновление reconcile |
| [Наблюдаемость](monitoring.md) | Events, conditions, мониторинг pipeline |

## Краткий справочник

### Частые команды

```bash
# Статус ClusterClaim
kubectl get clusterclaim -n <ns>
kubectl describe clusterclaim <name> -n <ns>

# Созданные ресурсы
kubectl get application <claim> -n <ns>
kubectl get certificateset -n <ns> -l clusterclaim.in-cloud.io/claim-name=<name>
kubectl get cluster -n <ns> -l clusterclaim.in-cloud.io/claim-name=<name>
kubectl get ccmcsrc <claim> -n <ns>

# Шаблоны (cluster-scoped)
kubectl get clusterclaimobserveresourcetemplates

# Pause/resume
kubectl annotate clusterclaim <name> -n <ns> clusterclaim.in-cloud.io/paused=true
kubectl annotate clusterclaim <name> -n <ns> clusterclaim.in-cloud.io/paused-

# Events
kubectl get events -n <ns> --field-selector involvedObject.name=<name> --sort-by='.lastTimestamp'
```

### Conditions

| Condition | True | False |
|-----------|------|-------|
| `Ready` | Pipeline завершён, кластер работает | Есть проблемы |
| `ApplicationCreated` | Application создан | — |
| `InfraCertificateReady` | CertificateSet[infra] Ready | Ожидание |
| `InfraProvisioned` | Инфраструктура создана | Ожидание provisioning |
| `InfraCPReady` | Control plane [infra] готов | Ожидание CP |
| `CcmCsrcCreated` | CcmCsrc создан | — |
| `RemoteConfigApplied` | Remote ConfigMaps применены | Ошибка apply |
| `ClientCPReady` | Control plane [client] готов | Ожидание client CP |
| `Paused` | Reconcile приостановлен | Reconcile активен |

### Фазы

| Фаза | Описание |
|------|----------|
| `Provisioning` | Pipeline выполняется, ресурсы создаются |
| `WaitingDependency` | Ожидание условия (WAIT-шаг) |
| `Ready` | Все шаги завершены |
| `Failed` | Ошибка рендеринга шаблона или API |
| `Degraded` | Невосстановимая ошибка |
| `Paused` | Reconcile приостановлен аннотацией |
| `Deleting` | ClusterClaim удаляется, ресурсы зачищаются |
