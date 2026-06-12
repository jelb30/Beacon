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

package main

import (
	"context"
	"flag"
	"os"
	"time"

	"github.com/go-logr/logr"
	_ "k8s.io/client-go/plugin/pkg/client/auth"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	autoscalingv1alpha1 "github.com/jelb30/Beacon/api/v1alpha1"
	"github.com/jelb30/Beacon/internal/agent"
)

func main() {
	var namespace string
	var policyName string
	var deploymentName string
	var containerName string
	var signalType string
	var severity string
	var mode string
	var interval time.Duration
	var once bool
	var kubeconfig string

	flag.StringVar(&namespace, "namespace", "default", "Namespace where StarvationEvent resources are created.")
	flag.StringVar(&policyName, "policy", "sample-api-policy", "BeaconPolicy name to route detections to.")
	flag.StringVar(&deploymentName, "deployment", "sample-api", "Target Deployment name.")
	flag.StringVar(&containerName, "container", "api", "Target container name.")
	flag.StringVar(&signalType, "signal", autoscalingv1alpha1.StarvationSignalCPUStarvation, "Starvation signal type.")
	flag.StringVar(&severity, "severity", autoscalingv1alpha1.StarvationSeverityHigh, "Starvation severity.")
	flag.StringVar(&mode, "mode", "synthetic", "Detector mode: synthetic, cgroup-psi, or ebpf.")
	flag.DurationVar(&interval, "interval", 5*time.Second, "Detection interval when --once=false.")
	flag.BoolVar(&once, "once", false, "Emit one StarvationEvent and exit.")
	if flag.Lookup("kubeconfig") == nil {
		flag.StringVar(&kubeconfig, "kubeconfig", "", "Optional kubeconfig path.")
	}

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()
	if kubeconfigFlag := flag.Lookup("kubeconfig"); kubeconfigFlag != nil {
		kubeconfig = kubeconfigFlag.Value.String()
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	log := ctrl.Log.WithName("beacon-agent")

	detector, err := agent.NewDetector(mode, agent.DetectorConfig{
		Namespace:     namespace,
		PolicyName:    policyName,
		Deployment:    deploymentName,
		ContainerName: containerName,
		SignalType:    signalType,
		Severity:      severity,
	})
	if err != nil {
		log.Error(err, "Failed to initialize detector", "mode", mode)
		os.Exit(1)
	}

	scheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(autoscalingv1alpha1.AddToScheme(scheme))

	restConfig, err := buildConfig(kubeconfig)
	if err != nil {
		log.Error(err, "Failed to build Kubernetes client config")
		os.Exit(1)
	}

	k8sClient, err := client.New(restConfig, client.Options{Scheme: scheme})
	if err != nil {
		log.Error(err, "Failed to create Kubernetes client")
		os.Exit(1)
	}

	ctx := ctrl.SetupSignalHandler()
	log.Info("Starting beacon-agent",
		"mode", detector.Name(),
		"namespace", namespace,
		"policy", policyName,
		"deployment", deploymentName,
		"container", containerName,
		"signal", signalType,
		"severity", severity,
		"once", once,
		"interval", interval.String())

	if once {
		runner := agentRunner{
			detector:  detector,
			k8sClient: k8sClient,
			log:       log,
		}
		if err := runner.detectAndCreate(ctx); err != nil {
			log.Error(err, "Failed to create StarvationEvent")
			os.Exit(1)
		}
		return
	}

	runner := agentRunner{
		detector:  detector,
		k8sClient: k8sClient,
		log:       log,
	}
	if err := runner.run(ctx, interval); err != nil {
		log.Error(err, "beacon-agent stopped with an error")
		os.Exit(1)
	}
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}

	return ctrl.GetConfig()
}

type agentRunner struct {
	detector  agent.Detector
	k8sClient client.Client
	log       logr.Logger
}

func (r agentRunner) run(ctx context.Context, interval time.Duration) error {
	if err := r.detectAndCreate(ctx); err != nil {
		return err
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			if err := r.detectAndCreate(ctx); err != nil {
				return err
			}
		}
	}
}

func (r agentRunner) detectAndCreate(ctx context.Context) error {
	detection, err := r.detector.Detect(ctx)
	if err != nil {
		return err
	}
	if detection == nil {
		r.log.Info("Detector returned no starvation signal", "detector", r.detector.Name())
		return nil
	}

	if err := agent.CreateStarvationEvent(ctx, r.k8sClient, *detection); err != nil {
		return err
	}

	r.log.Info("Created StarvationEvent",
		"detector", r.detector.Name(),
		"namespace", detection.Namespace,
		"policy", detection.PolicyName,
		"target", detection.TargetRef.Name,
		"container", detection.ContainerName,
		"signal", detection.SignalType,
		"severity", detection.Severity,
		"observedAt", detection.ObservedAt.Format(time.RFC3339))
	return nil
}
