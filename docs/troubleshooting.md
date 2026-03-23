# Устранение неполадок

Руководство по диагностике и решению типичных проблем с ClusterClaim Operator.

## Проверка статуса

### Статус ClusterClaim

```bash
kubectl get clusterclaim <name> -n <ns> -o yaml
```

Ключевые поля:
- `status.phase` — текущая фаза pipeline
- `status.conditions` — детальные conditions по каждому шагу

### Справочник по Conditions

| Condition | Status | Reason | Значение |
|-----------|--------|--------|----------|
| `Ready` | True | Ready | Pipeline завершён, кластер работает |
| `Ready` | False | StepFailed | Ошибка на шаге pipeline |
| `ApplicationCreated` | True | Created | Application создан |
| `InfraCertificateReady` | True | Ready | CertificateSet[infra] готов |
| `InfraCertificateReady` | False | Waiting | Ожидание Ready condition |
| `InfraProvisioned` | True | Provisioned | Инфраструктура создана |
| `InfraProvisioned` | False | Waiting | Ожидание provisioning |
| `InfraCPReady` | True | Ready | Control plane [infra] готов |
| `InfraCPReady` | False | Waiting | Ожидание CP initialization |
| `CcmCsrcCreated` | True | Created | CcmCsrc создан |
| `RemoteConfigApplied` | True | Applied | ConfigMaps применены |
| `RemoteConfigApplied` | False | RemoteConnectionError | Ошибка подключения к remote |
| `RemoteConfigApplied` | False | ApplyError | Ошибка apply ConfigMap |
| `ClientCPReady` | True | Ready | Control plane [client] готов |
| `ClientCPReady` | False | Waiting | Ожидание client CP |
| `Paused` | True | Paused | Reconcile приостановлен |

## Типичные проблемы

### 1. Phase: Failed, reason: StepFailed

**Симптом:**

```yaml
status:
  phase: Failed
  conditions:
    - type: Ready
      status: "False"
      reason: StepFailed
      message: "step Application: fetch template \"default-observe\": not found"
```

**Причина:** Шаблон `ClusterClaimObserveResourceTemplate`, указанный в templateRef, не существует.

**Решение:**

```bash
# Проверить существующие шаблоны
kubectl get clusterclaimobserveresourcetemplates

# Исправить ссылку или создать шаблон
kubectl apply -f templates/default-observe.yaml
```

### 2. Phase: Failed, ошибка рендеринга шаблона

**Симптом:**

```yaml
status:
  phase: Failed
  conditions:
    - type: Ready
      status: "False"
      reason: StepFailed
      message: "step ClusterInfra: render template \"v1.34.4\": template: ...: executing ... at <.ClusterClaim.spec.extraEnvs.missing_var>: map has no entry for key \"missing_var\""
```

**Причина:** Шаблон ссылается на переменную, которая не определена в ClusterClaim.

**Решение:**

1. Добавьте отсутствующую переменную в `spec.extraEnvs`:
   ```yaml
   spec:
     extraEnvs:
       missing_var: "value"
   ```

2. Или используйте `default` в шаблоне:
   ```
   {{ .ClusterClaim.spec.extraEnvs.missing_var | default "fallback" }}
   ```

### 3. Phase: WaitingDependency, InfraCertificateReady=False

**Симптом:**

```yaml
status:
  phase: WaitingDependency
  conditions:
    - type: InfraCertificateReady
      status: "False"
      reason: Waiting
      message: "Waiting for CertificateSet[infra] to become Ready"
```

**Причина:** CertificateSet[infra] не перешёл в Ready. Pipeline остановлен на Step 3.

**Решение:**

```bash
# Проверить статус CertificateSet
kubectl get certificateset <claim>-infra -n <ns> -o yaml

# Проверить cert-manager logs
kubectl logs -n cert-manager deployment/cert-manager -f

# Проверить ClusterIssuer
kubectl get clusterissuer
```

### 4. Phase: WaitingDependency, InfraProvisioned=False

