# function-aicr's Makefile holds the end-to-end suite's cluster (`make e2e`,
# "Testing" in AGENTS.md): a kind cluster with Crossplane, provider-helm and
# provider-kubernetes pointed at the cluster itself, an OCI registry, and
# this tree's function built and installed as a Function package. The suite
# under test/e2e runs against it.
#
# The crossplane build submodule (build/) provides the make library:
# common.mk's pipelines and variables, k8s_tools.mk's pinned kind, kubectl,
# helm and crossplane CLI (installed under .cache/tools on first use),
# controlplane.mk's kind + Crossplane cluster at CROSSPLANE_VERSION, and
# local.xpkg.mk's package cache sync, which installs a locally built package
# with packagePullPolicy: Never. What the library lacks for a Function
# package (xpkg.build embeds the runtime image only for a Provider, and there
# is no local.xpkg.deploy.function.%) is written below, in the library's
# shape.
#
# Everything else in the repository is plain Go tooling (go build, go test,
# go generate, golangci-lint); none of it goes through make.
PROJECT_NAME := function-aicr
PROJECT_REPO := github.com/jonasz-lasut/$(PROJECT_NAME)
# The runtime image's platforms; the suite builds the host's only.
PLATFORMS ?= linux_amd64 linux_arm64

ifeq (,$(wildcard build/makelib/common.mk))
$(error the crossplane build submodule is missing: run git submodule update --init)
endif

# The cluster the suite provisions and what it installs there. All are
# overridable from the environment. Renovate moves the defaults through the
# `# renovate:` comments.
KIND_CLUSTER_NAME ?= function-aicr-e2e
# The chart version, so without the release tag's `v`.
# renovate: datasource=github-releases depName=crossplane/crossplane extractVersion=^v(?<version>.*)$
CROSSPLANE_VERSION ?= 2.4.3
# renovate: datasource=github-releases depName=crossplane-contrib/provider-helm
PROVIDER_HELM_VERSION ?= v1.5.0
# renovate: datasource=github-releases depName=crossplane-contrib/provider-kubernetes
PROVIDER_KUBERNETES_VERSION ?= v1.4.1
# renovate: datasource=github-releases depName=crossplane-contrib/function-auto-ready
FUNCTION_AUTO_READY_VERSION ?= v0.7.3
# renovate: datasource=docker depName=registry
REGISTRY_VERSION ?= 3.0.0

include build/makelib/common.mk
include build/makelib/k8s_tools.mk
include build/makelib/controlplane.mk
# local.xpkg.mk reads the package from XPKG_OUTPUT_DIR/linux_<arch>/, where
# xpkg.mk would have written it; the Function package target below does.
XPKG_OUTPUT_DIR := $(OUTPUT_DIR)/xpkg
include build/makelib/local.xpkg.mk

# The end-to-end suite
#
# make e2e           up, the suite, down (the library's e2e pipeline:
#                    e2e.init, e2e.run, e2e.done)
# make e2e.up        kind + Crossplane (controlplane.up, then e2e.crossplane
#                    waits for its deployment and the package CRDs), the
#                    suite's setup (e2e.setup: the registry behind TLS from a
#                    CA made here, provider-helm and provider-kubernetes with
#                    a ClusterProviderConfig for the cluster itself,
#                    function-auto-ready, function-aicr's runtime config),
#                    then function-aicr's image and package installed from
#                    this tree (e2e.function)
# make e2e.function  rebuild the image and the package and reinstall them:
#                    the inner loop after a code change
# make e2e.test      the suite under test/e2e against the cluster
# make e2e.down      delete the cluster
# make e2e.dump      the cluster's state for a failed run, the function pod
#                    described and logged first
E2E_IMAGE := function-aicr-e2e:dev
E2E_DIR := $(OUTPUT_DIR)/e2e
E2E_CERTS := $(E2E_DIR)/certs
E2E_NAMESPACE := e2e
E2E_MANIFESTS := test/e2e/manifests/setup
# The registry's name inside the cluster; its certificate names it.
E2E_REGISTRY_HOST := registry.$(E2E_NAMESPACE).svc
E2E_REGISTRY := $(E2E_REGISTRY_HOST):5443
FUNCTION_RUNTIME_CONFIG := function-aicr-e2e
# The flags that point crossplane's e2e environment at a cluster it does not
# own: the suite neither creates nor destroys it and installs no Crossplane.
E2E_TEST_FLAGS ?= -v -count=1 -timeout=30m
E2E_ENV_FLAGS := --kind-cluster-name=$(KIND_CLUSTER_NAME) --create-kind-cluster=false --destroy-kind-cluster=false --preinstall-crossplane=false --load-images-kind-cluster=false

