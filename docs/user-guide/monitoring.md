# Наблюдаемость

## Events

Оператор записывает Kubernetes Events для ключевых действий:

| Event | Type | Описание |
|-------|------|----------|
| `CreatedApplication` | Normal | Application (ArgoCD) создан |
| `CertificateSetInfraReady` | Normal | CertificateSet[infra] перешёл в Ready |
| `CreatedClusterInfra` | Normal | Cluster[infra] создан |
| `InfraProvisioned` | Normal | Инфраструктура Cluster[infra] создана |
| `InfraCPReady` | Normal | Control plane [infra] инициализирован |
| `CreatedCcmCsrc` | Normal | CcmCsrc создан |
| `AppliedRemoteConfigMaps` | Normal | Remote ConfigMaps применены |
| `CreatedClusterClient` | Normal | Cluster[client] создан |
| `ClientCPReady` | Normal | Control plane [client] инициализирован |
| `UpdatedCcmCsrc` | Normal | CcmCsrc обновлён с client info |
| `ClusterClaimReady` | Normal | Pipeline завершён |
| `StepFailed` | Warning | Ошибка на шаге pipeline |
| `DeletingResources` | Normal | Начато удаление managed ресурсов |
| `DeletionComplete` | Normal | Все managed ресурсы удалены |

### Просмотр Events

```bash
# Events для конкретного ClusterClaim
kubectl get events -n <ns> --field-selector involvedObject.name=<name> --sort-by='.lastTimestamp'

# Все events оператора
kubectl get events -A --field-selector reason=ClusterClaimReady
```

## Conditions

Conditions отражают текущее состояние каждого этапа pipeline:

```bash
kubectl get clusterclaim <name> -n <ns> -o jsonpath='{range .status.conditions[*]}{.type}: {.status} ({.reason}) - {.message}{"\n"}{end}'
```

Пример вывода для кластера в процессе создания:

```
ApplicationCreated: True (Created) - Application created successfully
InfraCertificateReady: True (Ready) - CertificateSet[infra] is Ready
InfraProvisioned: True (Provisioned) - Cluster[infra] infrastructure is provisioned
InfraCPReady: False (Waiting) - Waiting for Cluster[infra] control plane to be initialized
```

## Phase

Текущая фаза pipeline доступна через `status.phase`:

```bash
kubectl get clusterclaim -n <ns>
```

Вывод:

```
NAME     PHASE              AGE
ec8a00   Ready              2h
sys001   WaitingDependency  5m
test01   Failed             1m
```

## Мониторинг всех ClusterClaim'ов

```bash
# Обзор всех кластеров
kubectl get clusterclaim -A

# Только проблемные
kubectl get clusterclaim -A -o jsonpath='{range .items[?(@.status.phase!="Ready")]}{.metadata.namespace}/{.metadata.name}: {.status.phase}{"\n"}{end}'
```

## Логи контроллера

```bash
# Все логи
kubectl logs -n cluster-claim-operator-system deployment/cluster-claim-operator-controller-manager -f

# Только ошибки
kubectl logs -n cluster-claim-operator-system deployment/cluster-claim-operator-controller-manager | grep "ERROR"

# Логи для конкретного ClusterClaim
kubectl logs -n cluster-claim-operator-system deployment/cluster-claim-operator-controller-manager | grep "ec8a00"
```

## Проверка managed ресурсов

```bash
# Все ресурсы с labels оператора
kubectl get application,certificateset,cluster,ccmcsrc -n <ns> \
  -l clusterclaim.in-cloud.io/claim-name=<name>
```