**Симптом:**

```yaml
status:
  phase: WaitingDependency
  conditions:
    - type: InfraProvisioned
      status: "False"
      reason: Waiting
      message: "Waiting for Cluster[infra] infrastructure to be provisioned"
```

**Причина:** CAPI Cluster не завершил provisioning. Pipeline остановлен на Step 5.

**Решение:**

```bash
# Проверить статус CAPI Cluster
kubectl get cluster <claim>-infra -n <ns> -o yaml

# Проверить Machine'ы
kubectl get machines -n <ns> -l cluster.x-k8s.io/cluster-name=<claim>-infra

# Проверить infrastructure provider logs
kubectl logs -n capi-system deployment/capi-controller-manager
```

### 5. Phase: WaitingDependency, InfraCPReady=False

**Симптом:**

```yaml
status:
  phase: WaitingDependency
  conditions:
    - type: InfraCPReady
      status: "False"
      reason: Waiting
      message: "Waiting for Cluster[infra] control plane to be initialized"
```

**Причина:** Control plane Cluster[infra] не инициализирован. Pipeline остановлен на Step 7.

**Решение:**

```bash
# Проверить статус initialization
kubectl get cluster <claim>-infra -n <ns> \
  -o jsonpath='{.status.initialization}'

# Проверить control plane machines
kubectl get machines -n <ns> -l cluster.x-k8s.io/cluster-name=<claim>-infra \
  -l cluster.x-k8s.io/control-plane=true
```

### 6. RemoteConfigApplied=False, RemoteConnectionError

**Симптом:**

```yaml
status:
  conditions:
    - type: RemoteConfigApplied
      status: "False"
      reason: RemoteConnectionError
      message: "get remote client: ..."
```

**Причина:** Не удалось подключиться к infra cluster через kubeconfig Secret.

**Решение:**

```bash
# Проверить наличие kubeconfig Secret
kubectl get secret <claim>-infra-kubeconfig -n <ns>

# Проверить валидность kubeconfig
kubectl get secret <claim>-infra-kubeconfig -n <ns> \
  -o jsonpath='{.data.value}' | base64 -d > /tmp/kc.yaml
kubectl --kubeconfig=/tmp/kc.yaml cluster-info

# Проверить сетевую доступность infra-кластера
```

### 7. RemoteConfigApplied=False, ApplyError

**Симптом:**

```yaml
status:
  conditions:
    - type: RemoteConfigApplied
      status: "False"
      reason: ApplyError
      message: "apply parameters-infra: render template \"default-cm-infra\": ..."
```

**Причина:** Ошибка рендеринга шаблона ConfigMap или ошибка API при apply в remote cluster.

**Решение:**

```bash
# Проверить шаблон
kubectl get clusterclaimobserveresourcetemplate default-cm-infra -o yaml

# Проверить, что namespace существует в remote кластере
kubectl --kubeconfig=/tmp/kc.yaml get namespace beget-system
```

### 8. Невозможно изменить client.enabled

**Симптом:**

```
Error: admission webhook "vclusterclaim-v1alpha1.kb.io" denied the request:
spec.client.enabled: Invalid value: "object": client.enabled is immutable
```

**Причина:** Поле `client.enabled` является **immutable** после создания. Это защита от несогласованного состояния pipeline.

**Решение:**

Удалите ClusterClaim и создайте заново с нужным значением:

```bash
kubectl delete clusterclaim <name> -n <ns>
# Отредактируйте YAML с нужным client.enabled
kubectl apply -f clusterclaim.yaml
```

### 9. ClusterClaim долго не удаляется

**Симптом:** ClusterClaim в Phase `Deleting` длительное время.

**Причина:** Оператор ждёт удаления CAPI Cluster'ов (steps 3-4 deletion pipeline). CAPI удаляет Machine'ы и инфраструктуру, что может занять время.

**Диагностика:**

