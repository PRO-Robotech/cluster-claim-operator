# Примеры

Готовые конфигурации для различных сценариев развёртывания кластеров.

## ClusterClaim

| Файл | Описание |
|------|----------|
| [clusterclaim.yaml](clusterclaim.yaml) | Customer infra + client кластер (полный pipeline) |
| [clusterclaim-system.yaml](clusterclaim-system.yaml) | System infra кластер (replicas=3, без client) |

## Шаблоны (ClusterClaimObserveResourceTemplate)

| Файл | GVK | Используется в |
|------|-----|---------------|
| [templates/default-observe.yaml](templates/default-observe.yaml) | Application (ArgoCD) | `observeTemplateRef` |
| [templates/default-certset-infra.yaml](templates/default-certset-infra.yaml) | CertificateSet | `certificateSetTemplateRef.infra` |
| [templates/default-certset-client.yaml](templates/default-certset-client.yaml) | CertificateSet | `certificateSetTemplateRef.client` |
| [templates/v1.34.4.yaml](templates/v1.34.4.yaml) | Cluster (CAPI) | `clusterTemplateRef.infra` |
| [templates/v1.35.2.yaml](templates/v1.35.2.yaml) | Cluster (CAPI) | `clusterTemplateRef.client` |
| [templates/default-ccm.yaml](templates/default-ccm.yaml) | CcmCsrc | `ccmCsrTemplateRef` |
| [templates/default-cm-infra.yaml](templates/default-cm-infra.yaml) | ConfigMap | `configMapTemplateRef.infra` |
| [templates/default-cm-client.yaml](templates/default-cm-client.yaml) | ConfigMap | `configMapTemplateRef.client` |

## CRD

| Файл | Описание |
|------|----------|
| [crd/ccmcsrcs.yaml](crd/ccmcsrcs.yaml) | CRD CcmCsrc (из ccm-csr-controller) |

## Сценарии

### Infra-only (без client)

Используйте `clusterclaim-system.yaml` как основу:
- `client.enabled: false`
- Не указывайте `certificateSetTemplateRef.client` и `clusterTemplateRef.client`
- Pipeline: Steps 1→2→3→4→5→7→8→9→13

### Infra + Client

Используйте `clusterclaim.yaml` как основу:
- `client.enabled: true`
- Обязательно указать client-шаблоны
- Pipeline: все 13 шагов

### Без Remote ConfigMaps

Не указывайте `configMapTemplateRef`:
- Step 9 пропускается
- Не требуется kubeconfig Secret для remote доступа на этом этапе
