# ClusterClaimObserveResourceTemplate

## Обзор

- Cluster-scoped ресурс, определяющий шаблон для генерации Kubernetes-ресурсов
- Один шаблон используется для одного типа ресурса (определяется полями `apiVersion` + `kind`)
- Поле `value` содержит Go text/template, который рендерится с контекстом `TemplateContext`
- Результат рендера — YAML-фрагмент с `metadata`, `spec` и/или `data`

## Spec

| Поле | Тип | Обязательно | Описание |
|------|-----|:-----------:|----------|
| `apiVersion` | `string` | Да | API version создаваемого ресурса (immutable) |
| `kind` | `string` | Да | Kind создаваемого ресурса (immutable) |
| `value` | `string` | Да | Go text/template, генерирующий YAML-фрагмент |

## Что шаблон определяет

Шаблон (`value`) содержит YAML с полями создаваемого ресурса:

- `metadata.labels` — будут merged с стандартными labels оператора
- `metadata.annotations` — будут скопированы в создаваемый ресурс
- `spec` — полная спецификация ресурса
- `data` — для ConfigMap'ов

## Что оператор добавляет (не содержится в шаблоне)

| Поле | Источник |
|------|----------|
| `metadata.name` | По naming convention (см. [ClusterClaim](clusterclaim.md)) |
| `metadata.namespace` | Из ClusterClaim |
| `metadata.ownerReferences` | На ClusterClaim (controller=true) |
| Label `clusterclaim.in-cloud.io/claim-name` | Из ClusterClaim.name |
| Label `clusterclaim.in-cloud.io/claim-namespace` | Из ClusterClaim.namespace |

## Template Context

При рендеринге шаблон получает объект `TemplateContext`:

| Поле | Тип | Доступно с шага | Описание |
|------|-----|:---------------:|----------|
| `.ClusterClaim` | Unstructured | Все шаги | Полный объект ClusterClaim |
| `.InfraControlPlaneEndpoint.Host` | `string` | Step 5+ | Хост control plane endpoint infra |
| `.InfraControlPlaneEndpoint.Port` | `int64` | Step 5+ | Порт control plane endpoint infra |
| `.InfraControlPlaneInitialized` | `bool` | Step 7+ | CP [infra] инициализирован |
| `.InfraControlPlaneAvailableReplicas` | `int32` | Step 7+ | Доступные реплики CP [infra] |
| `.InfraControlPlaneDesiredReplicas` | `int32` | Step 7+ | Желаемые реплики CP [infra] |
| `.ClientControlPlaneInitialized` | `bool` | Step 11+ | CP [client] инициализирован |

### Доступ к полям ClusterClaim

`ClusterClaim` передаётся как unstructured-объект, поэтому доступ к полям — через точечную нотацию:

```
{{ .ClusterClaim.metadata.name }}
{{ .ClusterClaim.metadata.namespace }}
{{ .ClusterClaim.spec.infra.role }}
{{ .ClusterClaim.spec.infra.network.serviceCidr }}
{{ .ClusterClaim.spec.configuration.cpuCount }}
{{ .ClusterClaim.spec.extraEnvs.beget_cluster_region }}
{{ .ClusterClaim.spec.remoteNamespace }}
```

### Template-функции

