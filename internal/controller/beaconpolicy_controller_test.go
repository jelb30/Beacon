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

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
)

var _ = Describe("BeaconPolicy Controller", func() {
	Context("When reconciling a resource", func() {
		const resourceName = "test-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind BeaconPolicy")
			resource := &autoscalingv1alpha1.BeaconPolicy{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
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
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		AfterEach(func() {
			resource := &autoscalingv1alpha1.BeaconPolicy{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			if errors.IsNotFound(err) {
				return
			}
			Expect(err).NotTo(HaveOccurred())

			By("Cleanup the specific resource instance BeaconPolicy")
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})
		It("should update BeaconPolicy status for the Phase 1 scaffold", func() {
			By("Reconciling the created resource")
			controllerReconciler := &BeaconPolicyReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			updated := &autoscalingv1alpha1.BeaconPolicy{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, updated)).To(Succeed())
			Expect(updated.Status.ObservedGeneration).To(Equal(updated.Generation))
			Expect(updated.Status.LastReconcileTime).NotTo(BeNil())
			Expect(updated.Status.LastDecision).To(Equal(phase1Decision))
			Expect(updated.Status.LastReactionLatencyMillis).To(BeNumerically(">=", 0))

			ready := meta.FindStatusCondition(updated.Status.Conditions, autoscalingv1alpha1.BeaconPolicyReadyCondition)
			Expect(ready).NotTo(BeNil())
			Expect(ready.Status).To(Equal(metav1.ConditionTrue))
			Expect(ready.ObservedGeneration).To(Equal(updated.Generation))
			Expect(ready.Reason).To(Equal(phase1ReadyReason))
			Expect(ready.Message).To(Equal(phase1ReadyMessage))
		})
	})
})
