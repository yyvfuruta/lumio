# Kubernetes deployment

Run the following commands from the repository root.

The default deployment uses one replica, `Recreate`, and a 100 GiB ReadWriteOnce PVC. PostgreSQL is an external dependency: use an existing PostgreSQL service or operator. Lumio creates its initial tables and indexes at startup under a migration lock; the database role needs schema creation privileges.

```sh
kubectl apply -f deploy/k8s/base/namespace.yaml
# Put DATABASE_URL and LUMIO_PASSWORD in an untracked .env.k8s file.
kubectl -n lumio create secret generic lumio-secrets --from-env-file=.env.k8s
kubectl apply -k deploy/k8s/pvc
```

Set the image tag to an available release or a locally published image before applying. Configure the PVC size/storage class for your cluster. The container runs as UID/GID 65532 with a read-only root filesystem; the volume must permit this user to write. `/tmp` has a separate ephemeral volume.

For buckets, edit `deploy/k8s/bucket/kustomization.yaml` to set your storage URL and add provider credentials to `lumio-secrets`. GCS credentials files can be mounted from a Secret; set `GOOGLE_APPLICATION_CREDENTIALS` to their mount path. Workload identity may require a Kubernetes service account and token projection appropriate to your provider. The base disables automatic service-account token mounting.

```sh
kubectl apply -k deploy/k8s/bucket
```

The bucket overlay runs two replicas without a photo PVC. All replicas must share the same database, password, and storage configuration. PostgreSQL coordinates deduplication and deletion, and sessions work across replicas without sticky sessions. Each process handles one upload at a time to bound decode memory; increase throughput with more bucket-backed replicas. The deployment is prepared for scaling but has not been load-tested.

Adapt `deploy/k8s/ingress.example.yaml` to your ingress controller, hostname, and TLS setup. Preserve the browser's Host header, allow request bodies of at least the upload limit plus 1 MiB multipart overhead, and allow up to two minutes for an upload. For plain HTTP port-forwarding, temporarily set `LUMIO_SECURE_COOKIE=false`.

`/livez` reports that HTTP is running. `/readyz` checks PostgreSQL and the registered storage configuration; it does not probe bucket permissions on every request. Uploads detect bucket permission or availability failures. The authenticated status panel checks bucket accessibility on demand. Filesystem usage shows the entire mounted filesystem, including files outside Lumio; bucket capacity/quota is not exposed as filesystem capacity.
