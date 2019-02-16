# Kubernetes Mutating Admission Webhook for doppler configuration injection

This webhook will automatically inject the configuration stored in [doppler](https://doppler.com) as environment variables on every pod. 

The webhook will mutate deployments if:
- They are not deployed on a system namespace
- Includes the `injector.doppler.com/inject: "yes"` annotation.

The deployment should provide the following annotations:
- `injector.doppler.com/environment: "prod"`
- `injector.doppler.com/pipeline: "100"`

If any of this is missing, the default value will be used. A sample of a deployment configured to use the injector [is available here](deployment/sample.yaml).


This is heavily based on https://github.com/morvencao/kube-mutating-webhook-tutorial

## Prerequisites

Kubernetes 1.9.0 or above with the `admissionregistration.k8s.io/v1beta1` API enabled. Verify that by the following command:
```
kubectl api-versions | grep admissionregistration.k8s.io/v1beta1
```
The result should be:
```
admissionregistration.k8s.io/v1beta1
```

In addition, the `MutatingAdmissionWebhook` and `ValidatingAdmissionWebhook` admission controllers should be added and listed in the correct order in the admission-control flag of kube-apiserver.

## Install

1. Create a signed cert/key pair and store it in a Kubernetes `secret` that will be consumed by sidecar deployment
```
./deployment/webhook-create-signed-cert.sh \
    --service doppler-injector-webhook-svc \
    --secret doppler-injector-webhook-certs \
    --namespace doppler-injector
```

2. Update the value of `caBundle` in `mutatingwebhook.yaml` with the value of your cluster's CA Bundle:
```
kubectl get configmap -n kube-system extension-apiserver-authentication -o=jsonpath='{.data.client-ca-file}' | base64 | tr -d '\n' | pbcopy
```

3. Update the value of `api` in `secret.yaml` with your Doppler API key in base64:
```
echo -n "$YOUR_KEY" | base64
```

Optionally, you can also update the pipeline and environment values, if you want to have a default value. If not, they'll default 0, and "dev".

4. Deploy the resources in the following order:
```
kubectl apply -f deployment/namespace.yaml
kubectl apply -f deployment/secret.yaml
kubectl apply -f deployment/deployment.yaml
kubectl apply -f deployment/service.yaml
kubectl apply -f deployment/mutatingwebhook.yaml
```

## Verify

1. The doppler inject webhook should be running
```
kubectl get pods --namespace=doppler-injector
NAME                                                   READY   STATUS    RESTARTS   AGE
doppler-injector-webhook-deployment-69fb7f8c79-stl2t   1/1     Running   0          3s
# kubectl get deployment --namespace=doppler-injector
NAME                                  DESIRED   CURRENT   UP-TO-DATE   AVAILABLE   AGE
doppler-injector-webhook-deployment   1         1         1            1           39s
```

2. Label the default namespace with `doppler-injector=enabled`
```
kubectl label namespace ramiro doppler-injector=enabled
kubectl get namespace -L doppler-injector
NAME          STATUS    AGE       DOPPLER-INJECTOR
default       Active    18h       enabled
kube-public   Active    18h
kube-system   Active    18h
```

3. Deploy the [sample deployment](deployment/sample.yaml)
```
kubectl apply -f deployment/sample.yaml
```

4. Verify that your doppler variables were injected:
```
kubectl get pods
NAME                         READY     STATUS        RESTARTS   AGE
webserver-65b4b5bc46-zksgk   1/1       Running       0          1m

kubectl get pods webserver-65b4b5bc46-zksgk -o jsonpath='{.spec.containers[0].env}'
```
