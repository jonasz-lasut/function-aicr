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

# The suite's profile (see "The end-to-end suite" below). Each has a cluster
# and a kubeconfig of its own.
E2E_PROFILE ?= catalog
ifeq (,$(filter $(E2E_PROFILE),catalog kwok))
$(error E2E_PROFILE is $(E2E_PROFILE): it must be catalog or kwok)
endif

# The cluster the suite provisions and what it installs there. All are
# overridable from the environment. Renovate moves the defaults through the
# `# renovate:` comments.
KIND_CLUSTER_NAME ?= function-aicr-e2e-$(E2E_PROFILE)
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
# The release without its tag's `v`, as the archive is named.
# renovate: datasource=github-releases depName=oras-project/oras extractVersion=^v(?<version>.*)$
ORAS_VERSION ?= 1.4.0
# renovate: datasource=github-releases depName=kubernetes-sigs/kwok
KWOK_VERSION ?= v0.8.0

# Crossplane's flags. A composite resource whose composed resources all
# report their status within a few seconds spends the default burst of its
# watch circuit breaker (100 events) at once. The breaker then lets one event
# through every 30 seconds for five minutes, which a feature would spend
# waiting for the next step of a rollout.
CROSSPLANE_ARGS ?= --circuit-breaker-burst=1000

include build/makelib/common.mk
include build/makelib/k8s_tools.mk
include build/makelib/controlplane.mk
# local.xpkg.mk reads the package from XPKG_OUTPUT_DIR/linux_<arch>/, where
# xpkg.mk would have written it; the Function package target below does.
XPKG_OUTPUT_DIR := $(OUTPUT_DIR)/xpkg
include build/makelib/local.xpkg.mk

# oras publishes the suite's recipe catalog; installed as k8s_tools.mk
# installs its tools.
ORAS := $(TOOLS_HOST_DIR)/oras-$(ORAS_VERSION)
$(ORAS):
	@$(INFO) installing oras $(ORAS_VERSION)
	@mkdir -p $(TOOLS_HOST_DIR)/tmp-oras
	@curl -fsSL https://github.com/oras-project/oras/releases/download/v$(ORAS_VERSION)/oras_$(ORAS_VERSION)_$(HOSTOS)_$(SAFEHOSTARCH).tar.gz | tar -xz -C $(TOOLS_HOST_DIR)/tmp-oras || $(FAIL)
	@mv $(TOOLS_HOST_DIR)/tmp-oras/oras $(ORAS)
	@rm -fr $(TOOLS_HOST_DIR)/tmp-oras
	@$(OK) installing oras $(ORAS_VERSION)

