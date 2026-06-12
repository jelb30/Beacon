locals {
  manager_name = "${var.name_prefix}-controller-manager"
  labels = merge({
    "app.kubernetes.io/name"       = "beacon"
    "app.kubernetes.io/component"  = "operator"
    "app.kubernetes.io/managed-by" = "terraform"
    "control-plane"                = "controller-manager"
  }, var.labels)
}

resource "kubernetes_service_account_v1" "manager" {
  metadata {
    name      = local.manager_name
    namespace = var.namespace
    labels    = local.labels
  }
}

resource "kubernetes_cluster_role_v1" "manager" {
  metadata {
    name   = "${var.name_prefix}-manager-role"
    labels = local.labels
  }

  rule {
    api_groups = ["autoscaling.beacon.dev"]
    resources  = ["beaconpolicies", "starvationevents"]
    verbs      = ["create", "delete", "get", "list", "patch", "update", "watch"]
  }

  rule {
    api_groups = ["autoscaling.beacon.dev"]
    resources  = ["beaconpolicies/status", "starvationevents/status"]
    verbs      = ["get", "patch", "update"]
  }

  rule {
    api_groups = ["autoscaling.beacon.dev"]
    resources  = ["beaconpolicies/finalizers", "starvationevents/finalizers"]
    verbs      = ["update"]
  }

  rule {
    api_groups = ["apps"]
    resources  = ["deployments"]
    verbs      = ["get", "list", "patch", "update", "watch"]
  }

  rule {
    api_groups = [""]
    resources  = ["pods"]
    verbs      = ["get", "list", "watch"]
  }

  rule {
    api_groups = ["authentication.k8s.io"]
    resources  = ["tokenreviews"]
    verbs      = ["create"]
  }

  rule {
    api_groups = ["authorization.k8s.io"]
    resources  = ["subjectaccessreviews"]
    verbs      = ["create"]
  }
}

resource "kubernetes_cluster_role_binding_v1" "manager" {
  metadata {
    name   = "${var.name_prefix}-manager-rolebinding"
    labels = local.labels
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "ClusterRole"
    name      = kubernetes_cluster_role_v1.manager.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account_v1.manager.metadata[0].name
    namespace = var.namespace
  }
}

resource "kubernetes_role_v1" "leader_election" {
  metadata {
    name      = "${var.name_prefix}-leader-election-role"
    namespace = var.namespace
    labels    = local.labels
  }

  rule {
    api_groups = [""]
    resources  = ["configmaps"]
    verbs      = ["create", "delete", "get", "list", "patch", "update", "watch"]
  }

  rule {
    api_groups = ["coordination.k8s.io"]
    resources  = ["leases"]
    verbs      = ["create", "delete", "get", "list", "patch", "update", "watch"]
  }

  rule {
    api_groups = [""]
    resources  = ["events"]
    verbs      = ["create", "patch"]
  }
}

resource "kubernetes_role_binding_v1" "leader_election" {
  metadata {
    name      = "${var.name_prefix}-leader-election-rolebinding"
    namespace = var.namespace
    labels    = local.labels
  }

  role_ref {
    api_group = "rbac.authorization.k8s.io"
    kind      = "Role"
    name      = kubernetes_role_v1.leader_election.metadata[0].name
  }

  subject {
    kind      = "ServiceAccount"
    name      = kubernetes_service_account_v1.manager.metadata[0].name
    namespace = var.namespace
  }
}

resource "kubernetes_deployment_v1" "manager" {
  metadata {
    name      = local.manager_name
    namespace = var.namespace
    labels    = local.labels
  }

  spec {
    replicas = var.replicas

    selector {
      match_labels = {
        "app.kubernetes.io/name"      = "beacon"
        "app.kubernetes.io/component" = "operator"
        "control-plane"               = "controller-manager"
      }
    }

    template {
      metadata {
        labels = local.labels
      }

      spec {
        service_account_name             = kubernetes_service_account_v1.manager.metadata[0].name
        termination_grace_period_seconds = 10

        security_context {
          run_as_non_root = true

          seccomp_profile {
            type = "RuntimeDefault"
          }
        }

        container {
          name              = "manager"
          image             = var.operator_image
          image_pull_policy = "IfNotPresent"
          command           = ["/manager"]

          args = [
            "--leader-elect",
            "--health-probe-bind-address=:8081",
            "--metrics-bind-address=:8443"
          ]

          port {
            name           = "health"
            container_port = 8081
            protocol       = "TCP"
          }

          port {
            name           = "metrics"
            container_port = 8443
            protocol       = "TCP"
          }

          liveness_probe {
            http_get {
              path = "/healthz"
              port = 8081
            }
            initial_delay_seconds = 15
            period_seconds        = 20
          }

          readiness_probe {
            http_get {
              path = "/readyz"
              port = 8081
            }
            initial_delay_seconds = 5
            period_seconds        = 10
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
              cpu    = "500m"
              memory = "128Mi"
            }
          }
        }
      }
    }
  }
}

resource "kubernetes_service_v1" "metrics" {
  metadata {
    name      = "${local.manager_name}-metrics-service"
    namespace = var.namespace
    labels    = local.labels
  }

  spec {
    selector = {
      "app.kubernetes.io/name"      = "beacon"
      "app.kubernetes.io/component" = "operator"
      "control-plane"               = "controller-manager"
    }

    port {
      name        = "https"
      port        = 8443
      target_port = "metrics"
      protocol    = "TCP"
    }
  }
}
