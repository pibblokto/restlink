package controller

import (
	"context"
	"time"

	restartv1alpha1 "github.com/pibblokto/restlink/api/v1alpha1"
	"github.com/pibblokto/restlink/internal/helpers"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

type RestartTriggerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=core.restlink.io,resources=restarttriggers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core.restlink.io,resources=restarttriggers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=core.restlink.io,resources=restarttriggers/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the RestartTrigger object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *RestartTriggerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	// Load RestartTrigger resource
	var trig restartv1alpha1.RestartTrigger
	if err := r.Get(ctx, req.NamespacedName, &trig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Build selectors
	srcSel, _ := metav1.LabelSelectorAsSelector(&trig.Spec.Source.Selector)
	srcNS := trig.Spec.Source.Namespace
	if srcNS == "" {
		srcNS = trig.Namespace
	}

	tgtSels := make([]labels.Selector, 2)
	tgtNss := make([]string, 2)
	for _, tgt := range trig.Spec.Targets {
		tgtSel, _ := metav1.LabelSelectorAsSelector(&tgt.Selector)
		tgtSels = append(tgtSels, tgtSel)
		if tgt.Namespace == "" {
			tgtNss = append(tgtNss, trig.Namespace)
		} else {
			tgtNss = append(tgtNss, tgt.Namespace)
		}
	}

	var srcPods corev1.PodList
	if err := r.List(ctx, &srcPods, client.InNamespace(srcNS), client.MatchingLabelsSelector{Selector: srcSel}); err != nil {
		return ctrl.Result{}, err
	}

	now := time.Now()
	since := now.Add(-time.Duration(trig.Spec.Source.RestartWithinSeconds) * time.Second)
	hits := 0

	for _, p := range srcPods.Items {
		if trig.Spec.Source.WatchPodCreation {
			if p.CreationTimestamp.Time.After(since) {
				hits++
				l.Info("hit via pod creation:", "pod name:", p.Name)
				continue
			}
		}

		for _, cs := range p.Status.ContainerStatuses {
			if cs.LastTerminationState.Terminated != nil &&
				cs.LastTerminationState.Terminated.FinishedAt.Time.After(since) &&
				cs.Name == trig.Spec.Source.ContainerName {
				hits++
				l.Info("hit via container restart:", "pod name:", p.Name, "container name:", trig.Spec.Source.ContainerName)
				break
			}
		}
	}

	shouldRestart := hits >= int(trig.Spec.Source.MinRestarts)
	cooldownOK := trig.Spec.Source.CooldownSeconds == 0 ||
		trig.Status.LastTriggered.IsZero() ||
		now.Sub(trig.Status.LastTriggered.Time) > time.Duration(trig.Spec.Source.CooldownSeconds)*time.Second

	var restarted []string
	if shouldRestart && cooldownOK {
		for i := 0; i < len(tgtSels); i++ {
			var tgtPods corev1.PodList
			if err := r.List(ctx, &tgtPods, client.InNamespace(tgtNss[i]), client.MatchingLabelsSelector{Selector: tgtSels[i]}); err != nil {
				return ctrl.Result{}, err
			}

			for _, p := range tgtPods.Items {
				if helpers.ShouldRestart(&p) {
					if err := r.Delete(ctx, &p, client.PropagationPolicy(metav1.DeletePropagationBackground)); err != nil && !errors.IsNotFound(err) {
						return ctrl.Result{}, err
					}
					restarted = append(restarted, p.Name)
				}
			}
			continue
		}

		trig.Status.LastTriggered = metav1.Time{Time: now}
		trig.Status.RestartCount++
		trig.Status.Phase = "Triggered"
		trig.Status.Triggered = true

		l.Info("pods restarted", "names", restarted, "trigger", trig.Name)
	} else {
		trig.Status.Phase = "Idle"
		trig.Status.Triggered = false
	}

	trig.Status.LastEvaluated = metav1.Time{Time: now}
	if err := r.Status().Update(ctx, &trig); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

// map Pod events --> matching RestartTriggers
func podToTriggersMapper(c client.Reader) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		pod := obj.(*corev1.Pod)

		var triggers restartv1alpha1.RestartTriggerList
		if err := c.List(ctx, &triggers); err != nil {
			utilruntime.HandleError(err)
			return nil
		}

		var reqs []reconcile.Request
		for _, t := range triggers.Items {
			effNS := t.Spec.Source.Namespace
			if effNS == "" {
				effNS = t.Namespace
			}
			if effNS != pod.Namespace {
				continue
			}

			sel, _ := metav1.LabelSelectorAsSelector(&t.Spec.Source.Selector)
			if sel.Matches(labels.Set(pod.Labels)) {
				reqs = append(reqs, reconcile.Request{
					NamespacedName: client.ObjectKeyFromObject(&t),
				})
			}
		}
		return reqs
	}
}

func (r *RestartTriggerReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&restartv1alpha1.RestartTrigger{}).
		Watches(&corev1.Pod{}, handler.EnqueueRequestsFromMapFunc(podToTriggersMapper(mgr.GetClient()))).
		Complete(r)
}
