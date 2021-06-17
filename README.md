# K8S Old Pod Killer

-----
[![Docker Repository on Quay](https://quay.io/repository/alantang888/k8s-old-pod-killer/status "Docker Repository on Quay")](https://quay.io/repository/alantang888/k8s-old-pod-killer)

-----

This application can help to kill deployment/statefulset/daemonset pod, 
when pod age (don't care container age) older than config age, then this application will kill that.

# Config file

This application use config file to control how it runs and what to kill.

You can set env var `CONFIG_PATH` to indicate where is config file. Default it will read on `/config/config.yaml`. 
Config file **will not** hot reload. It only parse once when application start.

Config structure:

| Key              | Type          | Description                                                          |
|------------------|---------------|----------------------------------------------------------------------|
| dryrun           | bool          | If `true` will not delete pod                                        |
| batch_mode       | bool          | If `true` application will only run once then exit                   |
| default_interval | time.Duration | When TargetInfo's interval < 10s, will use this value. Min value 10s |
| targets          | []TargetInfo  | Target information, read TargetInfo structure for more information   |

TargetInfo structure:

| Key            | Type          |Description                                                                                                                     |
|----------------|---------------|--------------------------------------------------------------------------------------------------------------------------------|
| Kind           | string        | K8S kind. Only support `deployment`/`statefulset`/`daemonset`                                                                  |
| name_space     | string        | K8S namespace for kind                                                                                                         |
| name           | string        | K8S kind name                                                                                                                  |
| max_life       | time.Duration | When pod age older than this value, this application will try to kill it                                                       | 
| interval       | time.Duration | How often to run perform check on this target, Min value 10s. Lower than this or not set, will apply config's default_interval |
| batch_max_kill | int64         | How many pod can kill on single iteration. This won't override PodDisruptionBudget                                             |

There a example [config file](/example/example_config.yaml)

# K8S Permission
This application require below permission:
- Get Deployment
- Get StatefulSet
- Get DaemonSet
- List Pods
- Create pod eviction (This application use eviction to delete pod)

# Helm Chart
[Helm Chart](https://github.com/alantang888/helm-charts/tree/master/charts/k8s-old-pod-killer)