# The end-to-end suite
#
# The suite has two profiles, each on a cluster of its own (E2E_PROFILE):
#
#   catalog  the default. The function serves the suite's own small catalog
#            from the registry in the cluster, so a run is hermetic and takes
#            minutes.
#   kwok     the function serves the embedded catalog, and a stock recipe is
#            deployed onto nodes that KWOK simulates, as AICR's own KWOK lane
#            does.
#
# make e2e           up, the suite, down (the library's e2e pipeline:
#                    e2e.init, e2e.run, e2e.done)
# make e2e.up        kind + Crossplane (controlplane.up, then e2e.crossplane
#                    waits for its deployment and the package CRDs), the
#                    suite's setup (e2e.setup: the registry behind TLS from a
#                    CA made here, provider-helm and provider-kubernetes with
#                    a ClusterProviderConfig for the cluster itself,
#                    function-auto-ready), the profile's own setup
#                    (e2e.catalog or e2e.kwok), then function-aicr's image
#                    and package installed from this tree (e2e.function)
# make e2e.catalog   publish the charts and the recipe catalog under
#                    test/e2e/catalog to the registry and point function-aicr
#                    at the catalog's digest: the inner loop after a change
#                    to the catalog
# make e2e.kwok      install KWOK, create the simulated nodes and give
#                    function-aicr a runtime config without a recipe source
# make e2e.function  rebuild the image and the package and reinstall them:
#                    the inner loop after a code change
# make e2e.test      the suite under test/e2e against the cluster
# make e2e.down      delete the cluster
# make e2e.dump      the cluster's state for a failed run, the function pod
#                    described and logged first
E2E_IMAGE := function-aicr-e2e:dev
E2E_DIR := $(OUTPUT_DIR)/e2e
# The cluster's kubeconfig. Every target and the suite itself read this file
# and no other, so nothing here reaches another cluster, whatever the current
# context of the developer's own kubeconfig is. To look at the cluster:
# export KUBECONFIG=$PWD/_output/e2e/kubeconfig-catalog
export KUBECONFIG := $(abspath $(E2E_DIR)/kubeconfig-$(E2E_PROFILE))
E2E_CERTS := $(E2E_DIR)/certs
E2E_NAMESPACE := e2e
E2E_MANIFESTS := test/e2e/manifests/setup
# The registry's name inside the cluster; its certificate names it.
E2E_REGISTRY_HOST := registry.$(E2E_NAMESPACE).svc
E2E_REGISTRY := $(E2E_REGISTRY_HOST):5443
# The host's port the registry is forwarded to while the catalog is published.
E2E_REGISTRY_LOCAL_PORT ?= 5443
E2E_CATALOG := test/e2e/catalog
E2E_CATALOG_OUT := $(E2E_DIR)/catalog
# The recipe catalog's repository; the function is given its manifest digest.
E2E_RECIPE_REPOSITORY := recipes/e2e
FUNCTION_RUNTIME_CONFIG := function-aicr-e2e
KWOK_RELEASE := https://github.com/kubernetes-sigs/kwok/releases/download/$(KWOK_VERSION)
E2E_KWOK_MANIFESTS := test/e2e/manifests/kwok
# The features to run: crossplane's e2e environment selects them by suite.
E2E_TEST_SUITE_catalog := base
E2E_TEST_SUITE_kwok := kwok
# The flags that point crossplane's e2e environment at a cluster it does not
# own: the suite neither creates nor destroys it and installs no Crossplane.
E2E_TEST_FLAGS ?= -v -count=1 -timeout=30m
E2E_ENV_FLAGS := --kind-cluster-name=$(KIND_CLUSTER_NAME) --create-kind-cluster=false --destroy-kind-cluster=false --preinstall-crossplane=false --load-images-kind-cluster=false --test-suite=$(E2E_TEST_SUITE_$(E2E_PROFILE))

.PHONY: e2e.up e2e.kubeconfig e2e.crossplane e2e.setup e2e.certs e2e.catalog e2e.kwok e2e.image e2e.function e2e.test e2e.down e2e.dump xpkg.build.function-aicr

e2e.init: e2e.up
e2e.run: e2e.test
e2e.done: e2e.down

e2e.up: e2e.kubeconfig controlplane.up e2e.crossplane
	@$(MAKE) e2e.setup
	@$(MAKE) e2e.$(E2E_PROFILE)
	@$(MAKE) e2e.function
	@$(OK) the end-to-end cluster is up: make e2e.test runs the suite

# A cluster that outlived its kubeconfig, as after `rm -rf _output`, gets it
# back; controlplane.up selects the context and would fail without it.
e2e.kubeconfig: $(KIND)
	@mkdir -p $(E2E_DIR)
	@$(KIND) export kubeconfig --name $(KIND_CLUSTER_NAME) >/dev/null 2>&1 || true

# controlplane.up returns as soon as the chart is installed; Crossplane's
# init container registers the package CRDs after that.
e2e.crossplane: $(KUBECTL)
	@$(INFO) waiting for crossplane and its package api
	@$(KUBECTL) wait --namespace $(CROSSPLANE_NAMESPACE) --for=condition=Available --timeout=300s deployment/crossplane || $(FAIL)
	@$(KUBECTL) wait --for=condition=Established --timeout=120s crd/providers.pkg.crossplane.io crd/functions.pkg.crossplane.io crd/deploymentruntimeconfigs.pkg.crossplane.io || $(FAIL)
	@$(OK) waiting for crossplane and its package api

