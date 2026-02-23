FROM mcr.microsoft.com/devcontainers/go:1.25

# ---------- versions (keep in sync with Makefile) ----------
ARG KUSTOMIZE_VERSION=v5.7.1
ARG CONTROLLER_TOOLS_VERSION=v0.20.0
ARG GOLANGCI_LINT_VERSION=v2.7.2
ARG HELM_DOCS_VERSION=v1.14.2

# ---------- system packages ----------
RUN apt-get update && apt-get install -y --no-install-recommends \
        openssl \
    && rm -rf /var/lib/apt/lists/*

# ---------- system binaries (root) ----------
ARG TARGETARCH
RUN ARCH=$(dpkg --print-architecture) \
    # kubectl
    && curl -fsSL "https://dl.k8s.io/release/$(curl -fsSL https://dl.k8s.io/release/stable.txt)/bin/linux/${ARCH}/kubectl" \
       -o /usr/local/bin/kubectl && chmod +x /usr/local/bin/kubectl \
    # kind
    && curl -fsSL "https://kind.sigs.k8s.io/dl/latest/kind-linux-${ARCH}" \
       -o /usr/local/bin/kind && chmod +x /usr/local/bin/kind \
    # helm
    && curl -fsSL https://raw.githubusercontent.com/helm/helm/main/scripts/get-helm-3 | bash

# ---------- Go tools (vscode user) ----------
USER vscode
ENV GOPATH=/home/vscode/go
ENV PATH="${GOPATH}/bin:${PATH}"

RUN go install sigs.k8s.io/kustomize/kustomize/v5@${KUSTOMIZE_VERSION} \
    && go install sigs.k8s.io/controller-tools/cmd/controller-gen@${CONTROLLER_TOOLS_VERSION} \
    && go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@${GOLANGCI_LINT_VERSION} \
    && go install github.com/norwoodj/helm-docs/cmd/helm-docs@${HELM_DOCS_VERSION} \
    && go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest \
    && go clean -cache -modcache

USER root
