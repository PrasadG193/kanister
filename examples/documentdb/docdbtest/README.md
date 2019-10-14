# DocumentDB test application

This is a sample application that uses a DocumentDB database and consumes the DB configuration via a Kubernetes ConfigMap and Secret.

This allows the configuration to be changed easily based on environment the application is deployed in.

For example, the ConfigMap and Secret when using an AWS RDS instance are as follows
```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  annotations:
    kasten.io/config: dataservice
  name: dbconfig
data:
  docdb.instanceid: prasad-docdb-instance
  docdb.host: prasad-docdb-instance.test.us-west-2.rds.amazonaws.com
  docdb.user: docdb
  docdb.secret: dbcreds # name of K8s secret in the same namespace
---

apiVersion: v1
kind: Secret
metadata:
  name: dbcreds
type: Opaque
data:
  password: <BASE64 encoded password>
  ```

## Build/Package application
```bash
make clean
make container
make push
```

## Deployment into Kubernetes
```bash
# Set namespace to deploy into
export NAMESPACE=docdbtest
kubectl create namespace ${NAMESPACE}
kubectl apply -f deploy/. --namespace ${NAMESPACE}
```

## Testing the application
Use `kubectl proxy` to connect to the service in the cluster
```
kubectl proxy&
```
### Get Service and Database Information
```bash
http://127.0.0.1:8001/api/v1/namespaces/docdbtestrds/services/docdbtestapp:8080/proxy/
```

### Count rows
```bash
http://127.0.0.1:8001/api/v1/namespaces/docdbtestrds/services/docdbtestapp:8080/proxy/count
```

### Insert a new row
```bash
http://127.0.0.1:8001/api/v1/namespaces/docdbtestrds/services/docdbtestapp:8080/proxy/insert
```


