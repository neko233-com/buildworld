package engine

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestStageTimeoutHonorsJenkinsBeforeOptions(t *testing.T) {
	for _, beforeOptions := range []bool{false, true} {
		stage := Stage{TimeoutSec: 1, JenkinsWhenBeforeOptions: beforeOptions}
		stageCtx, cancel, matched, err := evaluateStageCondition(context.Background(), stage, func() (bool, error) {
			time.Sleep(1100 * time.Millisecond)
			return true, nil
		})
		if !matched || err != nil {
			cancel()
			t.Fatalf("beforeOptions=%v matched=%v err=%v", beforeOptions, matched, err)
		}
		expired := errors.Is(stageCtx.Err(), context.DeadlineExceeded)
		cancel()
		if expired == beforeOptions {
			t.Fatalf("beforeOptions=%v condition timeout=%v", beforeOptions, expired)
		}
	}
}

func TestBeforeOptionsRequiresJenkinsCondition(t *testing.T) {
	config := &BuildConfig{Stages: []Stage{{Name: "deploy", JenkinsWhenBeforeOptions: true}}}
	if err := ValidatePipelineSemantics(config); err == nil {
		t.Fatal("beforeOptions accepted without a Jenkins condition")
	}
}

func TestStagePostRejectsUnrepresentableSemantics(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stage  Stage
		remote bool
	}{
		{"remote", Stage{Name: "build", Post: map[string][]Step{"always": {{Name: "cleanup", Type: "shell"}}}}, true},
		{"unknown", Stage{Name: "build", Post: map[string][]Step{"unstable": {{Name: "cleanup", Type: "shell"}}}}, false},
		{"step condition", Stage{Name: "build", Post: map[string][]Step{"always": {{Name: "cleanup", Type: "shell", If: "success()"}}}}, false},
		{"watch", Stage{Name: "build", Post: map[string][]Step{"always": {{Name: "watch", Type: "service_watch"}}}}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			config := &BuildConfig{Stages: []Stage{tc.stage}}
			if tc.remote {
				config.AgentRequirements = []string{"remote"}
			}
			if err := ValidatePipelineSemantics(config); err == nil {
				t.Fatal("unsupported stage post was accepted")
			}
		})
	}
}
