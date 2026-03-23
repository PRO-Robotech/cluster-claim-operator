# API Reference

API Group: `clusterclaim.in-cloud.io/v1alpha1`

## ClusterClaim

**Scope:** Namespaced

Основной ресурс, описывающий кластер. Содержит ссылки на шаблоны и параметры инфраструктуры.

### Spec

| Поле | Тип | Обязательно | Default | Описание |
|------|-----|:-----------:|---------|----------|
| `observeTemplateRef.name` | `string` | Да | — | Имя шаблона для Application |
| `certificateSetTemplateRef.infra.name` | `string` | Да | — | Имя шаблона для CertificateSet[infra] |
| `certificateSetTemplateRef.client.name` | `string` | Нет | — | Имя шаблона для CertificateSet[client] |
| `clusterTemplateRef.infra.name` | `string` | Да | — | Имя шаблона для Cluster[infra] |
| `clusterTemplateRef.client.name` | `string` | Нет | — | Имя шаблона для Cluster[client] |
| `ccmCsrTemplateRef.name` | `string` | Да | — | Имя шаблона для CcmCsrc |
| `configMapTemplateRef.infra.name` | `string` | Нет | — | Имя шаблона для remote ConfigMap[infra] |
| `configMapTemplateRef.client.name` | `string` | Нет | — | Имя шаблона для remote ConfigMap[client] |
| `replicas` | `int32` | Да | — | Количество control plane реплик (min: 1) |
| `configuration` | `ConfigurationSpec` | Да | — | Compute-ресурсы |
| `remoteNamespace` | `string` | Нет | флаг `--remote-namespace` | Namespace в remote кластере для ConfigMap'ов |
| `extraEnvs` | `map[string]apiextensionsv1.JSON` | Нет | `{}` | Произвольные переменные для шаблонов |
| `infra` | `InfraSpec` | Да | — | Конфигурация infra-кластера |
| `client` | `ClientSpec` | Да | — | Конфигурация client-кластера |

### ConfigurationSpec

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `cpuCount` | `int32` | Да | Количество CPU (min: 1) |
| `diskSize` | `int32` | Да | Размер диска в MB (min: 1) |
| `memory` | `int32` | Да | Объём памяти в MB (min: 1) |

### InfraSpec

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `role` | `string` | Да | Роль: `customer/infra`, `system/infra` |
| `paused` | `bool` | Да | Value для шаблонов (CAPI paused) |
| `network` | `NetworkConfig` | Да | Сетевые настройки |
| `componentVersions` | `map[string]ComponentVersion` | Да | Версии компонентов |

### ClientSpec

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `enabled` | `bool` | Да | Включить client-кластер (**immutable**) |
| `paused` | `*bool` | Нет | Value для шаблонов |
| `network` | `*NetworkConfig` | Нет | Сетевые настройки |
| `componentVersions` | `map[string]ComponentVersion` | Нет | Версии компонентов |

### NetworkConfig

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `serviceCidr` | `string` | Да | CIDR для сервисов |
| `podCidr` | `string` | Да | CIDR для подов |
| `podCidrMaskSize` | `*int32` | Нет | Размер маски подсети |
| `clusterDNS` | `string` | Да | IP-адрес cluster DNS |
| `kubeApiserverPort` | `int32` | Да | Порт API server (1-65535) |

### ComponentVersion

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `version` | `string` | Да | Версия компонента (напр. `v1.34.4`) |

### Status

| Поле | Тип | Описание |
|------|-----|----------|
| `phase` | `string` | `Provisioning`, `WaitingDependency`, `Ready`, `Failed`, `Degraded`, `Paused`, `Deleting` |
| `observedGeneration` | `int64` | Последнее обработанное `metadata.generation` |
| `conditions` | `[]metav1.Condition` | Стандартные conditions |

### Conditions

| Type | Описание |
|------|----------|
| `Ready` | Pipeline завершён, кластер работает |
| `ApplicationCreated` | Application (ArgoCD) создан |
| `InfraCertificateReady` | CertificateSet[infra] в состоянии Ready |
| `InfraProvisioned` | Cluster[infra] инфраструктура создана |
| `InfraCPReady` | Cluster[infra] control plane инициализирован |
| `CcmCsrcCreated` | CcmCsrc создан |
| `RemoteConfigApplied` | Remote ConfigMaps применены |
| `ClientCPReady` | Cluster[client] control plane инициализирован |
| `Paused` | Reconciliation приостановлен |

