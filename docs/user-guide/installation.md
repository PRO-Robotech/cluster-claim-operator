# Установка

## Предварительные требования

- Kubernetes-кластер 1.31+ (management cluster)
- [ClusterAPI](https://cluster-api.sigs.k8s.io/) v1beta2
- [Argo CD](https://argo-cd.readthedocs.io/)
- [cert-manager](https://cert-manager.io/) (для CertificateSet)
- ccm-csr-controller (для CcmCsrc)
- secret-copy-operator (для копирования секретов в remote кластеры)

## Установка CRD

```bash
make install
```

Проверка:

```bash
kubectl get crd | grep clusterclaim.in-cloud.io
```

## Развёртывание контроллера

### Production

```bash
make deploy IMG=<your-registry>/cluster-claim-operator:latest
```

### Локальная разработка

```bash
make run -- --remote-namespace=beget-system
```

**Важно:** Флаг `--remote-namespace` обязателен. Он задаёт namespace в remote кластерах, куда применяются ConfigMap'ы (Step 9). Значение по умолчанию отсутствует — необходимо указать явно.

### Конфигурация контроллера

| Флаг | Обязательно | Описание |
|------|:-----------:|----------|
| `--remote-namespace` | Да | Namespace в remote кластерах для ConfigMap'ов |
| `--metrics-bind-address` | Нет | Адрес метрик (default: `0` — отключено) |
| `--health-probe-bind-address` | Нет | Адрес health probe (default: `:8081`) |
| `--leader-elect` | Нет | Включить leader election |
| `--enable-webhook` | Нет | Включить validating webhook (default: `true`) |

### Настройка Deployment

В `config/manager/manager.yaml` добавьте аргумент:

```yaml
spec:
  template:
    spec:
      containers:
      - name: manager
        args:
        - --remote-namespace=beget-system
        - --leader-elect
```

## Проверка установки

```bash
kubectl get pods -n cluster-claim-operator-system
kubectl get crd | grep clusterclaim.in-cloud.io
```

## Удаление

```bash
# Удалить контроллер
make undeploy

# Удалить CRD (удалит все ClusterClaim и шаблоны!)
make uninstall
```
