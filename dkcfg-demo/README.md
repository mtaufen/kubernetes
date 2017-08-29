#Dynamic Config Demo that rolls out AppArmor

##Pre-demo
- `make quick-release`
- Run `setup.sh`, which will:
    + Call `kube-up.sh` to provision cluster with Dynamic Kubelet Config turned on and AppArmor turned off.
        * TODO: Do we need to configure an AppArmor profile or can we rely on the default being there?
        * TODO: Does kube-up apply the right credentials to kubectl?
    + Start `kubectl proxy` in the background.
    + Grab the current config from one node's `configz/` endpoint (which has whatever came from flags), and serialize it to a file called `kubelet`.
    + POST 1 pod per node, each of which specifies a `nodeSelector` and AppArmor annotation.
```
apiVersion: v1
kind: Pod
metadata:
    name: my-${hostname}-pod
    annotations:
        container.apparmor.security.beta.kubernetes.io/app-armor: runtime/default
spec:
    nodeSelector:
        kubernetes.io/hostname: ${hostname}
    containers:
      - name: app-armor
        image: busybox
        command: [ "sh", "-c", "sleep 1h" ]
```

##Live
- Run `watch -n1 showpods.sh` in one terminal to show that the pods are pending.
- Run `watch -n1 shownodes.sh` in another terminal to show which nodes have which config.
- Edit the `kubelet` file to turn AppArmor on.
- Run `kubectl create configmaps apparmor-enabled --from-file=kubelet` to create the new config's configmap.
- Run `rollout.sh apparmor-enabled` to roll out the `apparmor-enabled` configmap to each node, one-by-one (and wait a bit between each node). Observe that each Pod schedules once its Node has the new config.
- TODO: For fun, roll the change back
- TODO: do it with more nodes, say 10
