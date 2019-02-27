# Doppler sidecar for Kubernetes

This sidecar container will call the doppler API at the beginning of the deployment and create a dotfile with the available environment variables. 

# Usage

1. Add a shared volume to store the resulting dotfile with your api keys.
```yaml
volumes:
 - name: shared-data
   emptyDir: {}
```

2. Add the injector as an init container
```yaml
initContainers:
- image: doppler/injector:0.1.1
  name: doppler
  volumeMounts:
    - name: shared-data
    mountPath: /var/secret/doppler
  env:
    - name: DOPPLER_API_KEY
      value: "<API Key>"
    - name: DOPPLER_PIPELINE
      value: "<Pipeline ID>"
    - name: DOPPLER_ENVIRONMENT
      value: "<Environment Name>"
```

3. Add a read-only volume mount to each container
```yaml
volumeMounts:
    - name: shared-data
      mountPath: /var/secret/doppler
      readOnly: true
```

4. Update the `command` of every container to source `/var/secret/doppler/.env` on start
```yaml
command: 
    - sh
    - -c
    - source /var/secret/doppler/.env && yarn start
```

An example deployment manifest [is available here](./sample/deployment.yaml).
 