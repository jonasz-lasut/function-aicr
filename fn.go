package main

import (
	"context"
	"encoding/json"
	"maps"
	"slices"
	"strings"
	"time"

	"github.com/crossplane/crossplane-runtime/v2/pkg/fieldpath"
	xpv2 "github.com/crossplane/crossplane/apis/v2/core/v2"
	"github.com/crossplane/function-sdk-go/errors"
	"github.com/crossplane/function-sdk-go/logging"
	fnv1 "github.com/crossplane/function-sdk-go/proto/v1"
	"github.com/crossplane/function-sdk-go/request"
	"github.com/crossplane/function-sdk-go/resource"
	"github.com/crossplane/function-sdk-go/resource/composed"
	"github.com/crossplane/function-sdk-go/response"
	corev1 "k8s.io/api/core/v1"

	"github.com/NVIDIA/aicr/pkg/recipe"

	"github.com/jonasz-lasut/function-aicr/input/v1beta1"
	"github.com/jonasz-lasut/function-aicr/internal/compose"
	"github.com/jonasz-lasut/function-aicr/internal/resolve"
	"github.com/jonasz-lasut/function-aicr/internal/schema"
)

// A composedKind is a kind of composed resource the function emits, whose CRD
// schema it asks Crossplane for. name keys both the request's required_schemas
// map and the response's requirements; provider is the provider whose CRD
// carries the schema, named when it turns out not to be installed.
type composedKind struct {
	name       string
	apiVersion string
	kind       string
	provider   string
}

var (
	releaseKind = composedKind{name: "release", apiVersion: compose.ReleaseAPIVersion, kind: compose.ReleaseKind, provider: "provider-helm"}
	objectKind  = composedKind{name: "object", apiVersion: compose.ObjectAPIVersion, kind: compose.ObjectKind, provider: "provider-kubernetes"}
)

// require asks Crossplane to resolve the schema.
func (k composedKind) require(rsp *fnv1.RunFunctionResponse) {
	response.RequireSchema(rsp, k.name, k.apiVersion, k.kind)
}

// validator compiles the schema, once Crossplane has resolved it, into the one
// Validator every composed resource of the kind is checked against. A schema
// that is not needed counts as resolved and yields no validator, so an
// unneeded provider need not be installed; a schema Crossplane resolved but
// could not find means the provider is not installed.
func (k composedKind) validator(req *fnv1.RunFunctionRequest, needed bool) (v *schema.Validator, resolved bool, err error) {
	if !needed {
		return nil, true, nil
	}
	s, resolved := request.GetRequiredSchema(req, k.name)
	if !resolved {
		return nil, false, nil
	}
	if s == nil {
		return nil, true, errors.Errorf("no schema for %s, Kind=%s: is %s installed?", k.apiVersion, k.kind, k.provider)
	}
	v, err = schema.NewValidator(s)
	return v, true, errors.Wrapf(err, "cannot use the schema for %s, Kind=%s", k.apiVersion, k.kind)
}

// Function resolves an AICR recipe and expands it into provider-helm Releases
// and provider-kubernetes Objects, gated by AICR's dependency order.
type Function struct {
	fnv1.UnimplementedFunctionRunnerServiceServer

	log logging.Logger
	ttl time.Duration
	dp  recipe.DataProvider
	// recipeVersion stamps resolved recipes and is surfaced as the summary's
	// recipeVersion: the github.com/NVIDIA/aicr module version, since that is
	// what pins the embedded recipe data.
	recipeVersion string
}