# A CA and the registry's serving certificate, made once per output
# directory. The function and provider-helm trust the CA through the
# environment; AICR's recipe pull speaks HTTPS only. The certificate also
# names localhost, where the registry is forwarded to for a push.
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
	@$(KUBECTL) -n $(E2E_NAMESPACE) rollout status deployment/registry --timeout=180s || $(FAIL)
	@$(KUBECTL) wait provider.pkg.crossplane.io/provider-helm provider.pkg.crossplane.io/provider-kubernetes --for=condition=Healthy --timeout=300s || $(FAIL)
	@$(KUBECTL) wait function.pkg.crossplane.io/function-auto-ready --for=condition=Healthy --timeout=300s || $(FAIL)
	@$(KUBECTL) apply -f $(E2E_MANIFESTS)/provider-configs.yaml || $(FAIL)
	@$(OK) setting up the registry, the providers and function-auto-ready

# The suite's catalog, published the way docs/operators/recipe-source.md
# tells an operator to publish one: the charts with helm push, the recipe
# tree as one tar+gzip layer with oras. The registry is reached through a
# port-forward for the length of the recipe; oras would speak plain HTTP to
# localhost unless told not to. The function's runtime config
# is applied last, with the digest the recipe artifact got, and Crossplane
# restarts a function that is already installed on it.
e2e.catalog: $(KUBECTL) $(HELM) $(ORAS) e2e.certs
	@$(INFO) publishing the charts and the recipe catalog to $(E2E_REGISTRY)
	@rm -rf $(E2E_CATALOG_OUT) && mkdir -p $(E2E_CATALOG_OUT)
	@for chart in $(E2E_CATALOG)/charts/*/; do $(HELM) package $$chart --destination $(E2E_CATALOG_OUT) >/dev/null || exit 1; done
	@COPYFILE_DISABLE=1 tar -czf $(E2E_CATALOG_OUT)/recipe.tar.gz -C $(E2E_CATALOG)/recipe registry.yaml overlays components || $(FAIL)
	@$(KUBECTL) -n $(E2E_NAMESPACE) port-forward service/registry $(E2E_REGISTRY_LOCAL_PORT):5443 >/dev/null 2>&1 & forward=$$!; \
		trap "kill $$forward" EXIT; \
		local=localhost:$(E2E_REGISTRY_LOCAL_PORT); ca=$(abspath $(E2E_CERTS)/ca.crt); \
		for try in $$(seq 30); do curl -fsS --cacert $$ca https://$$local/v2/ >/dev/null 2>&1 && break; sleep 1; done; \
		for chart in $(E2E_CATALOG_OUT)/*.tgz; do $(HELM) push $$chart oci://$$local/charts --ca-file $$ca >/dev/null || exit 1; done; \
		cd $(E2E_CATALOG_OUT) && $(ORAS) push --plain-http=false --ca-file $$ca --artifact-type application/vnd.nvidia.aicr.artifact \
			$$local/$(E2E_RECIPE_REPOSITORY):e2e recipe.tar.gz:application/vnd.oci.image.layer.v1.tar+gzip >/dev/null && \
		$(ORAS) resolve --plain-http=false --ca-file $$ca $$local/$(E2E_RECIPE_REPOSITORY):e2e > recipe.digest || $(FAIL)
	@sed -e 's|RECIPE_OCI_REPOSITORY|$(E2E_REGISTRY)/$(E2E_RECIPE_REPOSITORY)|' -e "s|RECIPE_OCI_DIGEST|$$(cat $(E2E_CATALOG_OUT)/recipe.digest)|" \
		$(E2E_MANIFESTS)/function-runtime.yaml | $(KUBECTL) apply -f - || $(FAIL)
	@if $(KUBECTL) get function.pkg.crossplane.io/function-aicr >/dev/null 2>&1; then \
		digest=$$(cat $(E2E_CATALOG_OUT)/recipe.digest); \
		for try in $$(seq 90); do \
			$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) get pods -l pkg.crossplane.io/function=function-aicr --field-selector=status.phase=Running \
				-o jsonpath='{.items[*].spec.containers[0].args}' | grep -q "$$digest" && break; sleep 2; \
		done; \
		$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) wait pod -l pkg.crossplane.io/function=function-aicr --for=condition=Ready --timeout=300s || $(FAIL); \
	fi
	@$(OK) publishing the charts and the recipe catalog: $$(cat $(E2E_CATALOG_OUT)/recipe.digest)

# What AICR's `make kwok-cluster` and `make kwok-nodes` do to a kind cluster:
# the KWOK controller with the stages that take a pod to Running without a
# kubelet, from the KWOK release, and the simulated nodes. The real node is
# labelled as AICR's lane labels it, which keeps the EBS CSI node plugin off
# it: there the plugin would really start, and fail without the EC2 metadata
# service. The function gets its runtime config without a recipe source, so
# it serves the embedded catalog.
e2e.kwok: $(KUBECTL)
	@$(INFO) installing KWOK $(KWOK_VERSION) and the simulated nodes
	@$(KUBECTL) apply -f $(KWOK_RELEASE)/kwok.yaml >/dev/null || $(FAIL)
	@$(KUBECTL) apply -f $(KWOK_RELEASE)/stage-fast.yaml >/dev/null || $(FAIL)
	@$(KUBECTL) -n kube-system wait --for=condition=Available --timeout=180s deployment/kwok-controller || $(FAIL)
	@$(KUBECTL) apply -f $(E2E_KWOK_MANIFESTS)/nodes.yaml >/dev/null || $(FAIL)
	@$(KUBECTL) wait --for=condition=Ready --timeout=60s nodes --selector=type=kwok || $(FAIL)
	@$(KUBECTL) label node --selector='type!=kwok' eks.amazonaws.com/compute-type=hybrid --overwrite || $(FAIL)
	@sed -e '/recipe-oci/d' $(E2E_MANIFESTS)/function-runtime.yaml | $(KUBECTL) apply -f - || $(FAIL)
	@$(OK) installing KWOK $(KWOK_VERSION) and the simulated nodes

# The runtime image from this tree, built for the host's architecture with
# the Go version go.mod names, as the release workflow builds it.
e2e.image:
	@$(INFO) building the runtime image $(E2E_IMAGE) for linux/$(TARGETARCH)
	@docker buildx build --load --platform linux/$(TARGETARCH) --build-arg GO_VERSION=$$(sed -n 's/^go //p' go.mod) -t $(E2E_IMAGE) . || $(FAIL)
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
# that runs. The Function reports Healthy from its last revision's state,
# which the pod's deletion does not change at once, so the new pod is waited
# for by itself.
e2e.function: xpkg.build.function-aicr local.xpkg.deploy.function.function-aicr
	@$(INFO) waiting for function-aicr to become healthy
	@$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) delete pod -l pkg.crossplane.io/function=function-aicr --ignore-not-found >/dev/null || $(FAIL)
	@$(KUBECTL) wait function.pkg.crossplane.io/function-aicr --for=condition=Healthy --timeout=300s || $(FAIL)
	@for try in $$(seq 60); do $(KUBECTL) -n $(CROSSPLANE_NAMESPACE) get pods -l pkg.crossplane.io/function=function-aicr -o name | grep -q . && break; sleep 2; done
	@$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) wait pod -l pkg.crossplane.io/function=function-aicr --for=condition=Ready --timeout=300s || $(FAIL)
	@$(OK) waiting for function-aicr to become healthy

e2e.test:
	@$(INFO) running the end-to-end suite
	@cd test/e2e && go test $(E2E_TEST_FLAGS) ./... -args $(E2E_ENV_FLAGS) || $(FAIL)
	@$(OK) running the end-to-end suite

e2e.down: controlplane.down

e2e.dump: $(KUBECTL)
	@$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) describe pod -l pkg.crossplane.io/function=function-aicr || true
	@$(KUBECTL) -n $(CROSSPLANE_NAMESPACE) logs -l pkg.crossplane.io/function=function-aicr --tail=-1 || true
	@$(MAKE) controlplane.dump
