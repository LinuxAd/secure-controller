# Secure Kubernetes Workload Manager

To deploy a new cluster for testing:
```bash
make cluster
```
To deploy this manager to the cluster:
```bash
make docker-build deploy
```
To deploy the test deployment to the default namespace:
```bash
kubectl apply -f ./config/deployment
```
Watch the logs for the manager pod:
```bash
kubectl logs -f $(kubectl get pods -n secure-controller-system --no-headers | cut -d ' ' -f 1) -n secure-controller-system
```
