# Restlink Restart Trigger Operator

Simple Kubernetes operator that **watches source pods** and, when
they crash or restart too often, **automatically restarts target pods**.

Since recreation is handled via pod deletion and further recreation of a pod by its owner (e.g. Deployment), certain pods won't be deleted. Specifically those that have the following ownerRefs:

- StatefulSets (since stateful application may be a bit too complex for by simple by-label deletion)
- Jobs (since job already handles lifecycle of a pod and external dependency may cause unexpected behaviour)
- Standalone pods with no onwerRef (since there is simply no one who can recreate standalone pod. However, in the future container-level restarts will be added)

---

## How it works

1. **Custom Resource** `RestartTrigger`    
   *define once – the operator does the rest*

2. Operator runs reconciliation loop every 30 s (and on relevant Pod events)  
   - Count restarts or creations of *source* pods  
   - If `minRestarts` happened within `restartWithinSeconds` **and** cooldown
     passed → delete *target* pods

3. Deleting a pod makes its Deployment/ReplicaSet/etc recreate it.

---

## CRD Spec

| Field | Purpose | Example |
|-------|---------|---------|
| `.spec.source.selector` | Pod label selector to watch | `component=metrics-collector` |
| `.spec.source.namespace` | Where to watch (default: CR’s ns) | `telemetry` |
| `.spec.source.containerName` | Which container inside the pod | `collector` |
| `.spec.source.minRestarts` | Threshold to trigger | `3` |
| `.spec.source.restartWithinSeconds` | Look-back window | `120` |
| `.spec.source.cooldownSeconds` | Don’t trigger again for X s | `300` |
| `.targets[]` | List of namespaces & selectors to restart | `role=fluentd` |

Examples can be found in [**config/examples**](https://github.com/pibblokto/restlink/tree/main/config/examples)
