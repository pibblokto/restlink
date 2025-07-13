package helpers

import (
	corev1 "k8s.io/api/core/v1"
)

func ShouldRestart(pod *corev1.Pod) bool {
	if len(pod.OwnerReferences) == 0 {
		return false
	}
	owner := pod.OwnerReferences[0]
	if !*owner.Controller {
		return false // Later may be removed if container-level restarts will be implemented (which will allow to restart standalone pods)
	}
	switch owner.Kind {
	case "ReplicaSet":
		return true // We assume that it's part of the deployment. We rarely create standalone ReplicaSets
	case "StatefulSet", "Job":
		return false
	default:
		return false
	}
}
