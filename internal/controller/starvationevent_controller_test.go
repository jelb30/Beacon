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
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

var _ = Describe("Resource scaling helpers", func() {
	policy := autoscalingv1alpha1.BeaconPolicySpec{
		MinCPURequest:               "100m",
		MaxCPURequest:               "1000m",
		MinMemoryRequest:            "128Mi",
		MaxMemoryRequest:            "1Gi",
		ScaleUpStepPercent:          25,
		StarvationWindowSeconds:     10,
		ReactionLatencyBudgetMillis: 4000,
	}

	It("increases CPU requests by the configured percentage", func() {
		container := testContainer("api", "100m", "128Mi")

		result, err := calculateCPUScaleResult(container, policy)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.action).To(Equal(scaleActionCPURequestIncreased))
		Expect(result.shouldPatch).To(BeTrue())
		Expect(result.previousCPU).To(Equal("100m"))
		Expect(result.newCPU).To(Equal("125m"))
	})

	It("respects the CPU max cap", func() {
		container := testContainer("api", "1000m", "128Mi")

		result, err := calculateCPUScaleResult(container, policy)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.action).To(Equal(scaleActionMaxLimitReached))
		Expect(result.shouldPatch).To(BeFalse())
		Expect(result.newQuantity.Cmp(resource.MustParse(policy.MaxCPURequest))).To(Equal(0))
	})

	It("uses minCPURequest when the current CPU request is missing", func() {
		container := testContainer("api", "", "128Mi")

		result, err := calculateCPUScaleResult(container, policy)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.action).To(Equal(scaleActionCPURequestIncreased))
		Expect(result.shouldPatch).To(BeTrue())
		Expect(result.previousCPU).To(BeEmpty())
		Expect(result.newCPU).To(Equal("125m"))
	})

	It("increases memory requests by the configured percentage", func() {
		container := testContainer("api", "100m", "128Mi")

		result, err := calculateMemoryScaleResult(container, policy)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.action).To(Equal(scaleActionMemoryRequestIncreased))
		Expect(result.shouldPatch).To(BeTrue())
		Expect(result.previousMemory).To(Equal("128Mi"))
		Expect(result.newMemory).To(Equal("160Mi"))
	})

	It("respects the memory max cap", func() {
		container := testContainer("api", "100m", "1Gi")

		result, err := calculateMemoryScaleResult(container, policy)

		Expect(err).NotTo(HaveOccurred())
		Expect(result.action).To(Equal(scaleActionMaxLimitReached))
		Expect(result.shouldPatch).To(BeFalse())
		Expect(result.newQuantity.Cmp(resource.MustParse(policy.MaxMemoryRequest))).To(Equal(0))
	})
})

