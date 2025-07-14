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
	"k8s.io/apimachinery/pkg/types"
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
// +kubebuilder:rbac:groups=core,resources=secrets,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch;delete
func (r *RestartTriggerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)

	var trig restartv1alpha1.RestartTrigger
	if err := r.Get(ctx, req.NamespacedName, &trig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

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

	if trig.Spec.SlackNotification != nil {
		webhookSecret := types.NamespacedName{
			Namespace: trig.Namespace,
			Name:      trig.Spec.SlackNotification.WebhookSecret.Name,
		}
		var secret corev1.Secret
		if err := r.Get(ctx, webhookSecret, &secret); err != nil {
			l.Error(err, "unable to load Slack webhook secret")
			return ctrl.Result{}, err
		}
		webhook := string(secret.Data["webhook"])

		var srcInfos []helpers.PodInfo
		for _, p := range srcPods.Items {
			cause := "N/A"
			if trig.Spec.Source.WatchPodCreation && p.CreationTimestamp.Time.After(since) {
				cause = "PodCreation"
			} else {
				for _, cs := range p.Status.ContainerStatuses {
					if cs.LastTerminationState.Terminated != nil &&
						cs.LastTerminationState.Terminated.FinishedAt.Time.After(since) &&
						cs.Name == trig.Spec.Source.ContainerName {
						cause = "ContainerRestart"
						break
					}
				}
			}
			srcInfos = append(srcInfos, helpers.PodInfo{
				Name:      p.Name,
				Namespace: p.Namespace,
				Labels:    p.Labels,
				Cause:     cause,
			})
		}

		var tgtInfos []helpers.PodInfo
		for _, name := range restarted {
			tgtInfos = append(tgtInfos, helpers.PodInfo{
				Name:      name,
				Namespace: trig.Namespace,
				Labels:    map[string]string{},
				Cause:     "",
			})
		}

		err := helpers.SendSlackAlert(ctx, helpers.SlackAlertConfig{
			TriggerName:      trig.Name,
			Channel:          trig.Spec.SlackNotification.Channel,
			Webhook:          webhook,
			Timestamp:        now.Format("2006-01-02 15:04:05"),
			SourcePods:       srcInfos,
			TargetPods:       tgtInfos,
			IncludeNamespace: true,
			IncludeLabels:    true,
			ShowMoreEnabled:  true,
		})
		if err != nil {
			l.Error(err, "failed to send Slack alert")
		}
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
