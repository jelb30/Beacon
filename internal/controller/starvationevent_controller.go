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
	"fmt"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiequality "k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
	"github.com/jelb30/Beacon/internal/observability"
)

const (
	scaleActionNoop                         = "Noop"
	scaleActionCPURequestIncreased          = "CPURequestIncreased"
	scaleActionMemoryRequestIncreased       = "MemoryRequestIncreased"
	scaleActionCPUAndMemoryRequestIncreased = "CPUAndMemoryRequestIncreased"
	scaleActionMaxLimitReached              = "MaxLimitReached"
	scaleActionTargetDeploymentNotFound     = "TargetDeploymentNotFound"
	scaleActionTargetContainerNotFound      = "TargetContainerNotFound"
	decisionVerticalScalePatchApplied       = "VerticalScalePatchApplied"
	decisionStarvationEventObserved         = "StarvationEventObserved"
	reasonVerticalScalePatchApplied         = "VerticalScalePatchApplied"
	reasonMaxLimitReached                   = "MaxLimitReached"
	reasonTargetDeploymentNotFound          = "TargetDeploymentNotFound"
	reasonTargetContainerNotFound           = "TargetContainerNotFound"
	reasonBeaconPolicyNotFound              = "BeaconPolicyNotFound"
	reasonNoop                              = "Noop"
	messageVerticalScalePatchApplied        = "Beacon applied a vertical scaling patch to the target Deployment."
	messageEventVerticalScalePatchApplied   = "Starvation event was processed and a vertical scaling patch was applied."
	messageMaxLimitReached                  = "Configured maximum request is already reached."
	messageEventMaxLimitReached             = "Starvation event was processed but the configured maximum request was already reached."
	messageTargetDeploymentNotFound         = "Target Deployment was not found."
	messageTargetContainerNotFound          = "Target container was not found in the target Deployment."
	messageBeaconPolicyNotFound             = "Referenced BeaconPolicy was not found."
	messageNoop                             = "Starvation event was processed without a scaling patch."
)

const targetMissingRequeue = 5 * time.Second

// StarvationEventReconciler reconciles a StarvationEvent object
type StarvationEventReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=starvationevents,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=starvationevents/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=autoscaling.beacon.dev,resources=starvationevents/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;update;patch

