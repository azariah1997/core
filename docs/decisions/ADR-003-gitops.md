# ADR-003: GitOps
Status: Accepted

GitHub is the source of truth. Application and infrastructure changes flow through pull requests. Argo CD reconciles Kubernetes deployment state from Git. Production must not depend on undocumented manual console changes.