.PHONY: e2e.up e2e.crossplane e2e.setup e2e.certs e2e.image e2e.function e2e.test e2e.down e2e.dump xpkg.build.function-aicr

e2e.init: e2e.up
e2e.run: e2e.test
e2e.done: e2e.down

e2e.up: controlplane.up e2e.crossplane
	@$(MAKE) e2e.setup
	@$(MAKE) e2e.function
	@$(OK) the end-to-end cluster is up: make e2e.test runs the suite

# controlplane.up returns as soon as the chart is installed; Crossplane's
# init container registers the package CRDs after that.
e2e.crossplane: $(KUBECTL)
	@$(INFO) waiting for crossplane and its package api
	@$(KUBECTL) wait --namespace $(CROSSPLANE_NAMESPACE) --for=condition=Available --timeout=300s deployment/crossplane || $(FAIL)
	@$(KUBECTL) wait --for=condition=Established --timeout=120s crd/providers.pkg.crossplane.io crd/functions.pkg.crossplane.io crd/deploymentruntimeconfigs.pkg.crossplane.io || $(FAIL)
	@$(OK) waiting for crossplane and its package api

# A CA and the registry's serving certificate, made once per output
# directory. The function trusts the CA through SSL_CERT_FILE; AICR's recipe
# pull speaks HTTPS only.
e2e.certs: $(E2E_CERTS)/tls.crt
$(E2E_CERTS)/tls.crt:
	@$(INFO) creating the registry CA and certificate
	@mkdir -p $(E2E_CERTS)
	@openssl req -x509 -newkey rsa:2048 -nodes -days 30 -subj "/CN=function-aicr-e2e" \
		-addext "basicConstraints=critical,CA:TRUE" -addext "keyUsage=critical,keyCertSign,cRLSign" \
		-keyout $(E2E_CERTS)/ca.key -out $(E2E_CERTS)/ca.crt 2>/dev/null || $(FAIL)
	@openssl req -newkey rsa:2048 -nodes -subj "/CN=$(E2E_REGISTRY_HOST)" \
		-keyout $(E2E_CERTS)/tls.key -out $(E2E_CERTS)/tls.csr 2>/dev/null || $(FAIL)
	@printf 'subjectAltName=DNS:%s,DNS:%s.cluster.local,DNS:localhost,IP:127.0.0.1\nextendedKeyUsage=serverAuth\n' "$(E2E_REGISTRY_HOST)" "$(E2E_REGISTRY_HOST)" > $(E2E_CERTS)/tls.ext
	@openssl x509 -req -days 30 -in $(E2E_CERTS)/tls.csr -CA $(E2E_CERTS)/ca.crt -CAkey $(E2E_CERTS)/ca.key -CAcreateserial \
		-extfile $(E2E_CERTS)/tls.ext -out $(E2E_CERTS)/tls.crt 2>/dev/null || $(FAIL)
	@$(OK) creating the registry CA and certificate

# The setup is applied in two steps: the packages and the registry first,
# then, once the providers' CRDs exist, the provider configs.
e2e.setup: $(KUBECTL) e2e.certs
	@$(INFO) setting up the registry, the providers and function-auto-ready
	@$(KUBECTL) create namespace $(E2E_NAMESPACE) --dry-run=client -o yaml | $(KUBECTL) apply -f - || $(FAIL)
	@$(KUBECTL) -n $(E2E_NAMESPACE) create secret tls registry-tls --cert=$(E2E_CERTS)/tls.crt --key=$(E2E_CERTS)/tls.key --dry-run=client -o yaml | $(KUBECTL) apply -f - || $(FAIL)
	@$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) create configmap e2e-registry-ca --from-file=ca.crt=$(E2E_CERTS)/ca.crt --dry-run=client -o yaml | $(KUBECTL) apply -f - || $(FAIL)
	@sed -e 's|REGISTRY_VERSION|$(REGISTRY_VERSION)|' $(E2E_MANIFESTS)/registry.yaml | $(KUBECTL) apply -f - || $(FAIL)
	@sed -e 's|PROVIDER_HELM_VERSION|$(PROVIDER_HELM_VERSION)|' -e 's|PROVIDER_KUBERNETES_VERSION|$(PROVIDER_KUBERNETES_VERSION)|' \
		-e 's|FUNCTION_AUTO_READY_VERSION|$(FUNCTION_AUTO_READY_VERSION)|' $(E2E_MANIFESTS)/packages.yaml | $(KUBECTL) apply -f - || $(FAIL)
	@$(KUBECTL) apply -f $(E2E_MANIFESTS)/function-runtime.yaml || $(FAIL)
	@$(KUBECTL) -n $(E2E_NAMESPACE) rollout status deployment/registry --timeout=180s || $(FAIL)
	@$(KUBECTL) wait provider.pkg.crossplane.io/provider-helm provider.pkg.crossplane.io/provider-kubernetes --for=condition=Healthy --timeout=300s || $(FAIL)
	@$(KUBECTL) wait function.pkg.crossplane.io/function-auto-ready --for=condition=Healthy --timeout=300s || $(FAIL)
	@$(KUBECTL) apply -f $(E2E_MANIFESTS)/provider-configs.yaml || $(FAIL)
	@$(OK) setting up the registry, the providers and function-auto-ready