func (r *StarvationEventReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	reconcileStart := time.Now()
	log := logf.FromContext(ctx)
	tracer := observability.Tracer("github.com/jelb30/Beacon/internal/controller")
	ctx, span := tracer.Start(ctx, "StarvationEventReconcile")
	span.SetAttributes(
		attribute.String("namespace", req.Namespace),
		attribute.String("name", req.Name),
	)
	defer span.End()

	metricSignalType := "unknown"
	metricSeverity := "unknown"
	metricScaleAction := "unknown"
	defer func() {
		observability.RecordReconcileLatency("starvationevent", metricSignalType, metricSeverity, metricScaleAction, time.Since(reconcileStart))
	}()

	event := &autoscalingv1alpha1.StarvationEvent{}
	fetchEventCtx, fetchEventSpan := tracer.Start(ctx, "fetch_starvation_event")
	if err := r.Get(fetchEventCtx, req.NamespacedName, event); err != nil {
		if apierrors.IsNotFound(err) {
			fetchEventSpan.End()
			return ctrl.Result{}, nil
		}
		observability.RecordSpanError(fetchEventSpan, err)
		observability.RecordSpanError(span, err)
		fetchEventSpan.End()
		observability.RecordVerticalScaleError(metricSignalType, metricSeverity, "fetch_starvation_event")
		return ctrl.Result{}, err
	}
	fetchEventSpan.End()
	metricSignalType = event.Spec.SignalType
	metricSeverity = event.Spec.Severity
	span.SetAttributes(
		attribute.String("policyName", event.Spec.PolicyName),
		attribute.String("signalType", event.Spec.SignalType),
		attribute.String("severity", event.Spec.Severity),
		attribute.String("containerName", event.Spec.ContainerName),
	)

	if event.Status.Processed {
		metricScaleAction = "already_processed"
		return ctrl.Result{}, nil
	}

	log.Info("Reconciling StarvationEvent",
		"namespace", event.Namespace,
		"name", event.Name,
		"policyName", event.Spec.PolicyName,
		"targetRef", event.Spec.TargetRef,
		"containerName", event.Spec.ContainerName,
		"signalType", event.Spec.SignalType,
		"severity", event.Spec.Severity,
		"generation", event.Generation,
	)

	policy := &autoscalingv1alpha1.BeaconPolicy{}
	policyKey := types.NamespacedName{Namespace: event.Namespace, Name: event.Spec.PolicyName}
	fetchPolicyCtx, fetchPolicySpan := tracer.Start(ctx, "fetch_beacon_policy")
	if err := r.Get(fetchPolicyCtx, policyKey, policy); err != nil {
		if apierrors.IsNotFound(err) {
			observability.RecordSpanError(fetchPolicySpan, err)
			observability.RecordSpanError(span, err)
			fetchPolicySpan.End()
			metricScaleAction = reasonBeaconPolicyNotFound
			observability.RecordVerticalScaleError(metricSignalType, metricSeverity, metricScaleAction)
			now := metav1.NewTime(time.Now())
			if statusErr := r.updateEventStatus(ctx, event, eventStatusPatch{
				now:                   now,
				processed:             false,
				conditionStatus:       metav1.ConditionFalse,
				conditionReason:       reasonBeaconPolicyNotFound,
				conditionMessage:      messageBeaconPolicyNotFound,
				reactionLatencyMillis: reactionLatencyMillis(now, event.Spec.ObservedAt),
			}); statusErr != nil {
				observability.RecordSpanError(span, statusErr)
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: targetMissingRequeue}, nil
		}
		observability.RecordSpanError(fetchPolicySpan, err)
		observability.RecordSpanError(span, err)
		fetchPolicySpan.End()
		metricScaleAction = "fetch_beacon_policy"
		observability.RecordVerticalScaleError(metricSignalType, metricSeverity, metricScaleAction)
		return ctrl.Result{}, err
	}
	fetchPolicySpan.End()

	target := resolveDeploymentTarget(event, policy)
	span.SetAttributes(attribute.String("targetDeployment", target.name))
	now := metav1.NewTime(time.Now())
	latencyMillis := reactionLatencyMillis(now, event.Spec.ObservedAt)
	if !target.supported || target.name == "" {
		missingTargetErr := fmt.Errorf("target Deployment %q is not supported or empty", target.name)
		observability.RecordSpanError(span, missingTargetErr)
		metricScaleAction = scaleActionTargetDeploymentNotFound
		observability.RecordVerticalScaleError(metricSignalType, metricSeverity, metricScaleAction)
		if err := r.updatePolicyStatus(ctx, policy, policyStatusPatch{
			now:                   now,
			event:                 event,
			lastDecision:          scaleActionTargetDeploymentNotFound,
			conditionStatus:       metav1.ConditionFalse,
			conditionReason:       reasonTargetDeploymentNotFound,
			conditionMessage:      messageTargetDeploymentNotFound,
			reactionLatencyMillis: latencyMillis,
			scaleAction:           scaleActionTargetDeploymentNotFound,
			patchedDeployment:     target.name,
			patchedContainer:      event.Spec.ContainerName,
		}); err != nil {
			observability.RecordSpanError(span, err)
			return ctrl.Result{}, err
		}
		if err := r.updateEventStatus(ctx, event, eventStatusPatch{
			now:                   now,
			processed:             false,
			conditionStatus:       metav1.ConditionFalse,
			conditionReason:       reasonTargetDeploymentNotFound,
			conditionMessage:      messageTargetDeploymentNotFound,
			reactionLatencyMillis: latencyMillis,
		}); err != nil {
			observability.RecordSpanError(span, err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: targetMissingRequeue}, nil
	}

	deploymentKey := types.NamespacedName{Namespace: target.namespace, Name: target.name}
	scaleResult, containerMissing, err := r.patchDeploymentRequests(ctx, deploymentKey, event.Spec.ContainerName, policy.Spec, event.Spec.SignalType)
	if err != nil {
		if apierrors.IsNotFound(err) {
			observability.RecordSpanError(span, err)
			metricScaleAction = scaleActionTargetDeploymentNotFound
			observability.RecordVerticalScaleError(metricSignalType, metricSeverity, metricScaleAction)
			if statusErr := r.updatePolicyStatus(ctx, policy, policyStatusPatch{
				now:                   now,
				event:                 event,
				lastDecision:          scaleActionTargetDeploymentNotFound,
				conditionStatus:       metav1.ConditionFalse,
				conditionReason:       reasonTargetDeploymentNotFound,
				conditionMessage:      messageTargetDeploymentNotFound,
				reactionLatencyMillis: latencyMillis,
				scaleAction:           scaleActionTargetDeploymentNotFound,
				patchedDeployment:     target.name,
				patchedContainer:      event.Spec.ContainerName,
			}); statusErr != nil {
				observability.RecordSpanError(span, statusErr)
				return ctrl.Result{}, statusErr
			}
			if statusErr := r.updateEventStatus(ctx, event, eventStatusPatch{
				now:                   now,
				processed:             false,
				conditionStatus:       metav1.ConditionFalse,
				conditionReason:       reasonTargetDeploymentNotFound,
				conditionMessage:      messageTargetDeploymentNotFound,
				reactionLatencyMillis: latencyMillis,
			}); statusErr != nil {
				observability.RecordSpanError(span, statusErr)
				return ctrl.Result{}, statusErr
			}
			return ctrl.Result{RequeueAfter: targetMissingRequeue}, nil
		}
		observability.RecordSpanError(span, err)
		metricScaleAction = "patch_deployment"
		observability.RecordVerticalScaleError(metricSignalType, metricSeverity, metricScaleAction)
		return ctrl.Result{}, err
	}

	if containerMissing {
		containerErr := fmt.Errorf("target container %q was not found", event.Spec.ContainerName)
		observability.RecordSpanError(span, containerErr)
		metricScaleAction = scaleActionTargetContainerNotFound
		observability.RecordVerticalScaleError(metricSignalType, metricSeverity, metricScaleAction)
		if err := r.updatePolicyStatus(ctx, policy, policyStatusPatch{
			now:                   now,
			event:                 event,
			lastDecision:          scaleActionTargetContainerNotFound,
			conditionStatus:       metav1.ConditionFalse,
			conditionReason:       reasonTargetContainerNotFound,
			conditionMessage:      messageTargetContainerNotFound,
			reactionLatencyMillis: latencyMillis,
			scaleAction:           scaleActionTargetContainerNotFound,
			patchedDeployment:     target.name,
			patchedContainer:      event.Spec.ContainerName,
		}); err != nil {
			observability.RecordSpanError(span, err)
			return ctrl.Result{}, err
		}
		if err := r.updateEventStatus(ctx, event, eventStatusPatch{
			now:                   now,
			processed:             false,
			conditionStatus:       metav1.ConditionFalse,
			conditionReason:       reasonTargetContainerNotFound,
			conditionMessage:      messageTargetContainerNotFound,
			reactionLatencyMillis: latencyMillis,
		}); err != nil {
			observability.RecordSpanError(span, err)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: targetMissingRequeue}, nil
	}

	now = metav1.NewTime(time.Now())
	latencyMillis = reactionLatencyMillis(now, event.Spec.ObservedAt)
	outcome := outcomeForScaleResult(scaleResult, now)
	metricScaleAction = scaleResult.action
	span.SetAttributes(
		attribute.String("previous_cpu_request", scaleResult.previousCPU),
		attribute.String("new_cpu_request", scaleResult.newCPU),
		attribute.String("previous_memory_request", scaleResult.previousMemory),
		attribute.String("new_memory_request", scaleResult.newMemory),
		attribute.String("scale_action", scaleResult.action),
		attribute.Int64("reaction_latency_ms", latencyMillis),
		attribute.String("decision", outcome.policyDecision),
	)
	if err := r.updatePolicyStatus(ctx, policy, policyStatusPatch{
		now:                   now,
		event:                 event,
		lastDecision:          outcome.policyDecision,
		conditionStatus:       outcome.policyConditionStatus,
		conditionReason:       outcome.policyConditionReason,
		conditionMessage:      outcome.policyConditionMessage,
		reactionLatencyMillis: latencyMillis,
		lastScaledAt:          outcome.lastScaledAt,
		patchedDeployment:     target.name,
		patchedContainer:      event.Spec.ContainerName,
		previousCPURequest:    scaleResult.previousCPU,
		newCPURequest:         scaleResult.newCPU,
		previousMemoryRequest: scaleResult.previousMemory,
		newMemoryRequest:      scaleResult.newMemory,
		scaleAction:           scaleResult.action,
	}); err != nil {
		observability.RecordSpanError(span, err)
		observability.RecordVerticalScaleError(metricSignalType, metricSeverity, metricScaleAction)
		return ctrl.Result{}, err
	}

	if err := r.updateEventStatus(ctx, event, eventStatusPatch{
		now:                   now,
		processed:             true,
		routedPolicy:          policy.Name,
		conditionStatus:       metav1.ConditionTrue,
		conditionReason:       outcome.eventConditionReason,
		conditionMessage:      outcome.eventConditionMessage,
		reactionLatencyMillis: latencyMillis,
	}); err != nil {
		observability.RecordSpanError(span, err)
		observability.RecordVerticalScaleError(metricSignalType, metricSeverity, metricScaleAction)
		return ctrl.Result{}, err
	}

	observability.RecordStarvationEventProcessed(metricSignalType, metricSeverity, metricScaleAction)
	if scaleResult.shouldPatch {
		observability.RecordVerticalScalePatch(metricSignalType, metricSeverity, metricScaleAction)
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

type deploymentTarget struct {
	namespace string
	name      string
	supported bool
}

type scaleResult struct {
	action         string
	shouldPatch    bool
	resourceName   corev1.ResourceName
	newQuantity    resource.Quantity
	previousCPU    string
	newCPU         string
	previousMemory string
	newMemory      string
}

type policyStatusPatch struct {
	now                   metav1.Time
	event                 *autoscalingv1alpha1.StarvationEvent
	lastDecision          string
	conditionStatus       metav1.ConditionStatus
	conditionReason       string
	conditionMessage      string
	reactionLatencyMillis int64
	lastScaledAt          *metav1.Time
	patchedDeployment     string
	patchedContainer      string
	previousCPURequest    string
	newCPURequest         string
	previousMemoryRequest string
	newMemoryRequest      string
	scaleAction           string
}

type eventStatusPatch struct {
	now                   metav1.Time
	processed             bool
	routedPolicy          string
	conditionStatus       metav1.ConditionStatus
	conditionReason       string
	conditionMessage      string
	reactionLatencyMillis int64
}

type scaleOutcome struct {
	policyDecision         string
	policyConditionStatus  metav1.ConditionStatus
	policyConditionReason  string
	policyConditionMessage string
	eventConditionReason   string
	eventConditionMessage  string
	lastScaledAt           *metav1.Time
}

func resolveDeploymentTarget(event *autoscalingv1alpha1.StarvationEvent, policy *autoscalingv1alpha1.BeaconPolicy) deploymentTarget {
	namespace := event.Spec.TargetRef.Namespace
	if namespace == "" {
		namespace = policy.Spec.TargetRef.Namespace
	}
	if namespace == "" {
		namespace = event.Namespace
	}

	name := event.Spec.TargetRef.Name
	if name == "" {
		name = policy.Spec.TargetRef.Name
	}

	kind := event.Spec.TargetRef.Kind
	if kind == "" {
		kind = policy.Spec.TargetRef.Kind
	}

	return deploymentTarget{
		namespace: namespace,
		name:      name,
		supported: strings.EqualFold(kind, "Deployment"),
	}
}

func findContainerIndex(deployment *appsv1.Deployment, containerName string) int {
	for i := range deployment.Spec.Template.Spec.Containers {
		if deployment.Spec.Template.Spec.Containers[i].Name == containerName {
			return i
		}
	}
	return -1
}

func (r *StarvationEventReconciler) patchDeploymentRequests(
	ctx context.Context,
	key types.NamespacedName,
	containerName string,
	policy autoscalingv1alpha1.BeaconPolicySpec,
	signalType string,
) (scaleResult, bool, error) {
	var result scaleResult
	var containerMissing bool

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		tracer := observability.Tracer("github.com/jelb30/Beacon/internal/controller")
		deployment := &appsv1.Deployment{}
		fetchCtx, fetchSpan := tracer.Start(ctx, "fetch_target_deployment")
		fetchSpan.SetAttributes(
			attribute.String("namespace", key.Namespace),
			attribute.String("targetDeployment", key.Name),
		)
		if err := r.Get(fetchCtx, key, deployment); err != nil {
			observability.RecordSpanError(fetchSpan, err)
			fetchSpan.End()
			return err
		}
		fetchSpan.End()

		updatedDeployment := deployment.DeepCopy()
		_, calcSpan := tracer.Start(ctx, "calculate_resource_patch")
		calcSpan.SetAttributes(
			attribute.String("targetDeployment", key.Name),
			attribute.String("containerName", containerName),
			attribute.String("signalType", signalType),
		)
		containerIndex := findContainerIndex(updatedDeployment, containerName)
		if containerIndex < 0 {
			containerMissing = true
			result = scaleResult{}
			observability.RecordSpanError(calcSpan, fmt.Errorf("target container %q was not found", containerName))
			calcSpan.End()
			return nil
		}
		containerMissing = false

		nextResult, err := calculateScaleResult(
			updatedDeployment.Spec.Template.Spec.Containers[containerIndex],
			policy,
			signalType,
		)
		if err != nil {
			observability.RecordSpanError(calcSpan, err)
			calcSpan.End()
			return err
		}

		result = nextResult
		calcSpan.SetAttributes(
			attribute.String("previous_cpu_request", nextResult.previousCPU),
			attribute.String("new_cpu_request", nextResult.newCPU),
			attribute.String("previous_memory_request", nextResult.previousMemory),
			attribute.String("new_memory_request", nextResult.newMemory),
			attribute.String("scale_action", nextResult.action),
		)
		calcSpan.End()

		patchCtx, patchSpan := tracer.Start(ctx, "patch_deployment")
		patchSpan.SetAttributes(
			attribute.String("namespace", key.Namespace),
			attribute.String("targetDeployment", key.Name),
			attribute.String("containerName", containerName),
			attribute.String("scale_action", nextResult.action),
			attribute.Bool("should_patch", nextResult.shouldPatch),
		)
		defer patchSpan.End()
		if !nextResult.shouldPatch {
			return nil
		}

		applyScaleResult(&updatedDeployment.Spec.Template.Spec.Containers[containerIndex], nextResult)
		if err := r.Update(patchCtx, updatedDeployment); err != nil {
			observability.RecordSpanError(patchSpan, err)
			return err
		}

		return nil
	})
	if err != nil {
		return scaleResult{}, false, err
	}

	return result, containerMissing, nil
}

func calculateScaleResult(
	container corev1.Container,
	policy autoscalingv1alpha1.BeaconPolicySpec,
	signalType string,
) (scaleResult, error) {
	switch signalType {
	case autoscalingv1alpha1.StarvationSignalCPUStarvation:
		return calculateCPUScaleResult(container, policy)
	case autoscalingv1alpha1.StarvationSignalMemoryStarvation:
		return calculateMemoryScaleResult(container, policy)
	default:
		return scaleResult{}, fmt.Errorf("unsupported starvation signal type %q", signalType)
	}
}

func calculateCPUScaleResult(container corev1.Container, policy autoscalingv1alpha1.BeaconPolicySpec) (scaleResult, error) {
	calculation, err := calculateIncreasedQuantity(
		container.Resources.Requests,
		corev1.ResourceCPU,
		policy.MinCPURequest,
		policy.MaxCPURequest,
		policy.ScaleUpStepPercent,
		resource.DecimalSI,
	)
	if err != nil {
		return scaleResult{}, err
	}

	return scaleResult{
		action:       calculation.actionFor(corev1.ResourceCPU),
		shouldPatch:  calculation.shouldPatch,
		resourceName: corev1.ResourceCPU,
		newQuantity:  calculation.newQuantity,
		previousCPU:  calculation.previous,
		newCPU:       calculation.next,
	}, nil
}

func calculateMemoryScaleResult(container corev1.Container, policy autoscalingv1alpha1.BeaconPolicySpec) (scaleResult, error) {
	if policy.MinMemoryRequest == "" || policy.MaxMemoryRequest == "" {
		return scaleResult{action: scaleActionNoop}, nil
	}

	calculation, err := calculateIncreasedQuantity(
		container.Resources.Requests,
		corev1.ResourceMemory,
		policy.MinMemoryRequest,
		policy.MaxMemoryRequest,
		policy.ScaleUpStepPercent,
		resource.BinarySI,
	)
	if err != nil {
		return scaleResult{}, err
	}

	return scaleResult{
		action:         calculation.actionFor(corev1.ResourceMemory),
		shouldPatch:    calculation.shouldPatch,
		resourceName:   corev1.ResourceMemory,
		newQuantity:    calculation.newQuantity,
		previousMemory: calculation.previous,
		newMemory:      calculation.next,
	}, nil
}

type quantityCalculation struct {
	shouldPatch bool
	maxReached  bool
	previous    string
	next        string
	newQuantity resource.Quantity
}

func (q quantityCalculation) actionFor(resourceName corev1.ResourceName) string {
	if !q.shouldPatch {
		if q.maxReached {
			return scaleActionMaxLimitReached
		}
		return scaleActionNoop
	}

	switch resourceName {
	case corev1.ResourceCPU:
		return scaleActionCPURequestIncreased
	case corev1.ResourceMemory:
		return scaleActionMemoryRequestIncreased
	default:
		return scaleActionNoop
	}
}

func calculateIncreasedQuantity(
	requests corev1.ResourceList,
	resourceName corev1.ResourceName,
	minRequest string,
	maxRequest string,
	scaleUpStepPercent int32,
	format resource.Format,
) (quantityCalculation, error) {
	minQuantity, err := resource.ParseQuantity(minRequest)
	if err != nil {
		return quantityCalculation{}, fmt.Errorf("parse minimum %s request %q: %w", resourceName, minRequest, err)
	}
	maxQuantity, err := resource.ParseQuantity(maxRequest)
	if err != nil {
		return quantityCalculation{}, fmt.Errorf("parse maximum %s request %q: %w", resourceName, maxRequest, err)
	}
	if maxQuantity.Cmp(minQuantity) < 0 {
		return quantityCalculation{}, fmt.Errorf("maximum %s request %q is lower than minimum %q", resourceName, maxRequest, minRequest)
	}

	currentQuantity, hasCurrent := requests[resourceName]
	previous := ""
	if hasCurrent {
		previous = currentQuantity.String()
	}

	baseline := minQuantity.DeepCopy()
	if hasCurrent && currentQuantity.Cmp(baseline) > 0 {
		baseline = currentQuantity.DeepCopy()
	}

	if baseline.Cmp(maxQuantity) >= 0 {
		return quantityCalculation{
			maxReached:  true,
			previous:    previous,
			next:        maxQuantity.String(),
			newQuantity: maxQuantity,
		}, nil
	}

	newQuantity := increasedQuantity(baseline, scaleUpStepPercent, format)
	if newQuantity.Cmp(maxQuantity) > 0 {
		newQuantity = maxQuantity.DeepCopy()
	}

	if hasCurrent && newQuantity.Cmp(currentQuantity) <= 0 {
		return quantityCalculation{
			previous:    previous,
			next:        currentQuantity.String(),
			newQuantity: currentQuantity,
		}, nil
	}

	return quantityCalculation{
		shouldPatch: true,
		previous:    previous,
		next:        newQuantity.String(),
		newQuantity: newQuantity,
	}, nil
}

func increasedQuantity(base resource.Quantity, scaleUpStepPercent int32, format resource.Format) resource.Quantity {
	switch format {
	case resource.DecimalSI:
		currentMilli := base.MilliValue()
		nextMilli := currentMilli * int64(100+scaleUpStepPercent) / 100
		if nextMilli <= currentMilli {
			nextMilli = currentMilli + 1
		}
		return *resource.NewMilliQuantity(nextMilli, resource.DecimalSI)
	default:
		currentValue := base.Value()
		nextValue := currentValue * int64(100+scaleUpStepPercent) / 100
		if nextValue <= currentValue {
			nextValue = currentValue + 1
		}
		return *resource.NewQuantity(nextValue, resource.BinarySI)
	}
}

func applyScaleResult(container *corev1.Container, result scaleResult) {
	if container.Resources.Requests == nil {
		container.Resources.Requests = corev1.ResourceList{}
	}
	container.Resources.Requests[result.resourceName] = result.newQuantity

	if limit, ok := container.Resources.Limits[result.resourceName]; ok && limit.Cmp(result.newQuantity) < 0 {
		if container.Resources.Limits == nil {
			container.Resources.Limits = corev1.ResourceList{}
		}
		container.Resources.Limits[result.resourceName] = result.newQuantity
	}
}

func outcomeForScaleResult(result scaleResult, now metav1.Time) scaleOutcome {
	switch result.action {
	case scaleActionCPURequestIncreased, scaleActionMemoryRequestIncreased, scaleActionCPUAndMemoryRequestIncreased:
		return scaleOutcome{
			policyDecision:         decisionVerticalScalePatchApplied,
			policyConditionStatus:  metav1.ConditionTrue,
			policyConditionReason:  reasonVerticalScalePatchApplied,
			policyConditionMessage: messageVerticalScalePatchApplied,
			eventConditionReason:   reasonVerticalScalePatchApplied,
			eventConditionMessage:  messageEventVerticalScalePatchApplied,
			lastScaledAt:           now.DeepCopy(),
		}
	case scaleActionMaxLimitReached:
		return scaleOutcome{
			policyDecision:         scaleActionMaxLimitReached,
			policyConditionStatus:  metav1.ConditionTrue,
			policyConditionReason:  reasonMaxLimitReached,
			policyConditionMessage: messageMaxLimitReached,
			eventConditionReason:   reasonMaxLimitReached,
			eventConditionMessage:  messageEventMaxLimitReached,
		}
	default:
		return scaleOutcome{
			policyDecision:         decisionStarvationEventObserved,
			policyConditionStatus:  metav1.ConditionTrue,
			policyConditionReason:  reasonNoop,
			policyConditionMessage: messageNoop,
			eventConditionReason:   reasonNoop,
			eventConditionMessage:  messageNoop,
		}
	}
}

func (r *StarvationEventReconciler) updatePolicyStatus(
	ctx context.Context,
	policy *autoscalingv1alpha1.BeaconPolicy,
	patch policyStatusPatch,
) error {
	key := types.NamespacedName{Name: policy.Name, Namespace: policy.Namespace}
	ctx, span := observability.Tracer("github.com/jelb30/Beacon/internal/controller").Start(ctx, "update_beacon_policy_status")
	span.SetAttributes(
		attribute.String("namespace", policy.Namespace),
		attribute.String("name", policy.Name),
		attribute.String("decision", patch.lastDecision),
		attribute.String("scale_action", patch.scaleAction),
		attribute.Int64("reaction_latency_ms", patch.reactionLatencyMillis),
	)
	defer span.End()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &autoscalingv1alpha1.BeaconPolicy{}
		if err := r.Get(ctx, key, latest); err != nil {
			observability.RecordSpanError(span, err)
			return err
		}

		updated := latest.DeepCopy()
		updated.Status.ObservedGeneration = updated.Generation
		updated.Status.LastReconcileTime = patch.now.DeepCopy()
		updated.Status.LastDecision = patch.lastDecision
		updated.Status.LastReactionLatencyMillis = patch.reactionLatencyMillis
		updated.Status.LastEventName = patch.event.Name
		updated.Status.LastSignalType = patch.event.Spec.SignalType
		updated.Status.LastEventSeverity = patch.event.Spec.Severity
		updated.Status.LastEventObservedAt = patch.event.Spec.ObservedAt.DeepCopy()
		updated.Status.LastScaledAt = patch.lastScaledAt
		updated.Status.LastPatchedDeployment = patch.patchedDeployment
		updated.Status.LastPatchedContainer = patch.patchedContainer
		updated.Status.PreviousCPURequest = patch.previousCPURequest
		updated.Status.NewCPURequest = patch.newCPURequest
		updated.Status.PreviousMemoryRequest = patch.previousMemoryRequest
		updated.Status.NewMemoryRequest = patch.newMemoryRequest
		updated.Status.ScaleAction = patch.scaleAction
		meta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
			Type:               autoscalingv1alpha1.BeaconPolicyReadyCondition,
			Status:             patch.conditionStatus,
			ObservedGeneration: updated.Generation,
			LastTransitionTime: patch.now,
			Reason:             patch.conditionReason,
			Message:            patch.conditionMessage,
		})

		if apiequality.Semantic.DeepEqual(latest.Status, updated.Status) {
			return nil
		}

		return r.Status().Update(ctx, updated)
	})
	if err != nil {
		observability.RecordSpanError(span, err)
		return err
	}

	return nil
}

