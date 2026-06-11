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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

var _ = Describe("StarvationEvent Controller", func() {
	Context("When reconciling a resource", func() {
		const namespace = "default"

		ctx := context.Background()

		AfterEach(func() {
			deleteStarvationEventIfExists(ctx, types.NamespacedName{Name: "test-cpu-starvation", Namespace: namespace})
			deleteStarvationEventIfExists(ctx, types.NamespacedName{Name: "missing-policy-starvation", Namespace: namespace})
			deleteBeaconPolicyIfExists(ctx, types.NamespacedName{Name: "test-policy", Namespace: namespace})
		})

		It("marks the event processed and updates the referenced BeaconPolicy", func() {
			policyKey := types.NamespacedName{Name: "test-policy", Namespace: namespace}
			eventKey := types.NamespacedName{Name: "test-cpu-starvation", Namespace: namespace}
			observedAt := metav1.NewTime(time.Now().Add(-2 * time.Second).Truncate(time.Second))

			By("creating the referenced BeaconPolicy")
			Expect(k8sClient.Create(ctx, testBeaconPolicy(policyKey))).To(Succeed())

			By("creating a StarvationEvent for the policy")
			Expect(k8sClient.Create(ctx, testStarvationEvent(eventKey, policyKey.Name, observedAt))).To(Succeed())

			controllerReconciler := &StarvationEventReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: eventKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result).To(Equal(reconcile.Result{}))

			updatedEvent := &autoscalingv1alpha1.StarvationEvent{}
			Expect(k8sClient.Get(ctx, eventKey, updatedEvent)).To(Succeed())
			Expect(updatedEvent.Status.ObservedGeneration).To(Equal(updatedEvent.Generation))
			Expect(updatedEvent.Status.Processed).To(BeTrue())
			Expect(updatedEvent.Status.ProcessedAt).NotTo(BeNil())
			Expect(updatedEvent.Status.RoutedPolicy).To(Equal(policyKey.Name))
			Expect(updatedEvent.Status.ReactionLatencyMillis).To(BeNumerically(">=", int64(0)))

			processed := meta.FindStatusCondition(updatedEvent.Status.Conditions, autoscalingv1alpha1.StarvationEventProcessedCondition)
			Expect(processed).NotTo(BeNil())
			Expect(processed.Status).To(Equal(metav1.ConditionTrue))
			Expect(processed.Reason).To(Equal(starvationEventRoutedReason))

			updatedPolicy := &autoscalingv1alpha1.BeaconPolicy{}
			Expect(k8sClient.Get(ctx, policyKey, updatedPolicy)).To(Succeed())
			Expect(updatedPolicy.Status.ObservedGeneration).To(Equal(updatedPolicy.Generation))
			Expect(updatedPolicy.Status.LastDecision).To(Equal(starvationEventObservedDecision))
			Expect(updatedPolicy.Status.LastReactionLatencyMillis).To(Equal(updatedEvent.Status.ReactionLatencyMillis))
			Expect(updatedPolicy.Status.LastEventName).To(Equal(eventKey.Name))
			Expect(updatedPolicy.Status.LastSignalType).To(Equal(autoscalingv1alpha1.StarvationSignalCPUStarvation))
			Expect(updatedPolicy.Status.LastEventSeverity).To(Equal(autoscalingv1alpha1.StarvationSeverityHigh))
			Expect(updatedPolicy.Status.LastEventObservedAt).NotTo(BeNil())
			Expect(updatedPolicy.Status.LastEventObservedAt.Time.Equal(observedAt.Time)).To(BeTrue())

			ready := meta.FindStatusCondition(updatedPolicy.Status.Conditions, autoscalingv1alpha1.BeaconPolicyReadyCondition)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.Reason).To(Equal(starvationEventPolicyReadyReason))
		})

		It("sets a missing policy condition and requeues", func() {
			eventKey := types.NamespacedName{Name: "missing-policy-starvation", Namespace: namespace}
			observedAt := metav1.NewTime(time.Now().Add(-2 * time.Second).Truncate(time.Second))

			By("creating a StarvationEvent with a missing BeaconPolicy reference")
			Expect(k8sClient.Create(ctx, testStarvationEvent(eventKey, "missing-policy", observedAt))).To(Succeed())

			controllerReconciler := &StarvationEventReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: eventKey})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(Equal(starvationEventPolicyMissingRequeue))

			updatedEvent := &autoscalingv1alpha1.StarvationEvent{}
			Expect(k8sClient.Get(ctx, eventKey, updatedEvent)).To(Succeed())
			Expect(updatedEvent.Status.ObservedGeneration).To(Equal(updatedEvent.Generation))
			Expect(updatedEvent.Status.Processed).To(BeFalse())

			processed := meta.FindStatusCondition(updatedEvent.Status.Conditions, autoscalingv1alpha1.StarvationEventProcessedCondition)
			Expect(processed).NotTo(BeNil())
			Expect(processed.Status).To(Equal(metav1.ConditionFalse))
			Expect(processed.Reason).To(Equal(starvationEventPolicyNotFoundReason))
			Expect(processed.Message).To(Equal(starvationEventPolicyNotFoundMsg))
		})
	})
})

func testBeaconPolicy(key types.NamespacedName) *autoscalingv1alpha1.BeaconPolicy {
	return &autoscalingv1alpha1.BeaconPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: autoscalingv1alpha1.BeaconPolicySpec{
			TargetRef: autoscalingv1alpha1.BeaconPolicyTargetRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "sample-api",
			},
			MinCPURequest:               "100m",
			MaxCPURequest:               "1000m",
			ScaleUpStepPercent:          25,
			StarvationWindowSeconds:     10,
			ReactionLatencyBudgetMillis: 4000,
		},
	}
}

func testStarvationEvent(
	key types.NamespacedName,
	policyName string,
	observedAt metav1.Time,
) *autoscalingv1alpha1.StarvationEvent {
	return &autoscalingv1alpha1.StarvationEvent{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: autoscalingv1alpha1.StarvationEventSpec{
			PolicyName: policyName,
			TargetRef: autoscalingv1alpha1.StarvationEventTargetRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       "sample-api",
			},
			ContainerName: "api",
			SignalType:    autoscalingv1alpha1.StarvationSignalCPUStarvation,
			ObservedAt:    observedAt,
			Severity:      autoscalingv1alpha1.StarvationSeverityHigh,
			Message:       "Synthetic CPU starvation event for tests.",
		},
	}
}

func deleteStarvationEventIfExists(ctx context.Context, key types.NamespacedName) {
	resource := &autoscalingv1alpha1.StarvationEvent{}
	err := k8sClient.Get(ctx, key, resource)
	if errors.IsNotFound(err) {
		return
	}
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
}

func deleteBeaconPolicyIfExists(ctx context.Context, key types.NamespacedName) {
	resource := &autoscalingv1alpha1.BeaconPolicy{}
	err := k8sClient.Get(ctx, key, resource)
	if errors.IsNotFound(err) {
		return
	}
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
}
