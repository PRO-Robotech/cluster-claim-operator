# Создание шаблонов

Шаблоны `ClusterClaimObserveResourceTemplate` — cluster-scoped ресурсы, определяющие структуру создаваемых Kubernetes-ресурсов через Go text/template.

## Структура шаблона

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaimObserveResourceTemplate
metadata:
  name: <unique-name>       # Имя, на которое ссылается ClusterClaim
spec:
  apiVersion: <api-version>  # GVK создаваемого ресурса (immutable)
  kind: <kind>
  value: |                   # Go text/template → YAML-фрагмент
    metadata:
      labels: ...
      annotations: ...
    spec: ...
```

## Правила рендеринга

1. Шаблон рендерится в YAML-фрагмент (без `apiVersion`, `kind`, `metadata.name`, `metadata.namespace`)
2. Оператор добавляет `metadata.name`, `namespace`, `ownerReferences` и стандартные labels
3. Labels из шаблона мержатся с labels оператора (оператор имеет приоритет при конфликте ключей)
4. Если ресурс уже существует — сравнение и update при необходимости (merge-семантика для `spec`)

## Набор обязательных шаблонов

| TemplateRef в ClusterClaim | GVK | Минимальный набор |
|---|---|---|
| `observeTemplateRef` | `argoproj.io/v1alpha1/Application` | Да |
| `certificateSetTemplateRef.infra` | `in-cloud.io/v1alpha1/CertificateSet` | Да |
| `certificateSetTemplateRef.client` | `in-cloud.io/v1alpha1/CertificateSet` | Только при `client.enabled=true` |
| `clusterTemplateRef.infra` | `cluster.x-k8s.io/v1beta2/Cluster` | Да |
| `clusterTemplateRef.client` | `cluster.x-k8s.io/v1beta2/Cluster` | Только при `client.enabled=true` |
| `ccmCsrTemplateRef` | `controller.in-cloud.io/v1alpha1/CcmCsrc` | Да |
| `configMapTemplateRef.infra` | `v1/ConfigMap` | Нет (опционально) |
| `configMapTemplateRef.client` | `v1/ConfigMap` | Нет (опционально) |

## Template Context

Контекст зависит от шага pipeline, на котором рендерится шаблон:

```
Steps 1-4:  .ClusterClaim
Steps 5-6:  .ClusterClaim + .InfraControlPlaneEndpoint
Steps 7-8:  + .InfraControlPlaneInitialized + replica counts
Steps 9:    все infra-поля доступны
Steps 10-12: + .ClientControlPlaneInitialized + client endpoint
```

Подробности: [Template Context](../concepts/template.md#template-context).

## Примеры по типам ресурсов

### Application (ArgoCD)

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaimObserveResourceTemplate
metadata:
  name: default-observe
spec:
  apiVersion: argoproj.io/v1alpha1
  kind: Application
  value: |
    metadata:
      labels:
        xcluster.in-cloud.io/name: {{ .ClusterClaim.metadata.name }}
    spec:
      destination:
        name: in-cluster
        namespace: {{ .ClusterClaim.metadata.namespace }}
      project: common
      source:
        helm:
          releaseName: {{ .ClusterClaim.metadata.name }}
        path: .
        repoURL: https://gitlab.beget.ru/cloud/k8s/charts/cluster
        targetRevision: HEAD
      syncPolicy:
        automated:
          prune: true
          selfHeal: true
```

### CertificateSet

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

### CcmCsrc (с условной логикой)

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
              volumes:
                secret-ccm-kubeconfig:
                  volume:
                    secretName: {{ .ClusterClaim.metadata.name }}-infra-kubeconfig
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

### Remote ConfigMap

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
```

## Версионирование шаблонов

Рекомендуется использовать версию в имени шаблона для Cluster:

```
v1.34.4   → Cluster[infra] для Kubernetes 1.34.4
v1.35.2   → Cluster[client] для Kubernetes 1.35.2
```

Это позволяет создавать ClusterClaim'ы с разными версиями Kubernetes, ссылаясь на разные шаблоны.

## Обновление шаблонов

Изменение шаблона триггерит re-render **всех** ClusterClaim'ов, которые на него ссылаются. Используйте [pause](pause-resume.md) для поэтапного обновления.
