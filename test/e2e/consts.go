// Package e2e holds function-aicr's end-to-end suite. It follows
// crossplane/crossplane's own suite: features built with
// sigs.k8s.io/e2e-framework, run through Crossplane's e2e environment
// (test/e2e/config) and assembled from its helpers (test/e2e/funcs), with
// each feature's manifests in a directory of its own under manifests/.
//
// The cluster is not this package's to create. `make e2e.up` provisions it
// (kind, Crossplane, the providers, the registry and function-aicr built
// from this tree) and `make e2e.test` runs the suite against it.
package e2e

// LabelArea represents the 'area' of a feature. For example 'install',
// 'recipe', 'rollout', etc. Assessments roll up to features, which roll up
// to feature areas. Features within an area may be split across files.
const LabelArea = "area"

// LabelSize represents the 'size' (i.e. duration) of a test.
const LabelSize = "size"

const (
	// LabelSizeSmall is used for tests that usually complete in a minute.
	LabelSizeSmall = "small"

	// LabelSizeLarge is used for test that usually complete in over a minute.
	LabelSizeLarge = "large"
)

// FieldManager is the server-side apply field manager used when applying
// manifests.
const FieldManager = "function-aicr-e2e"
