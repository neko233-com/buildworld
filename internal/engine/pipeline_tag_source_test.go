package engine

import (
	"context"
	"strings"
	"testing"

	"github.com/neko233-com/buildworld/internal/store"
)

type tagProofFetcher struct {
	revision string
	objectID string
}

func (f tagProofFetcher) FetchSCMPipeline(context.Context, SCMPipelineSource) (string, PipelineFormat, error) {
	return "fixture", FormatJenkinsfile, nil
}

func (f tagProofFetcher) FetchSCMPipelineWithRevision(context.Context, SCMPipelineSource) (string, PipelineFormat, string, error) {
	return "fixture", FormatJenkinsfile, f.revision, nil
}

func (f tagProofFetcher) FetchSCMPipelineWithObject(context.Context, SCMPipelineSource) (string, PipelineFormat, string, string, error) {
	return "fixture", FormatJenkinsfile, f.revision, f.objectID, nil
}

func TestSCMSourceVerifiesAnnotatedTagObjectIdentity(t *testing.T) {
	commit := strings.Repeat("a", 40)
	object := strings.Repeat("b", 40)
	previous := DefaultSCMFetcher
	DefaultSCMFetcher = tagProofFetcher{revision: commit, objectID: object}
	t.Cleanup(func() { DefaultSCMFetcher = previous })
	project := &store.Project{PipelineSourceMode: "scm", PipelineSCMRepo: "unused", PipelineSCMPath: "Jenkinsfile"}
	for _, tc := range []struct {
		ref, requested string
		wantError      bool
	}{
		{"refs/tags/release", object, false},
		{"refs/tags/release", strings.Repeat("c", 40), true},
		{"main", object, true},
	} {
		_, revision, err := ResolveBuildPipelineSource(context.Background(), project, &store.Build{Branch: tc.ref, CommitSHA: tc.requested})
		if (err != nil) != tc.wantError || (!tc.wantError && revision != commit) {
			t.Fatalf("ref=%q revision=%q error=%v", tc.ref, revision, err)
		}
	}
}

func TestInlineTagRequiresConfiguredRepository(t *testing.T) {
	_, _, err := ResolveBuildPipelineSource(context.Background(), &store.Project{Config: "inline"}, &store.Build{Branch: "refs/tags/release"})
	if err == nil {
		t.Fatal("inline tag without a repository cannot be verified")
	}
}
