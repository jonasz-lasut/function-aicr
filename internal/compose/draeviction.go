package compose

import (
	"fmt"
	"maps"
	"strings"

	"aicr.run/pkg/defaults"
)

// The components and value paths of AICR's DRA eviction contract. AICR's
// bundler owns the contract and keeps these unexported, so they are copied
// from injectDRAEvictionLabel and its helpers in
// aicr.run/pkg/bundler/bundler.go; TestDRAEvictionConstants fails when the
// two drift apart.
const (
	GPUOperatorComponent    = "gpu-operator"
	DRADriverComponent      = "nvidia-dra-driver-gpu"
	DRANodeLabelerComponent = "dra-node-labeler"

	draEvictionEnvName          = "NODE_LABEL_FOR_GPU_POD_EVICTION"
	draEvictionNodeSelectorPath = "kubeletPlugin.nodeSelector"
	draEvictionEnvPath          = "driver.manager.env"
	draNodeLabelerKeyPath       = "labelKey"
	draNodeLabelerValuePath     = "labelValue"
	draNodeLabelerEnabledPath   = "enabled"
)

// WireDRAEviction returns the values of gpu-operator, nvidia-dra-driver-gpu
// and dra-node-labeler with AICR's DRA eviction contract wired in, so the GPU
// Operator's Driver Manager can evict the DRA kubelet plugin ahead of an
// in-place driver upgrade:
//
//   - the DRA driver's kubelet plugin is scheduled only on nodes that carry
//     the eviction label (kubeletPlugin.nodeSelector);
//   - the GPU Operator is told the label's key (driver.manager.env);
//   - dra-node-labeler is enabled and applies that label to every GPU node.
//
// Other node selectors and environment entries are kept. The arguments are
// the values each component is deployed with, overrides included, and are not
// mutated.
func WireDRAEviction(operator, driver, labeler map[string]any) (wiredOperator, wiredDriver, wiredLabeler map[string]any, err error) {
	key, value := defaults.DRAEvictionNodeLabelKey, defaults.DRAEvictionNodeLabelValue

	wiredDriver = maps.Clone(driver)
	if wiredDriver == nil {
		wiredDriver = make(map[string]any)
	}
	nodeSelector, err := objectAt(wiredDriver, DRADriverComponent, draEvictionNodeSelectorPath)
	if err != nil {
		return nil, nil, nil, err
	}
	nodeSelector[key] = value

	wiredOperator = maps.Clone(operator)
	if wiredOperator == nil {
		wiredOperator = make(map[string]any)
	}
	envParent, envKey := draEvictionEnvPath[:strings.LastIndex(draEvictionEnvPath, ".")], draEvictionEnvPath[strings.LastIndex(draEvictionEnvPath, ".")+1:]
	manager, err := objectAt(wiredOperator, GPUOperatorComponent, envParent)
	if err != nil {
		return nil, nil, nil, err
	}
	env, err := upsertEnv(manager[envKey], key)
	if err != nil {
		return nil, nil, nil, err
	}
	manager[envKey] = env

	wiredLabeler = maps.Clone(labeler)
	if wiredLabeler == nil {
		wiredLabeler = make(map[string]any)
	}
	wiredLabeler[draNodeLabelerKeyPath] = key
	wiredLabeler[draNodeLabelerValuePath] = value
	wiredLabeler[draNodeLabelerEnabledPath] = true

	return wiredOperator, wiredDriver, wiredLabeler, nil
}

// objectAt walks the dotted path from values and returns the object at its
// end, creating the objects that are absent. Every object on the way is
// replaced by a shallow copy, so writing to the result does not mutate the
// values the caller passed in.
func objectAt(values map[string]any, component, path string) (map[string]any, error) {
	current := values
	for i, key := range strings.Split(path, ".") {
		switch child := current[key].(type) {
		case nil:
			next := make(map[string]any)
			current[key] = next
			current = next
		case map[string]any:
			next := maps.Clone(child)
			current[key] = next
			current = next
		default:
			at := strings.Join(strings.Split(path, ".")[:i+1], ".")
			return nil, fmt.Errorf("cannot wire DRA eviction: %s value %q must be an object, got %T", component, at, child)
		}
	}
	return current, nil
}

// upsertEnv returns the environment list with the eviction variable set to
// the label key: an existing entry is rewritten in place, a duplicate is
// dropped, and every other entry is kept.
func upsertEnv(current any, labelKey string) ([]any, error) {
	var existing []any
	if current != nil {
		list, ok := current.([]any)
		if !ok {
			return nil, fmt.Errorf("cannot wire DRA eviction: %s value %q must be an array, got %T", GPUOperatorComponent, draEvictionEnvPath, current)
		}
		existing = list
	}
	env := make([]any, 0, len(existing)+1)
	found := false
	for _, entry := range existing {
		e, ok := entry.(map[string]any)
		if !ok || e["name"] != draEvictionEnvName {
			env = append(env, entry)
			continue
		}
		if found {
			continue
		}
		env = append(env, map[string]any{"name": draEvictionEnvName, "value": labelKey})
		found = true
	}
	if !found {
		env = append(env, map[string]any{"name": draEvictionEnvName, "value": labelKey})
	}
	return env, nil
}
