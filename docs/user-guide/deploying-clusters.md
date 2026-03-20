# Развёртывание кластеров

## Создание infra-only кластера

Минимальный ClusterClaim без client-кластера:

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaim
metadata:
  name: ec8a00
  namespace: dlputi1u
spec:
  observeTemplateRef:
    name: "default-observe"
  certificateSetTemplateRef:
    infra:
      name: "default-certset-infra"
  clusterTemplateRef:
    infra:
      name: "v1.34.4"
  ccmCsrTemplateRef:
    name: "default-ccm"

  replicas: 1
  configuration:
    cpuCount: 6
    diskSize: 51200
    memory: 12288

  infra:
    role: "customer/infra"
    paused: false
    network:
      serviceCidr: "10.96.0.0/12"
      podCidr: "10.244.0.0/16"
      kubeApiserverPort: 6443
      clusterDNS: "10.96.0.10"
    componentVersions:
      kubernetes: { version: "v1.34.4" }
      containerd: { version: "1.7.19" }
      runc: { version: "1.1.12" }
      etcd: { version: "3.5.12" }

  client:
    enabled: false
```

Pipeline: Steps 1→2→3→4→5→7→8→9→13 (steps 6, 10-12 пропускаются).

## Создание кластера infra + client

Добавьте `client.enabled: true` и client-шаблоны:

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaim
metadata:
  name: ec8a00
  namespace: dlputi1u
spec:
  observeTemplateRef:
    name: "default-observe"
  certificateSetTemplateRef:
    infra:
      name: "default-certset-infra"
    client:
      name: "default-certset-client"
  clusterTemplateRef:
    infra:
      name: "v1.34.4"
    client:
      name: "v1.35.2"
  ccmCsrTemplateRef:
    name: "default-ccm"
  configMapTemplateRef:
    infra:
      name: "default-cm-infra"
    client:
      name: "default-cm-client"

  replicas: 1
  configuration:
    cpuCount: 6
    diskSize: 51200
    memory: 12288

  infra:
    role: "customer/infra"
    paused: false
    network:
      serviceCidr: "10.155.0.0/16"
      podCidr: "10.156.0.0/16"
      podCidrMaskSize: 27
      clusterDNS: "10.155.0.10"
      kubeApiserverPort: 6443
    componentVersions:
      kubernetes: { version: "v1.34.4" }
      containerd: { version: "1.7.19" }
      runc: { version: "1.1.12" }
      etcd: { version: "3.5.12" }

  client:
    enabled: true
    paused: false
    network:
      serviceCidr: "10.155.0.0/16"
      podCidr: "10.156.0.0/16"
      podCidrMaskSize: 27
      clusterDNS: "10.155.0.10"
      kubeApiserverPort: 26443
    componentVersions:
      kubernetes: { version: "v1.35.2" }

  extraEnvs:
    beget_cluster_region: "ru1"
    beget_cluster_customer_login: "dlputi1u"
```

Pipeline: полный (Steps 1-13).

## Создание system-кластера

System-кластер: `role: system/infra`, больше реплик, без client:

```yaml
apiVersion: clusterclaim.in-cloud.io/v1alpha1
kind: ClusterClaim
metadata:
  name: sys001
  namespace: system-ns
spec:
  observeTemplateRef:
    name: "default-observe"
  certificateSetTemplateRef:
    infra:
      name: "default-certset-infra"
  clusterTemplateRef:
    infra:
      name: "v1.34.4"
  ccmCsrTemplateRef:
    name: "default-ccm"
  configMapTemplateRef:
    infra:
      name: "default-cm-infra"

  replicas: 3
  configuration:
    cpuCount: 8
    diskSize: 102400
    memory: 16384

  infra:
    role: "system/infra"
    paused: false
    network:
      serviceCidr: "10.96.0.0/12"
      podCidr: "10.244.0.0/16"
      kubeApiserverPort: 6443
      clusterDNS: "10.96.0.10"
    componentVersions:
      kubernetes: { version: "v1.34.4" }

  client:
    enabled: false
```

При `role: system/infra` оператор на Step 9 создаёт ConfigMap `parameters-system` в дополнение к `parameters-infra`.

## Override remote namespace

По умолчанию ConfigMap'ы применяются в namespace из флага `--remote-namespace`. Для per-claim override:

```yaml
spec:
  remoteNamespace: "custom-system"  # вместо глобального default
```

## Отслеживание прогресса

```bash
# Текущая фаза
kubectl get clusterclaim ec8a00 -n dlputi1u -o jsonpath='{.status.phase}'

# Conditions
kubectl get clusterclaim ec8a00 -n dlputi1u -o jsonpath='{range .status.conditions[*]}{.type}: {.status} ({.message}){"\n"}{end}'

# Events
kubectl get events -n dlputi1u --field-selector involvedObject.name=ec8a00 --sort-by='.lastTimestamp'
```

## Обновление кластера

Измените поля `ClusterClaim.spec` — оператор выполнит полный проход pipeline:

```bash
kubectl patch clusterclaim ec8a00 -n dlputi1u --type merge \
  -p '{"spec":{"configuration":{"cpuCount":8}}}'
```

На каждом шаге pipeline: render → сравнить → update если отличается. WAIT-шаги пропускаются если условие уже выполнено.

## Удаление кластера

```bash
kubectl delete clusterclaim ec8a00 -n dlputi1u
```

Оператор выполняет обратный pipeline:

1. Phase → `Deleting`
2. Delete remote ConfigMaps (ошибки игнорируются)
3. Delete CcmCsrc
4. Delete Cluster[client] → wait
5. Delete Cluster[infra] → wait
6. Delete CertificateSet'ы
7. Delete Application
8. Remove finalizer

**Важно:** `client.enabled` — immutable. Невозможно добавить или убрать client после создания. Для изменения — удалите и создайте заново.