var _ = Describe("StarvationEvent Controller", func() {
	const namespace = "default"

	ctx := context.Background()

	AfterEach(func() {
		deleteStarvationEventIfExists(ctx, types.NamespacedName{Name: "test-cpu-starvation", Namespace: namespace})
		deleteStarvationEventIfExists(ctx, types.NamespacedName{Name: "missing-deployment-starvation", Namespace: namespace})
		deleteStarvationEventIfExists(ctx, types.NamespacedName{Name: "missing-container-starvation", Namespace: namespace})
		deleteDeploymentIfExists(ctx, types.NamespacedName{Name: "sample-api", Namespace: namespace})
		deleteDeploymentIfExists(ctx, types.NamespacedName{Name: "sample-api-missing-container", Namespace: namespace})
		deleteBeaconPolicyIfExists(ctx, types.NamespacedName{Name: "test-policy", Namespace: namespace})
	})

	It("patches Deployment CPU request and records status", func() {
		policyKey := types.NamespacedName{Name: "test-policy", Namespace: namespace}
		deploymentKey := types.NamespacedName{Name: "sample-api", Namespace: namespace}
		eventKey := types.NamespacedName{Name: "test-cpu-starvation", Namespace: namespace}
		observedAt := metav1.NewTime(time.Now().Add(-2 * time.Second).Truncate(time.Second))

		Expect(k8sClient.Create(ctx, testBeaconPolicy(policyKey, deploymentKey.Name))).To(Succeed())
		Expect(k8sClient.Create(ctx, testDeployment(deploymentKey, "api", "100m", "128Mi"))).To(Succeed())
		Expect(k8sClient.Create(ctx, testStarvationEvent(eventKey, policyKey.Name, deploymentKey.Name, "api", autoscalingv1alpha1.StarvationSignalCPUStarvation, observedAt))).To(Succeed())

		controllerReconciler := &StarvationEventReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: eventKey})
		Expect(err).NotTo(HaveOccurred())
		Expect(result).To(Equal(reconcile.Result{}))

		updatedDeployment := &appsv1.Deployment{}
		Expect(k8sClient.Get(ctx, deploymentKey, updatedDeployment)).To(Succeed())
		container := updatedDeployment.Spec.Template.Spec.Containers[0]
		cpuRequest := container.Resources.Requests[corev1.ResourceCPU]
		Expect(cpuRequest.String()).To(Equal("125m"))

		updatedEvent := &autoscalingv1alpha1.StarvationEvent{}
		Expect(k8sClient.Get(ctx, eventKey, updatedEvent)).To(Succeed())
		Expect(updatedEvent.Status.Processed).To(BeTrue())
		Expect(updatedEvent.Status.RoutedPolicy).To(Equal(policyKey.Name))
		Expect(updatedEvent.Status.ReactionLatencyMillis).To(BeNumerically(">=", int64(0)))
		processed := meta.FindStatusCondition(updatedEvent.Status.Conditions, autoscalingv1alpha1.StarvationEventProcessedCondition)
		Expect(processed).NotTo(BeNil())
		Expect(processed.Status).To(Equal(metav1.ConditionTrue))
		Expect(processed.Reason).To(Equal(reasonVerticalScalePatchApplied))

		updatedPolicy := &autoscalingv1alpha1.BeaconPolicy{}
		Expect(k8sClient.Get(ctx, policyKey, updatedPolicy)).To(Succeed())
		Expect(updatedPolicy.Status.LastDecision).To(Equal(decisionVerticalScalePatchApplied))
		Expect(updatedPolicy.Status.LastEventName).To(Equal(eventKey.Name))
		Expect(updatedPolicy.Status.LastSignalType).To(Equal(autoscalingv1alpha1.StarvationSignalCPUStarvation))
		Expect(updatedPolicy.Status.LastEventSeverity).To(Equal(autoscalingv1alpha1.StarvationSeverityHigh))
		Expect(updatedPolicy.Status.LastScaledAt).NotTo(BeNil())
		Expect(updatedPolicy.Status.LastPatchedDeployment).To(Equal(deploymentKey.Name))
		Expect(updatedPolicy.Status.LastPatchedContainer).To(Equal("api"))
		Expect(updatedPolicy.Status.PreviousCPURequest).To(Equal("100m"))
		Expect(updatedPolicy.Status.NewCPURequest).To(Equal("125m"))
		Expect(updatedPolicy.Status.ScaleAction).To(Equal(scaleActionCPURequestIncreased))
		ready := meta.FindStatusCondition(updatedPolicy.Status.Conditions, autoscalingv1alpha1.BeaconPolicyReadyCondition)
		Expect(ready).NotTo(BeNil())
		Expect(ready.Status).To(Equal(metav1.ConditionTrue))
		Expect(ready.Reason).To(Equal(reasonVerticalScalePatchApplied))
	})

	It("sets TargetDeploymentNotFound when the target Deployment is missing", func() {
		policyKey := types.NamespacedName{Name: "test-policy", Namespace: namespace}
		eventKey := types.NamespacedName{Name: "missing-deployment-starvation", Namespace: namespace}
		observedAt := metav1.NewTime(time.Now().Add(-2 * time.Second).Truncate(time.Second))

		Expect(k8sClient.Create(ctx, testBeaconPolicy(policyKey, "missing-api"))).To(Succeed())
		Expect(k8sClient.Create(ctx, testStarvationEvent(eventKey, policyKey.Name, "missing-api", "api", autoscalingv1alpha1.StarvationSignalCPUStarvation, observedAt))).To(Succeed())

		controllerReconciler := &StarvationEventReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: eventKey})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(targetMissingRequeue))

		updatedEvent := &autoscalingv1alpha1.StarvationEvent{}
		Expect(k8sClient.Get(ctx, eventKey, updatedEvent)).To(Succeed())
		Expect(updatedEvent.Status.Processed).To(BeFalse())
		processed := meta.FindStatusCondition(updatedEvent.Status.Conditions, autoscalingv1alpha1.StarvationEventProcessedCondition)
		Expect(processed).NotTo(BeNil())
		Expect(processed.Status).To(Equal(metav1.ConditionFalse))
		Expect(processed.Reason).To(Equal(reasonTargetDeploymentNotFound))

		updatedPolicy := &autoscalingv1alpha1.BeaconPolicy{}
		Expect(k8sClient.Get(ctx, policyKey, updatedPolicy)).To(Succeed())
		Expect(updatedPolicy.Status.ScaleAction).To(Equal(scaleActionTargetDeploymentNotFound))
	})

	It("sets TargetContainerNotFound when the target container is missing", func() {
		policyKey := types.NamespacedName{Name: "test-policy", Namespace: namespace}
		deploymentKey := types.NamespacedName{Name: "sample-api-missing-container", Namespace: namespace}
		eventKey := types.NamespacedName{Name: "missing-container-starvation", Namespace: namespace}
		observedAt := metav1.NewTime(time.Now().Add(-2 * time.Second).Truncate(time.Second))

		Expect(k8sClient.Create(ctx, testBeaconPolicy(policyKey, deploymentKey.Name))).To(Succeed())
		Expect(k8sClient.Create(ctx, testDeployment(deploymentKey, "worker", "100m", "128Mi"))).To(Succeed())
		Expect(k8sClient.Create(ctx, testStarvationEvent(eventKey, policyKey.Name, deploymentKey.Name, "api", autoscalingv1alpha1.StarvationSignalCPUStarvation, observedAt))).To(Succeed())

		controllerReconciler := &StarvationEventReconciler{
			Client: k8sClient,
			Scheme: k8sClient.Scheme(),
		}

		result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{NamespacedName: eventKey})
		Expect(err).NotTo(HaveOccurred())
		Expect(result.RequeueAfter).To(Equal(targetMissingRequeue))

		updatedEvent := &autoscalingv1alpha1.StarvationEvent{}
		Expect(k8sClient.Get(ctx, eventKey, updatedEvent)).To(Succeed())
		Expect(updatedEvent.Status.Processed).To(BeFalse())
		processed := meta.FindStatusCondition(updatedEvent.Status.Conditions, autoscalingv1alpha1.StarvationEventProcessedCondition)
		Expect(processed).NotTo(BeNil())
		Expect(processed.Status).To(Equal(metav1.ConditionFalse))
		Expect(processed.Reason).To(Equal(reasonTargetContainerNotFound))

		updatedPolicy := &autoscalingv1alpha1.BeaconPolicy{}
		Expect(k8sClient.Get(ctx, policyKey, updatedPolicy)).To(Succeed())
		Expect(updatedPolicy.Status.ScaleAction).To(Equal(scaleActionTargetContainerNotFound))
	})
})