# The runtime image from this tree, built for the host's architecture.
e2e.image:
	@$(INFO) building the runtime image $(E2E_IMAGE) for linux/$(TARGETARCH)
	@docker buildx build --load --platform linux/$(TARGETARCH) -t $(E2E_IMAGE) . || $(FAIL)
	@$(OK) building the runtime image $(E2E_IMAGE)

# The Function package from the image: what xpkg.mk's xpkg.build.% would do
# for a Provider.
xpkg.build.function-aicr: $(CROSSPLANE_CLI) e2e.image
	@$(INFO) building package function-aicr-$(VERSION).xpkg
	@rm -rf $(XPKG_OUTPUT_DIR)/linux_$(TARGETARCH) && mkdir -p $(XPKG_OUTPUT_DIR)/linux_$(TARGETARCH)
	@$(CROSSPLANE_CLI) xpkg build --package-root package --embed-runtime-image $(E2E_IMAGE) \
		--package-file $(XPKG_OUTPUT_DIR)/linux_$(TARGETARCH)/function-aicr-$(VERSION).xpkg || $(FAIL)
	@$(OK) building package function-aicr-$(VERSION).xpkg

# local.xpkg.deploy.provider.%'s shape for a Function: the runtime image
# loaded into the kind node and the Function installed from the synced cache
# with packagePullPolicy Never, on the suite's runtime config.
local.xpkg.deploy.function.%: $(KIND) $(KUBECTL) local.xpkg.sync
	@$(INFO) deploying function package $* $(VERSION)
	@$(KIND) load docker-image $(E2E_IMAGE) -n $(KIND_CLUSTER_NAME) || $(FAIL)
	@echo '{"apiVersion":"pkg.crossplane.io/v1","kind":"Function","metadata":{"name":"$*"},"spec":{"package":"xpkg.crossplane.internal/dev/$*@$(LOCAL_XPKG_DIGEST)","skipDependencyResolution":true,"packagePullPolicy":"Never","runtimeConfigRef":{"name":"$(FUNCTION_RUNTIME_CONFIG)"}}}' | $(KUBECTL) apply -f - || $(FAIL)
	@$(OK) deploying function package $* $(VERSION)

# The pod is restarted so that a rebuilt image under the same tag is the one
# that runs.
e2e.function: xpkg.build.function-aicr local.xpkg.deploy.function.function-aicr
	@$(INFO) waiting for function-aicr to become healthy
	@$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) delete pod -l pkg.crossplane.io/function=function-aicr --ignore-not-found --wait=false >/dev/null || $(FAIL)
	@$(KUBECTL) wait function.pkg.crossplane.io/function-aicr --for=condition=Healthy --timeout=300s || $(FAIL)
	@$(OK) waiting for function-aicr to become healthy

e2e.test:
	@$(INFO) running the end-to-end suite
	@cd test/e2e && E2E_REGISTRY=$(E2E_REGISTRY) E2E_REGISTRY_CA=$(abspath $(E2E_CERTS)/ca.crt) \
		go test $(E2E_TEST_FLAGS) ./... -args $(E2E_ENV_FLAGS) || $(FAIL)
	@$(OK) running the end-to-end suite

e2e.down: controlplane.down

e2e.dump: $(KUBECTL)
	@$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) describe pod -l pkg.crossplane.io/function=function-aicr || true
	@$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) logs -l pkg.crossplane.io/function=function-aicr --tail=-1 || true
	@$(MAKE) controlplane.dump
