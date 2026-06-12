locals {
  labels = merge({
    "app.kubernetes.io/name"       = "beacon"
    "app.kubernetes.io/component"  = "agent"
    "app.kubernetes.io/managed-by" = "terraform"
  }, var.labels)
}

resource "kubernetes_service_account_v1" "agent" {
  metadata {
    name      = "beacon-agent"
    namespace = var.namespace
    labels    = local.labels
  }
}

resource "kubernetes_cluster_role_v1" "agent" {
  metadata {
    name   = "beacon-agent-starvationevent-writer"
    labels = local.labels
  }

  rule {
    api_groups = ["autoscaling.beacon.dev"]
    resources  = ["starvationevents"]
    verbs      = ["create", "get"]
  }
}

resource "kubernetes_cluster_role_binding_v1" "agent" {
  metadata {
    name   = "beacon-agent-starvationevent-writer"
    labels = local.labels
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role_v1.agent.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account_v1.agent.metadata[0].name
    namespace = var.namespace
  }
}

resource "kubernetes_daemon_set_v1" "agent" {
  metadata {
    name      = "beacon-agent"
    namespace = var.namespace
    labels    = local.labels
  }

  spec {
    selector {
      match_labels = {
        "app.kubernetes.io/name"      = "beacon"
        "app.kubernetes.io/component" = "agent"
      }
    }

    template {
      metadata {
        labels = local.labels
      }

      spec {
        service_account_name = kubernetes_service_account_v1.agent.metadata[0].name

        node_selector = {
          "kubernetes.io/os" = "linux"
        }

        toleration {
          operator = "Exists"
        }

        container {
          name              = "beacon-agent"
          image             = var.agent_image
          image_pull_policy = "Always"

          args = [
            "--mode",
            "cgroup-psi",
            "--interval",
            var.interval,
            "--namespace",
            var.target_namespace,
            "--policy",
            var.policy_name,
            "--deployment",
            var.deployment_name,
            "--container",
            var.container_name
          ]

          env {
            name  = "BEACON_CGROUP_PSI_CPU_PATH"
            value = "/host/sys/fs/cgroup/cpu.pressure"
          }

          env {
            name  = "BEACON_CGROUP_PSI_MEMORY_PATH"
            value = "/host/sys/fs/cgroup/memory.pressure"
          }

          env {
            name  = "BEACON_CGROUP_PSI_THRESHOLD_AVG10"
            value = var.psi_threshold_avg10
          }

          volume_mount {
            name       = "host-cgroup"
            mount_path = "/host/sys/fs/cgroup"
            read_only  = true
          }

          security_context {
            allow_privilege_escalation = false
            read_only_root_filesystem  = true

            capabilities {
              drop = ["ALL"]
            }
          }

          resources {
            requests = {
              cpu    = "10m"
              memory = "64Mi"
            }
            limits = {
              cpu    = "100m"
              memory = "128Mi"
            }
          }
        }

        volume {
          name = "host-cgroup"

          host_path {
            path = "/sys/fs/cgroup"
            type = "Directory"
          }
        }
      }
    }
  }
}
