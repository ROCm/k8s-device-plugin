This example show hwo to deploy the vLLM serve with ROCm/k8s-device-plugin.

# Setup the k8s cluster
You should setup the k8s cluster at first.

# Install the k8s-device-plugin
 The device plugin will enable registration of AMD GPU to a container cluster.

```
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-dp.yaml
kubectl create -f https://raw.githubusercontent.com/ROCm/k8s-device-plugin/master/k8s-ds-amdgpu-labeller.yaml
```

# Prepare the k8s yaml files

1. Secret is optional and only required for accessing gated models, you can skip this step if you are not using gated models.

    Here is the example `hf_token.yaml`

    ```
    apiVersion: v1
    kind: Secret
    metadata:
    name: hf-token-secret
    namespace: default
    type: Opaque
    data:
    token: "REPLACE_WITH_TOKEN"
    ```

    NOTE: you should use base64 to encode your HF TOKEN for the hf_token.yaml

    ```
    echo -n `<your HF TOKEN>` | base64
    ```

2. Define the deployment workload
    
    deployment.yaml

    ```
    apiVersion: apps/v1
    kind: Deployment
    metadata:
    name: mistral-7b
    namespace: default
    labels:
        app: mistral-7b
    spec:
    replicas: 1
    selector:
        matchLabels:
        app: mistral-7b
    template:
        metadata:
        labels:
            app: mistral-7b
        spec:
        volumes:
        - name: dev-kfd
            hostPath:
            path: /dev/kfd
        - name: dev-dri
            hostPath:
            path: /dev/dri
        # vLLM needs to access the host's shared memory for tensor parallel inference.
        - name: shm
            emptyDir:
            medium: Memory
            sizeLimit: "8Gi"
        hostNetwork: true
        hostIPC: true
        containers:
        - name: mistral-7b
            image: rocm/vllm:rocm6.2_mi300_ubuntu20.04_py3.9_vllm_0.6.4
            securityContext:
            seccompProfile:
                type: Unconfined
            capabilities:
                add:
                - SYS_PTRACE
            command: ["/bin/sh", "-c"]
            args: [
            "vllm serve mistralai/Mistral-7B-v0.3 --port 8000 --trust-remote-code --enable-chunked-prefill --max_num_batched_tokens 1024"
            ]
            env:
            - name: HUGGING_FACE_HUB_TOKEN
            valueFrom:
                secretKeyRef:
                name: hf-token-secret
                key: token
            ports:
            - containerPort: 8000
            resources:
            limits:
                cpu: "10"
                memory: 20G
                amd.com/gpu: "1"
            requests:
                cpu: "6"
                memory: 6G
                amd.com/gpu: "1"
            volumeMounts:
            - name: shm
            mountPath: /dev/shm
            - name: dev-kfd
            mountPath: /dev/kfd
            - name: dev-dri
            mountPath: /dev/dri
    ```   

3. Define the service.yaml

    ```
    apiVersion: v1
    kind: Service
    metadata:
    name: mistral-7b
    namespace: default
    spec:
    ports:
    - name: http-mistral-7b
        port: 80
        protocol: TCP
        targetPort: 8000
    # The label selector should match the deployment labels & it is useful for prefix caching feature
    selector:
        app: mistral-7b
    sessionAffinity: None
    type: ClusterIP
    ```


# Launch the pods

```
kubectl apply -f hf_token.yaml
kubectl apply -f deployment.yaml
kubectl apply -f service.yaml
```
    

# Test the service

Get the Service IP by 

```
kubectl get svc
```
The mistral-7b is the service name. We can access the vllm serve by the CLUSTER-IP and PORT of it like,

Get models by (please use the real CLUSTER-IP of your environment)

```
curl http://<CLUSTER-IP>:80/v1/models
```

Do request
```
curl http://<CLUSTER-IP>:80/v1/completions   -H "Content-Type: application/json"   -d '{
        "model": "mistralai/Mistral-7B-v0.3",
        "prompt": "San Francisco is a",
        "max_tokens": 7,
        "temperature": 0
      }'
```