```bash
# Проверить, какие ресурсы ещё существуют
kubectl get cluster -n <ns> -l clusterclaim.in-cloud.io/claim-name=<name>

# Проверить статус удаления Cluster
kubectl get cluster <claim>-infra -n <ns> -o jsonpath='{.metadata.deletionTimestamp}'

# Логи контроллера
kubectl logs -n cluster-claim-operator-system deployment/cluster-claim-operator-controller-manager | grep <name>
```

**Решение:**

1. Подождите — CAPI удаляет инфраструктуру, это нормальный процесс
2. Если Cluster завис, проверьте CAPI logs
3. В крайнем случае, удалите финализатор Cluster вручную (потеря ресурсов!)

### 10. ClusterClaim не реагирует на изменения

**Симптом:** изменили spec, но Phase остаётся прежним.

**Проверить:**

```bash
# Не на паузе ли?
kubectl get clusterclaim <name> -n <ns> \
  -o jsonpath='{.metadata.annotations.clusterclaim\.in-cloud\.io/paused}'

# Если "true" — снять паузу
kubectl annotate clusterclaim <name> -n <ns> clusterclaim.in-cloud.io/paused-
```

### 11. Pipeline завершён, но ресурс отличается от ожидаемого

**Симптом:** Phase = Ready, но managed ресурс имеет неожиданные поля.

**Причина:** Оператор использует merge-семантику при update — он обновляет только ключи, присутствующие в rendered шаблоне. Поля, добавленные внешними контроллерами, сохраняются.

**Решение:** Проверьте шаблон — если нужно удалить поле, явно задайте его в шаблоне с пустым значением.

### 12. Шаблон обновлён, но ресурсы не изменились

**Симптом:** Изменили `ClusterClaimObserveResourceTemplate`, но managed ресурсы не обновились.

**Причина:** Watch на шаблоны триггерит reconcile через field indexer. Проверьте, что ClusterClaim ссылается на обновлённый шаблон.

**Решение:**

```bash
# Проверить, что шаблон обновлён
kubectl get clusterclaimobserveresourcetemplate <name> -o yaml

# Вручную триггернуть reconcile (добавить/удалить annotation)
kubectl annotate clusterclaim <name> -n <ns> trigger-reconcile=$(date +%s)
kubectl annotate clusterclaim <name> -n <ns> trigger-reconcile-
```

## Команды отладки

### Просмотр всех ClusterClaim

```bash
kubectl get clusterclaim -A -o wide
```

### Просмотр managed ресурсов

```bash
kubectl get application,certificateset,cluster,ccmcsrc -n <ns> \
  -l clusterclaim.in-cloud.io/claim-name=<name>
```

### Проверка шаблонов

```bash
kubectl get clusterclaimobserveresourcetemplates -o wide
```

### Проверка логов контроллера

```bash
kubectl logs -n cluster-claim-operator-system \
  deployment/cluster-claim-operator-controller-manager -f
```

### Проверка Events

```bash
kubectl get events -n <ns> --field-selector involvedObject.name=<name> --sort-by='.lastTimestamp'
```

## Настройка логирования

| Уровень | Флаг | Что логируется |
|---------|------|----------------|
| 0 (default) | `-v=0` | Важные события: создание/удаление, ошибки |
| 1 (debug) | `-v=1` | + каждый reconcile, промежуточные шаги |
| 2 (verbose) | `-v=2` | + детали рендеринга, полные объекты |

```yaml
# config/manager/manager.yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --zap-log-level=1  # debug mode
```

## Получение помощи

Если не удаётся решить проблему:

1. Соберите отладочную информацию:
   ```bash
   kubectl get clusterclaim <name> -n <ns> -o yaml > clusterclaim.yaml
   kubectl get events -n <ns> --field-selector involvedObject.name=<name> > events.txt
   kubectl logs -n cluster-claim-operator-system \
     deployment/cluster-claim-operator-controller-manager --tail=200 > logs.txt
   ```

2. Откройте issue: https://github.com/PRO-Robotech/cluster-claim-operator/issues