func (r *StarvationEventReconciler) updateEventStatus(
	ctx context.Context,
	event *autoscalingv1alpha1.StarvationEvent,
	patch eventStatusPatch,
) error {
	key := types.NamespacedName{Name: event.Name, Namespace: event.Namespace}
	ctx, span := observability.Tracer("github.com/jelb30/Beacon/internal/controller").Start(ctx, "update_starvation_event_status")
	span.SetAttributes(
		attribute.String("namespace", event.Namespace),
		attribute.String("name", event.Name),
		attribute.Bool("processed", patch.processed),
		attribute.String("condition_reason", patch.conditionReason),
		attribute.Int64("reaction_latency_ms", patch.reactionLatencyMillis),
	)
	defer span.End()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &autoscalingv1alpha1.StarvationEvent{}
		if err := r.Get(ctx, key, latest); err != nil {
			observability.RecordSpanError(span, err)
			return err
		}

		updated := latest.DeepCopy()
		updated.Status.ObservedGeneration = updated.Generation
		updated.Status.Processed = patch.processed
		if patch.processed {
			updated.Status.ProcessedAt = patch.now.DeepCopy()
		}
		updated.Status.RoutedPolicy = patch.routedPolicy
		updated.Status.ReactionLatencyMillis = patch.reactionLatencyMillis
		meta.SetStatusCondition(&updated.Status.Conditions, metav1.Condition{
			Type:               autoscalingv1alpha1.StarvationEventProcessedCondition,
			Status:             patch.conditionStatus,
			ObservedGeneration: updated.Generation,
			LastTransitionTime: patch.now,
			Reason:             patch.conditionReason,
			Message:            patch.conditionMessage,
		})

		if apiequality.Semantic.DeepEqual(latest.Status, updated.Status) {
			return nil
		}

		return r.Status().Update(ctx, updated)
	})
	if err != nil {
		observability.RecordSpanError(span, err)
		return err
	}

	return nil
}

func reactionLatencyMillis(now metav1.Time, observedAt metav1.Time) int64 {
	latency := now.Time.Sub(observedAt.Time).Milliseconds()
	if latency < 0 {
		return 0
	}
	return latency
}
