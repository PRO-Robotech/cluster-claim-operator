# Pause / Resume

## Два механизма паузы

ClusterClaim поддерживает два разных механизма паузы:

| Механизм | Что делает | Когда использовать |
|----------|-----------|-------------------|
| Аннотация `clusterclaim.in-cloud.io/paused: "true"` | Останавливает reconcile оператора | Batch-обновления, maintenance |
| `spec.infra.paused: true` | Передаётся как value в шаблоны (CAPI Cluster paused) | Пауза CAPI-кластера |

**Важно:** `spec.infra.paused` **не останавливает** оператор. Это только переменная для шаблонов.

## Приостановка reconcile (аннотация)

### Пауза

```bash
kubectl annotate clusterclaim <name> -n <ns> clusterclaim.in-cloud.io/paused=true
```

Результат:
- Phase → `Paused`
- Condition `Paused=True`
- Никаких requeue — оператор полностью игнорирует ClusterClaim

### Возобновление

```bash
kubectl annotate clusterclaim <name> -n <ns> clusterclaim.in-cloud.io/paused-
```

Результат:
- Следующий reconcile запустит pipeline с начала
- Phase вернётся к `Provisioning` / `Ready`

## Сценарии использования

### Batch-обновление

Нужно изменить несколько полей одновременно, запустив один проход pipeline:

```bash
# 1. Поставить на паузу
kubectl annotate clusterclaim ec8a00 -n dlputi1u clusterclaim.in-cloud.io/paused=true

# 2. Внести изменения
kubectl patch clusterclaim ec8a00 -n dlputi1u --type merge -p '{
  "spec": {
    "configuration": {"cpuCount": 8, "memory": 16384},
    "infra": {"componentVersions": {"kubernetes": {"version": "v1.35.0"}}}
  }
}'

# 3. Снять паузу — один проход pipeline
kubectl annotate clusterclaim ec8a00 -n dlputi1u clusterclaim.in-cloud.io/paused-
```

### Maintenance window

При обслуживании management-кластера:

```bash
# Поставить все ClusterClaim'ы на паузу
kubectl get clusterclaim -A -o name | xargs -I{} kubectl annotate {} clusterclaim.in-cloud.io/paused=true --overwrite

# После обслуживания — снять паузу
kubectl get clusterclaim -A -o name | xargs -I{} kubectl annotate {} clusterclaim.in-cloud.io/paused-
```

## Проверка статуса паузы

```bash
# Проверить аннотацию
kubectl get clusterclaim <name> -n <ns> -o jsonpath='{.metadata.annotations.clusterclaim\.in-cloud\.io/paused}'

# Проверить Phase
kubectl get clusterclaim <name> -n <ns> -o jsonpath='{.status.phase}'
```