### Annotations

| Annotation | Значение | Описание |
|-----------|---------|----------|
| `clusterclaim.in-cloud.io/paused` | `"true"` | Приостановить reconcile для этого ClusterClaim |

### Labels (на создаваемых ресурсах)

| Label | Значение | На каких ресурсах |
|-------|---------|-------------------|
| `clusterclaim.in-cloud.io/claim-name` | Имя ClusterClaim | Все managed ресурсы |
| `clusterclaim.in-cloud.io/claim-namespace` | Namespace ClusterClaim | Все managed ресурсы |

### Именование ресурсов

```
Application:            {claim}
CertificateSet[infra]:  {claim}-infra
CertificateSet[client]: {claim}-client
Cluster[infra]:         {claim}-infra
Cluster[client]:        {claim}-client
CcmCsrc:                {claim}
ConfigMap (remote):     parameters-infra, parameters-system, parameters-client
Kubeconfig Secret:      {claim}-infra-kubeconfig
```

### Finalizer

| Finalizer | Описание |
|-----------|----------|
| `clusterclaim.in-cloud.io/finalizer` | Обеспечивает удаление managed ресурсов перед удалением ClusterClaim |

---

## ClusterClaimObserveResourceTemplate

**Scope:** Cluster

Go-template для генерации Kubernetes-ресурсов.

### Spec

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `apiVersion` | `string` | Да | API version создаваемого ресурса (immutable) |
| `kind` | `string` | Да | Kind создаваемого ресурса (immutable) |
| `value` | `string` | Да | Go text/template, генерирующий YAML-фрагмент |

### Template Context

| Поле | Тип | Доступно с шага | Описание |
|------|-----|:---------------:|----------|
| `.ClusterClaim` | Unstructured | Все | Полный объект ClusterClaim |
| `.InfraControlPlaneEndpoint.Host` | `string` | Step 5+ | Хост CP endpoint infra |
| `.InfraControlPlaneEndpoint.Port` | `int64` | Step 5+ | Порт CP endpoint infra |
| `.InfraControlPlaneInitialized` | `bool` | Step 7+ | CP [infra] инициализирован |
| `.InfraControlPlaneAvailableReplicas` | `int32` | Step 7+ | Доступные реплики CP [infra] |
| `.InfraControlPlaneDesiredReplicas` | `int32` | Step 7+ | Желаемые реплики CP [infra] |
| `.ClientControlPlaneInitialized` | `bool` | Step 11+ | CP [client] инициализирован |
| `.ClientControlPlaneEndpoint.Host` | `string` | Step 11+ | Хост CP endpoint client |
| `.ClientControlPlaneEndpoint.Port` | `int64` | Step 11+ | Порт CP endpoint client |
| `.ClientControlPlaneAvailableReplicas` | `int32` | Step 11+ | Доступные реплики CP [client] |
| `.ClientControlPlaneDesiredReplicas` | `int32` | Step 11+ | Желаемые реплики CP [client] |

### Template-функции

Доступны стандартные Go template функции и [Sprig](http://masterminds.github.io/sprig/):

| Функция | Описание | Пример |
|---------|----------|--------|
| `default` | Значение по умолчанию | `{{ .ClusterClaim.spec.extraEnvs.region \| default "ru-1" }}` |
| `quote` | Обёртка в кавычки | `{{ .val \| quote }}` |
| `upper` / `lower` | Регистр строки | `{{ .val \| upper }}` |
| `contains` | Проверка подстроки | `{{ if contains "system" .val }}` |
| `ternary` | Условное значение | `{{ ternary "a" "b" .cond }}` |
| `toJson` | JSON-сериализация | `{{ .obj \| toJson }}` |
| `indent N` | Отступ N пробелов | `{{ .block \| indent 8 }}` |

### Пример

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

---

## Флаги контроллера

| Флаг | Обязательно | Default | Описание |
|------|:-----------:|---------|----------|
| `--remote-namespace` | Да | — | Namespace в remote кластерах для ConfigMap'ов |
| `--metrics-bind-address` | Нет | `0` | Адрес метрик |
| `--health-probe-bind-address` | Нет | `:8081` | Адрес health probe |
| `--leader-elect` | Нет | `false` | Leader election |
| `--enable-webhook` | Нет | `true` | Validating webhook |
| `--enable-http2` | Нет | `false` | HTTP/2 для metrics и webhook |
