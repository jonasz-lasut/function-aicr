package main

import (
	"context"
	"testing"

	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"

	"github.com/NVIDIA/aicr/pkg/oci"

	"github.com/crossplane/function-sdk-go/logging"
)

func TestOCIPullOptions(t *testing.T) {
	const digest = "sha256:8ac7f6d54e8bcbf074f156a11f2c8c1ca6dcafed85e880e99a3126c0810cd66f"

	type args struct {
		repository string
		digest     string
	}
	type want struct {
		opts   oci.RecipePullOptions
		source string
		err    error
	}

	cases := map[string]struct {
		reason string
		args   args
		want   want
	}{
		"DigestPinnedRepository": {
			reason: "A repository plus a sha256 manifest digest is the one accepted shape; the source is the canonical repository@digest identity.",
			args:   args{repository: "ghcr.io/acme/aicr-recipes", digest: digest},
			want: want{
				opts:   oci.RecipePullOptions{Repository: "ghcr.io/acme/aicr-recipes", Selector: digest},
				source: "ghcr.io/acme/aicr-recipes@" + digest,
			},
		},
		"StripsOCISchemePrefixFromTheSource": {
			reason: "AICR accepts an oci:// prefix on the repository, but the summary's source identity should not carry it.",
			args:   args{repository: "oci://ghcr.io/acme/aicr-recipes", digest: digest},
			want: want{
				opts:   oci.RecipePullOptions{Repository: "oci://ghcr.io/acme/aicr-recipes", Selector: digest},
				source: "ghcr.io/acme/aicr-recipes@" + digest,
			},
		},
		"RejectsRepositoryWithoutDigest": {
			reason: "The flags only make sense together; a repository alone must not silently serve embedded data.",
			args:   args{repository: "ghcr.io/acme/aicr-recipes"},
			want:   want{err: cmpopts.AnyError},
		},
		"RejectsDigestWithoutRepository": {
			reason: "The flags only make sense together; a digest alone must not silently serve embedded data.",
			args:   args{digest: digest},
			want:   want{err: cmpopts.AnyError},
		},
		"RejectsTagSelector": {
			reason: "A tag would let mutable registry state change what the function deploys; only an immutable sha256 manifest digest is accepted.",
			args:   args{repository: "ghcr.io/acme/aicr-recipes", digest: "v1.2.3"},
			want:   want{err: cmpopts.AnyError},
		},
		"RejectsRepositoryEmbeddingAReference": {
			reason: "AICR requires the digest flag to be the single source of truth: a repository embedding its own tag or digest is invalid.",
			args:   args{repository: "ghcr.io/acme/aicr-recipes:latest", digest: digest},
			want:   want{err: cmpopts.AnyError},
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			opts, source, err := ociPullOptions(tc.args.repository, tc.args.digest)

			if diff := cmp.Diff(tc.want.err, err, cmpopts.EquateErrors()); diff != "" {
				t.Fatalf("%s\nociPullOptions(...): -want err, +got err:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.opts, opts); diff != "" {
				t.Errorf("%s\nociPullOptions(...): -want opts, +got opts:\n%s", tc.reason, diff)
			}
			if diff := cmp.Diff(tc.want.source, source); diff != "" {
				t.Errorf("%s\nociPullOptions(...): -want source, +got source:\n%s", tc.reason, diff)
			}
		})
	}
}

func TestNewDataProviderEmbedded(t *testing.T) {
	// With no OCI flags the provider is the embedded catalog and the source
	// identity is empty; the OCI path itself is exercised by AICR's own
	// acceptance tests and needs a registry this suite deliberately avoids.
	dp, source, err := newDataProvider(context.Background(), logging.NewNopLogger(), "", "")
	if err != nil {
		t.Fatalf("newDataProvider(ctx, log, \"\", \"\"): unexpected error: %v", err)
	}
	if dp == nil {
		t.Error("newDataProvider(ctx, log, \"\", \"\"): got a nil DataProvider")
	}
	if source != "" {
		t.Errorf("newDataProvider(ctx, log, \"\", \"\"): source = %q, want \"\"", source)
	}
}