Доступны стандартные Go template функции и библиотека [Sprig](http://masterminds.github.io/sprig/):

| Функция | Описание | Пример |
|---------|----------|--------|
| `default` | Значение по умолчанию | `{{ .ClusterClaim.spec.extraEnvs.region \| default "ru-1" }}` |
| `quote` | Оборачивание в кавычки | `{{ .ClusterClaim.spec.infra.role \| quote }}` |
| `upper` / `lower` | Регистр | `{{ .ClusterClaim.metadata.name \| upper }}` |
| `contains` | Проверка подстроки | `{{ if contains "system" .ClusterClaim.spec.infra.role }}` |
| `ternary` | Условное значение | `{{ ternary "true" "false" .InfraControlPlaneInitialized }}` |
| `toJson` | JSON-сериализация | `{{ .ClusterClaim.spec.extraEnvs \| toJson }}` |

## Пример: CertificateSet[infra]

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
      annotations:
        secret-copy.in-cloud.io/dstClusterKubeconfig: {{ .ClusterClaim.metadata.namespace }}/{{ .ClusterClaim.metadata.name }}-infra-kubeconfig
        secret-copy.in-cloud.io/dstNamespace: beget-system
      labels:
        cluster.x-k8s.io/cluster-name: {{ .ClusterClaim.metadata.name }}-infra
    spec:
      environment: {{ .ClusterClaim.spec.infra.role }}
      issuerRef:
        kind: ClusterIssuer
        name: selfsigned
```

Оператор создаст ресурс:

```yaml
apiVersion: in-cloud.io/v1alpha1
kind: CertificateSet
metadata:
  name: ec8a00-infra                           # ← оператор
  namespace: dlputi1u                           # ← оператор
  ownerReferences:                              # ← оператор
    - apiVersion: clusterclaim.in-cloud.io/v1alpha1
      kind: ClusterClaim
      name: ec8a00
      controller: true
  labels:
    cluster.x-k8s.io/cluster-name: ec8a00-infra  # ← шаблон
    clusterclaim.in-cloud.io/claim-name: ec8a00   # ← оператор (merge)
    clusterclaim.in-cloud.io/claim-namespace: dlputi1u  # ← оператор (merge)
  annotations:
    secret-copy.in-cloud.io/dstClusterKubeconfig: dlputi1u/ec8a00-infra-kubeconfig  # ← шаблон
    secret-copy.in-cloud.io/dstNamespace: beget-system                               # ← шаблон
spec:
  environment: customer/infra                   # ← шаблон
  issuerRef:
    kind: ClusterIssuer
    name: selfsigned
```

## Пример: CcmCsrc с условной логикой

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaimObserveResourceTemplate
metadata:
  name: default-ccm
spec:
  apiVersion: controller.in-cloud.io/v1alpha1
  kind: CcmCsrc
  value: |
    spec:
      beget-ccm:
        appSpec:
          applications:
            ccmInfra:
              enabled: true
              containers:
                manager:
                  extraEnv:
                    CLUSTER_NAME: {{ .ClusterClaim.metadata.name }}-infra
            ccmClient:
              enabled: {{ .ClientControlPlaneInitialized }}
      beget-csrc:
        appSpec:
          applications:
            csrcInfra:
              enabled: true
            csrcClient:
              enabled: {{ .ClientControlPlaneInitialized }}
```

На Step 8 `ClientControlPlaneInitialized = false` → client компоненты выключены.
На Step 12 `ClientControlPlaneInitialized = true` → client компоненты включены.

## Пример: Remote ConfigMap

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaimObserveResourceTemplate
metadata:
  name: default-cm-infra
spec:
  apiVersion: v1
  kind: ConfigMap
  value: |
    data:
      KUBERNETES_VERSION: {{ .ClusterClaim.spec.infra.componentVersions.kubernetes.version }}
      SERVICE_CIDR: {{ .ClusterClaim.spec.infra.network.serviceCidr }}
      POD_CIDR: {{ .ClusterClaim.spec.infra.network.podCidr }}
      CLUSTER_DNS: {{ .ClusterClaim.spec.infra.network.clusterDNS }}
```

Remote ConfigMap'ы создаются в infra cluster (не в management cluster) — без ownerReferences.

## Watches

Изменение шаблона триггерит re-render всех ClusterClaim'ов, которые на него ссылаются. Оператор использует field indexer'ы по всем templateRef-полям для эффективного поиска.

## Лучшие практики

1. **Используйте версионирование шаблонов** — имена вида `v1.34.4` для шаблонов Cluster
2. **Один шаблон = один GVK.** Не пытайтесь создать несколько типов ресурсов из одного шаблона
3. **Проверяйте доступность полей контекста.** На Step 1-4 вычисляемые поля (InfraControlPlaneEndpoint и т.д.) ещё не заполнены
4. **Используйте `default`** для optional полей: `{{ .ClusterClaim.spec.extraEnvs.region | default "ru-1" }}`

## Связанные ресурсы

- [ClusterClaim](clusterclaim.md) — основной ресурс
- [Pipeline](pipeline.md) — в каком порядке шаблоны рендерятся
