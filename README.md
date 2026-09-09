# widgets-apiserver: getting a real APIService running

This template ships with a working example (group `widgets.example.com`, kind
`Widget`). This guide takes you from "example on disk" to "real APIService
registered with your cluster's control plane".

## 1. Rename the example to your real API group/kind

Search-and-replace across the repo (case-sensitive), in this order:

| Find                  | Replace with              | Where it shows up |
|-----------------------|----------------------------|--------------------|
| `widgets.example.com` | your real API group, e.g. `apps.acme.io` | Go const `GroupName`, `deploy/kustomize/base/apiservice.yaml`, `rbac.yaml` |
| `Widget` / `widgets` / `widget` | your real Kind/plural/singular, e.g. `Order`/`orders`/`order` | Go types in `internal/apis/.../types.go`, `internal/domain/widget/`, `internal/adapters/...`, SQL in `db/migrations/` |
| `widgets-apiserver`   | `<yourservice>-apiserver`  | `deploy/kustomize/base/*.yaml` (Deployment/Service/ServiceAccount names) |
| `widgets-system`      | `<yourservice>-system`     | Namespace used throughout `deploy/kustomize/base/*.yaml` |

Practical tips:
- Do the Go rename first, then `go build ./...` - the compiler will point at
  every remaining reference.
- Rename the package `internal/domain/widget` and `internal/apis/widgets/v1alpha1`
  directories/packages too (not just the identifiers inside them) if you want
  the naming to be fully consistent.
- Keep one resource per API group per service for now; add more resources to
  the *same* group later by adding more entries to `apiserverpkg.Storage{...}`
  in `cmd/apiserver/main.go`.

## 2. Prerequisites

- A container registry you can push to.
- A Postgres instance reachable from the cluster.
- [cert-manager](https://cert-manager.io/docs/installation/) installed in the
  cluster (used to issue the TLS serving certificate the control plane needs
  to talk to your server, and to auto-inject the CA bundle into the
  `APIService` object). If you'd rather not depend on cert-manager, see
  "Alternative: manual TLS" below.

## 3. Database: run the migrations

Point any Postgres migration tool at `db/migrations/` (the SQL is plain,
numbered up/down files - no specific tool required). For example, with
[golang-migrate](https://github.com/golang-migrate/migrate):

```sh
migrate -path db/migrations -database "$POSTGRES_DSN" up
```

## 4. Build and push the image

```sh
docker build -t <registry>/<yourservice>-apiserver:v0.1.0 .
docker push <registry>/<yourservice>-apiserver:v0.1.0
```

Update `deploy/kustomize/base/deployment.yaml`'s `image:` field (or override
it with a kustomize image transformer in an overlay) to point at this tag.

## 5. Create the Postgres credentials secret

Not part of the kustomize base on purpose, so a real DSN never ends up in
version control:

```sh
kubectl create namespace widgets-system
kubectl -n widgets-system create secret generic widgets-postgres-credentials \
  --from-literal=dsn="postgres://user:password@host:5432/dbname?sslmode=require"
```

## 6. Apply the manifests

```sh
kubectl apply -k deploy/kustomize/base
```

This creates, in order of what matters conceptually:
1. `ServiceAccount` + RBAC (`rbac.yaml`) - lets your server delegate
   authn/authz decisions to the real API server.
2. `Certificate`/`Issuer` (`certificate.yaml`) - cert-manager issues a serving
   cert and a CA, and will auto-populate the `APIService`'s `caBundle`.
3. `Deployment` + `Service` - runs your server and exposes it in-cluster.
4. `APIService` (`apiservice.yaml`) - **this is the actual registration
   step**: it tells kube-apiserver "proxy `widgets.example.com/v1alpha1`
   requests to this Service". There is no separate "install" step beyond
   applying this one object.

## 7. Verify it's registered and working

```sh
kubectl get apiservice v1alpha1.widgets.example.com
# STATUS should be "True" (Available). If not:
kubectl describe apiservice v1alpha1.widgets.example.com
```

Common `Available: False` causes and fixes:
- **`caBundle` missing/empty** - cert-manager's cainjector isn't running, or
  the `cert-manager.io/inject-ca-from` annotation doesn't match the
  Certificate's `namespace/name`.
- **x509 errors** - the `Certificate`'s `dnsNames` must match
  `<service>.<namespace>.svc` exactly as kube-apiserver dials it.
- **`Service ... not found` / connection refused** - check the Deployment
  pods are `Running` (`kubectl -n widgets-system get pods`) and the container
  port (`6443`) matches `service.yaml`'s `targetPort`.
- **Pod crashlooping on startup** - almost always the `POSTGRES_DSN` secret;
  check `kubectl -n widgets-system logs deploy/widgets-apiserver`.

Once available, exercise it like any built-in resource:

```sh
kubectl apply -f - <<EOF
apiVersion: widgets.example.com/v1alpha1
kind: Widget
metadata:
  name: example
  namespace: default
spec:
  size: small
EOF

kubectl get widgets
kubectl get widget example -o yaml
kubectl delete widget example
```

## 8. Grant real users access

Aggregated resources use normal Kubernetes RBAC - nothing custom. Bind your
users/groups/service accounts to a `ClusterRole` scoped to the new group, e.g.
the example `widgets-admin` `ClusterRole` in `rbac.yaml`:

```sh
kubectl create clusterrolebinding widgets-admin-alice \
  --clusterrole=widgets-admin --user=alice@example.com
```

## Alternative: manual TLS (no cert-manager)

1. Generate a CA + serving cert for `<service>.<namespace>.svc` yourself
   (e.g. with `openssl` or `step`).
2. Create the secret manually instead of via `Certificate`:
   `kubectl -n widgets-system create secret tls widgets-apiserver-tls --cert=tls.crt --key=tls.key`.
3. Remove `certificate.yaml` from `kustomization.yaml`.
4. Remove the `cert-manager.io/inject-ca-from` annotation from
   `apiservice.yaml` and set `spec.caBundle` to your CA cert, base64-encoded:
   `kubectl get apiservice v1alpha1.widgets.example.com -o json | jq '.spec.caBundle="'"$(base64 -w0 ca.crt)"'"'`
   (or simplest: hand-edit the YAML with the base64 value before applying).

## Adding more resources or API groups

- **Same group, new resource**: add a new domain package (mirroring
  `internal/domain/widget`), a new REST adapter, register its type in the
  `v1alpha1` scheme, and add an entry to the `apiserverpkg.Storage{...}` map
  in `main.go`.
- **New, unrelated API group**: this template's convention is one service per
  API group - copy the whole repo as a new starting point rather than serving
  two groups from one binary, to keep ownership and scaling independent.
