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
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

const (
	phase1Decision     = "NoopPhase1ScaffoldReady"
	phase1ReadyReason  = "Phase1ScaffoldReady"
	phase1ReadyMessage = "Beacon operator scaffold is installed and reconciling BeaconPolicy resources."
)

// BeaconPolicyReconciler reconciles a BeaconPolicy object
type BeaconPolicyReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=beaconpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=beaconpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=beaconpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch
// +kubebuilder:rbac:groups="",resources=pods,verbs=get;list;watch

func (r *BeaconPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	start := time.Now()
	log := logf.FromContext(ctx)

	policy := &autoscalingv1alpha1.BeaconPolicy{}
	if err := r.Get(ctx, req.NamespacedName, policy); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	log.Info("Reconciling BeaconPolicy",
		"namespace", policy.Namespace,
		"name", policy.Name,
		"targetRef", policy.Spec.TargetRef,
		"generation", policy.Generation,
	)

	if beaconPolicyReadyForCurrentGeneration(policy) {
		return ctrl.Result{}, nil
	}

	now := metav1.NewTime(time.Now())
	updated := policy.DeepCopy()
	updated.Status.ObservedGeneration = updated.Generation
	updated.Status.LastReconcileTime = &now
	updated.Status.LastDecision = phase1Decision
	updated.Status.LastReactionLatencyMillis = time.Since(start).Milliseconds()
	meta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
		Type:               autoscalingv1alpha1.BeaconPolicyReadyCondition,
		Status:             metav1.ConditionTrue,
		ObservedGeneration: updated.Generation,
		LastTransitionTime: now,
		Reason:             phase1ReadyReason,
		Message:            phase1ReadyMessage,
	})

	if apiequality.Semantic.DeepEqual(policy.Status, updated.Status) {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, r.Status().Update(ctx, updated)
}

// SetupWithManager sets up the controller with the Manager.
func (r *BeaconPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&autoscalingv1alpha1.BeaconPolicy{}).
		Named("beaconpolicy").
		Complete(r)
}

func phase1StatusCurrent(policy *autoscalingv1alpha1.BeaconPolicy) bool {
	ready := meta.FindStatusCondition(policy.Status.Conditions, autoscalingv1alpha1.BeaconPolicyReadyCondition)
	if ready == nil {
		return false
	}

	return policy.Status.ObservedGeneration == policy.Generation &&
		policy.Status.LastReconcileTime != nil &&
		policy.Status.LastDecision == phase1Decision &&
		ready.Status == metav1.ConditionTrue &&
		ready.ObservedGeneration == policy.Generation &&
		ready.Reason == phase1ReadyReason &&
		ready.Message == phase1ReadyMessage
}

func beaconPolicyReadyForCurrentGeneration(policy *autoscalingv1alpha1.BeaconPolicy) bool {
	ready := meta.FindStatusCondition(policy.Status.Conditions, autoscalingv1alpha1.BeaconPolicyReadyCondition)
	if ready == nil {
		return false
	}

	return policy.Status.ObservedGeneration == policy.Generation &&
		ready.Status == metav1.ConditionTrue &&
		ready.ObservedGeneration == policy.Generation
}
