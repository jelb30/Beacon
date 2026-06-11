/*
Copyright 2026.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"time"

	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

const (
	starvationEventObservedDecision = "StarvationEventObserved"

	starvationEventPolicyReadyReason  = "StarvationEventIngested"
	starvationEventPolicyReadyMessage = "Beacon observed a starvation event and routed it to this policy."

	starvationEventPolicyNotFoundReason = "BeaconPolicyNotFound"
	starvationEventPolicyNotFoundMsg    = "Referenced BeaconPolicy was not found."

	starvationEventRoutedReason  = "RoutedToBeaconPolicy"
	starvationEventRoutedMessage = "Starvation event was routed to the referenced BeaconPolicy."
)

const starvationEventPolicyMissingRequeue = 5 * time.Second

// StarvationEventReconciler reconciles a StarvationEvent object
type StarvationEventReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=starvationevents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=starvationevents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=starvationevents/finalizers,verbs=update

func (r *StarvationEventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)

	event := &autoscalingv1alpha1.StarvationEvent{}
	if err := r.Get(ctx, req.NamespacedName, event); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if event.Status.Processed {
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling StarvationEvent",
		"namespace", event.Namespace,
		"name", event.Name,
		"policyName", event.Spec.PolicyName,
		"targetRef", event.Spec.TargetRef,
		"signalType", event.Spec.SignalType,
		"severity", event.Spec.Severity,
		"generation", event.Generation,
	)

	policy := &autoscalingv1alpha1.BeaconPolicy{}
	policyKey := types.NamespacedName{Namespace: event.Namespace, Name: event.Spec.PolicyName}
	if err := r.Get(ctx, policyKey, policy); err != nil {
		if apierrors.IsNotFound(err) {
			now := metav1.NewTime(time.Now())
			if statusErr := r.updateStarvationEventPolicyMissingStatus(ctx, event, now); statusErr != nil {
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: starvationEventPolicyMissingRequeue}, nil
		}
		return ctrl.Result{}, err
	}

	now := metav1.NewTime(time.Now())
	latencyMillis := reactionLatencyMillis(now, event.Spec.ObservedAt)

	if err := r.updateBeaconPolicyForStarvationEvent(ctx, policy, event, now, latencyMillis); err != nil {
		return ctrl.Result{}, err
	}
	if err := r.updateStarvationEventProcessedStatus(ctx, event, policy, now, latencyMillis); err != nil {
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *StarvationEventReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.StarvationEvent{}).
		Named("starvationevent").
		Complete(r)
}

func (r *StarvationEventReconciler) updateBeaconPolicyForStarvationEvent(
	ctx context.Context,
	policy *autoscalingv1alpha1.BeaconPolicy,
	event *autoscalingv1alpha1.StarvationEvent,
	now metav1.Time,
	reactionLatencyMillis int64,
) error {
	updated := policy.DeepCopy()
	updated.Status.ObservedGeneration = updated.Generation
	updated.Status.LastReconcileTime = now.DeepCopy()
	updated.Status.LastDecision = starvationEventObservedDecision
	updated.Status.LastReactionLatencyMillis = reactionLatencyMillis
	updated.Status.LastEventName = event.Name
	updated.Status.LastSignalType = event.Spec.SignalType
	updated.Status.LastEventSeverity = event.Spec.Severity
	updated.Status.LastEventObservedAt = event.Spec.ObservedAt.DeepCopy()
	meta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
		Type:               autoscalingv1alpha1.BeaconPolicyReadyCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: updated.Generation,
		LastTransitionTime: now,
		Reason:             starvationEventPolicyReadyReason,
		Message:            starvationEventPolicyReadyMessage,
	})

	if apiequality.Semantic.DeepEqual(policy.Status, updated.Status) {
		return nil
	}

	return r.Status().Update(ctx, updated)
}

func (r *StarvationEventReconciler) updateStarvationEventProcessedStatus(
	ctx context.Context,
	event *autoscalingv1alpha1.StarvationEvent,
	policy *autoscalingv1alpha1.BeaconPolicy,
	now metav1.Time,
	reactionLatencyMillis int64,
) error {
	updated := event.DeepCopy()
	updated.Status.ObservedGeneration = updated.Generation
	updated.Status.Processed = true
	updated.Status.ProcessedAt = now.DeepCopy()
	updated.Status.RoutedPolicy = policy.Name
	updated.Status.ReactionLatencyMillis = reactionLatencyMillis
	meta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
		Type:               autoscalingv1alpha1.StarvationEventProcessedCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: updated.Generation,
		LastTransitionTime: now,
		Reason:             starvationEventRoutedReason,
		Message:            starvationEventRoutedMessage,
	})

	if apiequality.Semantic.DeepEqual(event.Status, updated.Status) {
		return nil
	}

	return r.Status().Update(ctx, updated)
}

func (r *StarvationEventReconciler) updateStarvationEventPolicyMissingStatus(
	ctx context.Context,
	event *autoscalingv1alpha1.StarvationEvent,
	now metav1.Time,
) error {
	updated := event.DeepCopy()
	updated.Status.ObservedGeneration = updated.Generation
	updated.Status.Processed = false
	meta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
		Type:               autoscalingv1alpha1.StarvationEventProcessedCondition,
		Status:             metav1.ConditionFalse,
		ObservedGeneration: updated.Generation,
		LastTransitionTime: now,
		Reason:             starvationEventPolicyNotFoundReason,
		Message:            starvationEventPolicyNotFoundMsg,
	})

	if apiequality.Semantic.DeepEqual(event.Status, updated.Status) {
		return nil
	}

	return r.Status().Update(ctx, updated)
}

func reactionLatencyMillis(now metav1.Time, observedAt metav1.Time) int64 {
	latency := now.Time.Sub(observedAt.Time).Milliseconds()
	if latency < 0 {
		return 0
	}
	return latency
}