// RunFunction resolves the recipe and emits the desired composed resources.
func (f *Function) RunFunction(ctx context.Context, req *fnv1.RunFunctionRequest) (*fnv1.RunFunctionResponse, error) {
	f.log.Info("Running function", "tag", req.GetMeta().GetTag())
	rsp := response.To(req, f.ttl)

	in := &v1beta1.Input{}
	if err := request.GetInput(req, in); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get function input from %T", req))
		return rsp, nil
	}

	if err := validateTarget(in); err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}

	overrides, err := parseOverrides(in)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}

	oxr, err := request.GetObservedCompositeResource(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composite resource from %T", req))
		return rsp, nil
	}

	paved := fieldpath.Pave(oxr.Resource.Object)
	pcRef, err := resolve.ProviderConfigRef(in, paved)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot resolve the provider config reference"))
		return rsp, nil
	}

	criteria, err := resolve.Criteria(in, paved)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot resolve AICR recipe criteria"))
		return rsp, nil
	}
	if err := criteria.Validate(); err != nil {
		response.Fatal(rsp, errors.Wrap(err, "invalid AICR recipe criteria"))
		return rsp, nil
	}

	if !request.HasCapability(req, fnv1.Capability_CAPABILITY_REQUIRED_SCHEMAS) {
		response.Fatal(rsp, errors.New("function-aicr requires a Crossplane that supports required schemas; upgrade Crossplane, or pass --required-schemas to crossplane render"))
		return rsp, nil
	}

	res, err := recipe.NewBuilder(recipe.WithVersion(f.recipeVersion), recipe.WithDataProvider(f.dp)).BuildFromCriteria(ctx, criteria)
	if err != nil {
		response.Fatal(rsp, errors.Wrap(err, "cannot resolve AICR recipe"))
		return rsp, nil
	}

	managed := managedComponents(rsp, res, in)

	// Only require what this recipe actually composes. A recipe with no manifest
	// files must not oblige the user to install provider-kubernetes.
	needRelease, needObject := needs(managed)
	if needRelease {
		releaseKind.require(rsp)
	}
	if needObject {
		objectKind.require(rsp)
	}

	releaseSchema, relResolved, err := releaseKind.validator(req, needRelease)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	objectSchema, objResolved, err := objectKind.validator(req, needObject)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	if !relResolved || !objResolved {
		// Crossplane has not resolved our schemas yet. Emit no composed
		// resources; it will call again once it has.
		return rsp, nil
	}

	desired, err := request.GetDesiredComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get desired composed resources from %T", req))
		return rsp, nil
	}
	observed, err := request.GetObservedComposedResources(req)
	if err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot get observed composed resources from %T", req))
		return rsp, nil
	}
	values, err := resolveValues(ctx, res, managed)
	if err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}

	s := &stack{
		managed:       managed,
		values:        values,
		overrides:     overrides,
		pcRef:         pcRef,
		dp:            f.dp,
		releaseSchema: releaseSchema,
		objectSchema:  objectSchema,
		desired:       desired,
		observed:      observed,
	}
	if err := s.composeReleases(); err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	if err := s.composeObjects(ctx); err != nil {
		response.Fatal(rsp, err)
		return rsp, nil
	}
	if !in.SkipDeploymentOrder {
		s.gate(rsp)
	}

	if err := response.SetDesiredComposedResources(rsp, s.desired); err != nil {
		response.Fatal(rsp, errors.Wrapf(err, "cannot set desired composed resources from %T", req))
		return rsp, nil
	}

	if in.Target != nil {
		if err := setStatus(rsp, req, in.Target.FieldPath, s.summary(criteria, res)); err != nil {
			response.Fatal(rsp, errors.Wrap(err, "cannot set the resolved-recipe summary"))
			return rsp, nil
		}
	}

	response.ConditionTrue(rsp, "FunctionSuccess", "Success").TargetCompositeAndClaim()
	return rsp, nil
}

// A stack is what one composite resource's AICR recipe expands into: the
// managed components, with the values, overrides and provider config they
// deploy with; the Releases and Objects composed from them into desired; and
// what Crossplane has observed of those, which the gate reads to order the
// rollout.
type stack struct {
	managed   map[string]*recipe.ComponentRef
	values    map[string]map[string]any // the recipe's effective values, per component
	overrides map[string]*compose.Override
	pcRef     compose.ProviderConfigRef

	dp            recipe.DataProvider // holds the components' manifest files
	releaseSchema *schema.Validator   // nil when no component has a chart
	objectSchema  *schema.Validator   // nil when no component has manifest files

	desired  map[resource.Name]*resource.DesiredComposed
	observed map[resource.Name]resource.ObservedComposed
	// objects indexes, per component, the names of the Objects composed for
	// its pre-manifest and post-manifest files. The gate correlates observed
	// against desired resources by these names.
	objects manifestObjects
}

// resolveValues resolves the values every managed component deploys with; its
// Release and the rendering of its manifest files derive from them, and a
// manifest-only component has values too.
func resolveValues(ctx context.Context, res *recipe.RecipeResult, managed map[string]*recipe.ComponentRef) (map[string]map[string]any, error) {
	values := make(map[string]map[string]any, len(managed))
	for _, name := range slices.Sorted(maps.Keys(managed)) {
		v, err := res.GetValuesForComponentWithContext(ctx, name)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot resolve values for %q", name)
		}
		values[name] = v
	}
	return values, nil
}