func testBeaconPolicy(key types.NamespacedName, deploymentName string) *autoscalingv1alpha1.BeaconPolicy {
	return &autoscalingv1alpha1.BeaconPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: autoscalingv1alpha1.BeaconPolicySpec{
			TargetRef: autoscalingv1alpha1.BeaconPolicyTargetRef{
				APIVersion: "apps/v1",
				Kind:       "Deployment",
				Name:       deploymentName,
			},
			MinCPURequest:               "100m",
			MaxCPURequest:               "1000m",
			MinMemoryRequest:            "128Mi",
			MaxMemoryRequest:            "1Gi",
			ScaleUpStepPercent:          25,
			StarvationWindowSeconds:     10,
			ReactionLatencyBudgetMillis: 4000,
		},
	}
}

func testStarvationEvent(
	key types.NamespacedName,
	policyName string,
	deploymentName string,
	containerName string,
	signalType string,
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
				Name:       deploymentName,
			},
			ContainerName: containerName,
			SignalType:    signalType,
			ObservedAt:    observedAt,
			Severity:      autoscalingv1alpha1.StarvationSeverityHigh,
			Message:       "Synthetic starvation event for tests.",
		},
	}
}

func testDeployment(key types.NamespacedName, containerName string, cpuRequest string, memoryRequest string) *appsv1.Deployment {
	labels := map[string]string{"app": key.Name}
	replicas := int32(1)
	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      key.Name,
			Namespace: key.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{MatchLabels: labels},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						testContainer(containerName, cpuRequest, memoryRequest),
					},
				},
			},
		},
	}
}

func testContainer(name string, cpuRequest string, memoryRequest string) corev1.Container {
	requests := corev1.ResourceList{}
	if cpuRequest != "" {
		requests[corev1.ResourceCPU] = resource.MustParse(cpuRequest)
	}
	if memoryRequest != "" {
		requests[corev1.ResourceMemory] = resource.MustParse(memoryRequest)
	}

	return corev1.Container{
		Name:  name,
		Image: "nginx:stable",
		Resources: corev1.ResourceRequirements{
			Requests: requests,
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("1000m"),
				corev1.ResourceMemory: resource.MustParse("1Gi"),
			},
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

func deleteDeploymentIfExists(ctx context.Context, key types.NamespacedName) {
	resource := &appsv1.Deployment{}
	err := k8sClient.Get(ctx, key, resource)
	if errors.IsNotFound(err) {
		return
	}
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
}
