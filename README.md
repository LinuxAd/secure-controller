# Secure Kubernetes Workload Manager

Developed and tested on a mac running Mac OSX 12.1 Monterey.

## Requirements

* Docker (tested on 20.10.11 docker desktop)
* Kubectl (tested on 1.22)
* Kind or minikube (tested on kind v0.11.1)
* GNU Make (tested on 3.81)
* bash needs to be installed

All other requirements will be downloaded automatically if you use the makefile

## Running the project

To deploy a new cluster for testing, run the tests, build a docker image and deploy it to the cluster:
```bash
make cluster docker-build deploy
```
To deploy the test deployment to the default namespace:
```bash
kubectl apply -f ./config/testfiles
```
Watch the logs for the manager pod:
```bash
kubectl logs -f $(kubectl get pods -n secure-controller-system --no-headers | cut -d ' ' -f 1) -n secure-controller-system
```
Cleanup:
```bash
make clean
```