// composeReleases adds a provider-helm Release per chart-backed managed
// component to desired, validated against the Release schema.
func (s *stack) composeReleases() error {
	for _, name := range slices.Sorted(maps.Keys(s.managed)) {
		c := s.managed[name]
		if c.IsManifestOnlyHelm() {
			continue // no Release
		}
		rel := compose.NewRelease(c, s.values[name], s.overrides[name], s.pcRef)
		if err := s.releaseSchema.Validate(rel); err != nil {
			return errors.Wrapf(err, "invalid Release for %q", name)
		}
		s.desired[resource.Name(name)] = &resource.DesiredComposed{Resource: toComposed(rel)}
	}
	return nil
}

// composeObjects renders every manifest file of every managed component with
// the component's effective values — AICR manifest files are Helm-style
// templates, rendered here exactly as AICR's bundler renders them — and adds a
// provider-kubernetes Object per resulting document to desired, validated
// against the Object schema. The Object names are indexed per component in
// objects, split into pre-manifest and post-manifest, for the gate. A file
// that renders to no document (its template gated everything off) composes
// nothing; a file that fails to render or parse is an error, never a silently
// missing piece of the recipe.
func (s *stack) composeObjects(ctx context.Context) error {
	s.objects = manifestObjects{pre: make(map[string][]string), post: make(map[string][]string)}
	for _, name := range slices.Sorted(maps.Keys(s.managed)) {
		c := s.managed[name]
		pre, err := s.composeWave(ctx, c, "pre", c.PreManifestFiles)
		if err != nil {
			return err
		}
		post, err := s.composeWave(ctx, c, "post", c.ManifestFiles)
		if err != nil {
			return err
		}
		if len(pre) > 0 {
			s.objects.pre[name] = pre
		}
		if len(post) > 0 {
			s.objects.post[name] = post
		}
	}
	return nil
}

// composeWave composes the Objects of one wave — pre or post — of a component's
// manifest files and returns their names, in document order.
func (s *stack) composeWave(ctx context.Context, c *recipe.ComponentRef, wave string, paths []string) ([]string, error) {
	var names []string
	for _, path := range paths {
		data, err := recipe.GetManifestContentWithContext(ctx, s.dp, path)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot read manifest %q for %q", path, c.Name)
		}
		rendered, err := compose.RenderManifest(c, s.values[c.Name], s.overrides[c.Name], data)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot render manifest %q for %q", path, c.Name)
		}
		docs, err := compose.SplitManifests(rendered)
		if err != nil {
			return nil, errors.Wrapf(err, "cannot parse rendered manifest %q for %q", path, c.Name)
		}
		for _, doc := range docs {
			name := compose.ManifestObjectName(c.Name, wave, doc, len(names))
			obj := compose.NewObject(doc, name, s.pcRef)
			if err := s.objectSchema.Validate(obj); err != nil {
				return nil, errors.Wrapf(err, "invalid Object %q", name)
			}
			s.desired[resource.Name(name)] = &resource.DesiredComposed{Resource: toComposed(obj)}
			names = append(names, name)
		}
	}
	return names, nil
}

// gate withholds composed resources whose prerequisites are not yet observed
// Ready, so a stack rolls out in AICR's order. For each managed component: its
// pre-manifest Objects wait for the component's managed dependencies; its
// Release waits for those dependencies and for its pre-manifest Objects; its
// post-manifest Objects wait for its Release. A manifest-only component has no
// Release, so all of its Objects wait for its dependencies. Nothing already
// observed is ever withheld — gating orders creation; it must never tear down
// a live resource because a sibling flapped.
//
// Readiness is read from observed, never from desired[...].Ready: that field
// is set by function-auto-ready, which runs later in the pipeline, so reading
// it here would deadlock the gate. Dependencies that are skipped or absent from
// the managed set are treated as satisfied — the user opted to manage them
// elsewhere.
func (s *stack) gate(rsp *fnv1.RunFunctionResponse) {
	for _, name := range slices.Sorted(maps.Keys(s.managed)) {
		c := s.managed[name]
		dep, blocked := s.blockingDependency(c)

		if c.IsManifestOnlyHelm() {
			if blocked && s.withhold(s.objects.all(name)...) {
				response.Normalf(rsp, "delaying objects of %q until dependency %q is ready", name, dep)
			}
			continue
		}

		if blocked {
			if s.withhold(s.objects.pre[name]...) {
				response.Normalf(rsp, "delaying pre-manifest objects of %q until dependency %q is ready", name, dep)
			}
			if s.withhold(name) {
				response.Normalf(rsp, "delaying %q until dependency %q is ready", name, dep)
			}
		} else if obj, unready := s.firstUnready(s.objects.pre[name]); unready && s.withhold(name) {
			response.Normalf(rsp, "delaying %q until pre-manifest object %q is ready", name, obj)
		}

		if !s.ready(name) && s.withhold(s.objects.post[name]...) {
			response.Normalf(rsp, "delaying post-manifest objects of %q until its Release is ready", name)
		}
	}
}

