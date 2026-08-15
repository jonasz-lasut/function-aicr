package compose

import (
	"strings"

	"github.com/NVIDIA/aicr/pkg/manifest"
	"github.com/NVIDIA/aicr/pkg/recipe"
)

// defaultChartVersion is what AICR's deployers use for .Chart.Version when a
// component pins no version.
const defaultChartVersion = "0.1.0"

// RenderManifest renders an AICR manifest file for a component. AICR manifest
// files are Go templates with Helm conventions — .Values.<component>,
// .Release.Namespace, .Chart.Name/.Chart.Version, toYaml | nindent — that
// AICR's bundler renders into a local chart; this renders them the same way,
// with the same renderer, from the component's effective namespace, chart,
// version and values (overrides applied). The result is plain YAML, possibly
// empty when the template gates every document off.
func RenderManifest(c *recipe.ComponentRef, values map[string]any, ov *Override, data []byte) ([]byte, error) {
	namespace, version, values := effective(c, values, ov)
	return manifest.Render(data, manifest.RenderInput{
		ComponentName: c.Name,
		Namespace:     namespace,
		ChartName:     c.EffectiveChart(),
		ChartVersion:  chartVersion(version),
		Values:        values,
	})
}

// chartVersion is the .Chart.Version AICR's deployers expose to templates: the
// component's version without a leading "v", defaulted when unset.
func chartVersion(version string) string {
	version = strings.TrimPrefix(version, "v")
	if version == "" {
		return defaultChartVersion
	}
	return version
}