// ready reports whether a composed resource is observed Ready.
func (s *stack) ready(name string) bool {
	oc, ok := s.observed[resource.Name(name)]
	return ok && oc.Resource.GetCondition(xpv2.TypeReady).Status == corev1.ConditionTrue
}

// firstUnready names the first of names that is not observed Ready.
func (s *stack) firstUnready(names []string) (string, bool) {
	for _, n := range names {
		if !s.ready(n) {
			return n, true
		}
	}
	return "", false
}

// componentReady reports whether a managed dependency is Ready: its Release
// for a chart-backed component; every Object it composed for a manifest-only
// one, whose readiness nothing else carries (no Objects composed: nothing to
// wait for).
func (s *stack) componentReady(name string) bool {
	if s.managed[name].IsManifestOnlyHelm() {
		_, unready := s.firstUnready(s.objects.all(name))
		return !unready
	}
	return s.ready(name)
}

// blockingDependency names the first managed dependency of c that is not yet
// Ready.
func (s *stack) blockingDependency(c *recipe.ComponentRef) (string, bool) {
	for _, dep := range c.DependencyRefs {
		if _, isManaged := s.managed[dep]; !isManaged {
			continue
		}
		if !s.componentReady(dep) {
			return dep, true
		}
	}
	return "", false
}

// withhold removes the named resources from desired unless they are already
// observed, and reports whether it removed any.
func (s *stack) withhold(names ...string) bool {
	removed := false
	for _, n := range names {
		if _, created := s.observed[resource.Name(n)]; created {
			continue // never gate something already created
		}
		if _, ok := s.desired[resource.Name(n)]; !ok {
			continue
		}
		delete(s.desired, resource.Name(n))
		removed = true
	}
	return removed
}

// deployed lists, in AICR's deployment order, the managed components the gate
// did not withhold: a chart-backed component's Release, or every Object of a
// manifest-only component, is present in desired.
func (s *stack) deployed(order []string) []string {
	inDesired := func(names ...string) bool {
		for _, n := range names {
			if _, ok := s.desired[resource.Name(n)]; !ok {
				return false
			}
		}
		return true
	}
	deployed := make([]string, 0, len(s.managed))
	for _, name := range order {
		c, ok := s.managed[name]
		if !ok {
			continue
		}
		if c.IsManifestOnlyHelm() && !inDesired(s.objects.all(name)...) {
			continue
		}
		if !c.IsManifestOnlyHelm() && !inDesired(name) {
			continue
		}
		deployed = append(deployed, name)
	}
	return deployed
}

// summary is the resolved-recipe summary written to the composite resource:
// the recipe's identity and the components deployed so far, in AICR's order.
func (s *stack) summary(criteria *recipe.Criteria, res *recipe.RecipeResult) map[string]any {
	deployed := s.deployed(res.DeploymentOrder)
	components := make([]map[string]string, 0, len(deployed))
	for _, name := range deployed {
		components = append(components, map[string]string{"name": name})
	}
	return map[string]any{
		"recipeName":         recipeName(criteria),
		"recipeVersion":      res.Metadata.Version,
		"componentCount":     len(deployed),
		"deployedComponents": components,
	}
}

// setStatus writes the summary keys to the field path named by the function
// input's target. Each key is set individually so sibling keys already at the
// target — written by the XR's author or by an earlier function in the
// pipeline — survive. It reads the desired composite from the request rather
// than reconstructing it, so the function never needs to know the XR's kind.
func setStatus(rsp *fnv1.RunFunctionResponse, req *fnv1.RunFunctionRequest, fieldPath string, summary map[string]any) error {
	dxr, err := request.GetDesiredCompositeResource(req)
	if err != nil {
		return errors.Wrapf(err, "cannot get desired composite resource from %T", req)
	}
	paved := fieldpath.Pave(dxr.Resource.Object)
	for _, key := range slices.Sorted(maps.Keys(summary)) {
		path := fieldPath + "." + key
		if err := paved.SetValue(path, summary[key]); err != nil {
			return errors.Wrapf(err, "cannot set %q on the desired composite resource", path)
		}
	}
	return response.SetDesiredCompositeResource(rsp, dxr)
}

// recipeName is the identifier AICR gives a recipe, built from the criteria
// as accelerator-service[-os]-intent[-platform]. Unstated dimensions are
// omitted rather than left as empty segments.
func recipeName(c *recipe.Criteria) string {
	parts := make([]string, 0, 5)
	for _, part := range []string{string(c.Accelerator), string(c.Service), string(c.OS), string(c.Intent), string(c.Platform)} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	return strings.Join(parts, "-")
}

// manifestObjects indexes, per component, the names of the Objects composed
// for its pre-manifest and post-manifest files.
type manifestObjects struct {
	pre  map[string][]string
	post map[string][]string
}

// all returns every Object name composed for a component, pre then post.
func (m manifestObjects) all(name string) []string {
	return slices.Concat(m.pre[name], m.post[name])
}

// needs reports whether the managed components yield any Release and any Object.
func needs(managed map[string]*recipe.ComponentRef) (release, object bool) {
	for _, c := range managed {
		if !c.IsManifestOnlyHelm() {
			release = true
		}
		if len(c.PreManifestFiles) > 0 || len(c.ManifestFiles) > 0 {
			object = true
		}
	}
	return release, object
}

// managedComponents indexes the resolved, enabled, not-skipped Helm components
// by name. Components the recipe disables (an overlay set overrides.enabled or
// overrides.install to false) stay in AICR's inventory but are never deployed,
// exactly as AICR's own deployers treat them. A skipComponents or
// componentOverrides entry that names nothing is reported as a warning: a typo
// there would otherwise silently deploy the component, or drop the override.
func managedComponents(rsp *fnv1.RunFunctionResponse, res *recipe.RecipeResult, in *v1beta1.Input) map[string]*recipe.ComponentRef {
	skip := make(map[string]bool, len(in.SkipComponents))
	for _, c := range in.SkipComponents {
		skip[c.Name] = true
	}

	inRecipe := make(map[string]bool, len(res.ComponentRefs))
	managed := make(map[string]*recipe.ComponentRef, len(res.ComponentRefs))
	for i := range res.ComponentRefs {
		c := &res.ComponentRefs[i]
		inRecipe[c.Name] = true
		if !c.IsEnabled() || skip[c.Name] {
			continue
		}
		if c.Type != recipe.ComponentTypeHelm {
			response.Normalf(rsp, "skipping non-Helm component %q (type %s) — not yet supported", c.Name, c.Type)
			continue
		}
		managed[c.Name] = c
	}

	for _, name := range slices.Sorted(maps.Keys(skip)) {
		if !inRecipe[name] {
			response.Warning(rsp, errors.Errorf("skipComponents names %q, which is not a component of the resolved recipe; check the spelling", name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(in.ComponentOverrides)) {
		if _, ok := managed[name]; !ok {
			response.Warning(rsp, errors.Errorf("componentOverrides[%q] names no managed component of the resolved recipe (skipped, disabled by the recipe, or misspelled); the override is ignored", name))
		}
	}
	return managed
}

func validateTarget(in *v1beta1.Input) error {
	if in.Target == nil {
		return nil
	}
	if in.Target.FieldPath != "status" && !strings.HasPrefix(in.Target.FieldPath, "status.") {
		return errors.Errorf("target.fieldPath must begin with %q, got %q", "status", in.Target.FieldPath)
	}
	return nil
}

// parseOverrides decodes every componentOverrides entry up front, so a
// malformed override is a Fatal on the first reconcile rather than one that
// waits for its component to be resolved. Values must be a JSON object; any
// other JSON kind is an error, never a silently dropped override.
func parseOverrides(in *v1beta1.Input) (map[string]*compose.Override, error) {
	overrides := make(map[string]*compose.Override, len(in.ComponentOverrides))
	for _, name := range slices.Sorted(maps.Keys(in.ComponentOverrides)) {
		e := in.ComponentOverrides[name]
		o := &compose.Override{Version: e.Version, Namespace: e.Namespace}
		if e.Values != nil && len(e.Values.Raw) > 0 {
			var v any
			if err := json.Unmarshal(e.Values.Raw, &v); err != nil {
				return nil, errors.Wrapf(err, "cannot parse componentOverrides[%q].values", name)
			}
			switch values := v.(type) {
			case nil:
			case map[string]any:
				o.Values = values
			default:
				return nil, errors.Errorf("componentOverrides[%q].values must be an object, got %s", name, jsonKind(v))
			}
		}
		overrides[name] = o
	}
	return overrides, nil
}

// jsonKind names the JSON kind of a value decoded into an any.
func jsonKind(v any) string {
	switch v.(type) {
	case string:
		return "string"
	case float64:
		return "number"
	case bool:
		return "boolean"
	case []any:
		return "array"
	default:
		return "unknown"
	}
}

func toComposed(obj map[string]any) *composed.Unstructured {
	cd := composed.New()
	cd.Object = obj
	return cd
}
