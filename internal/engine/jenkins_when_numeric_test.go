package engine

import (
	"encoding/json"
	"testing"
)

func TestBuildParametersPreserveLargeIntegerForJenkinsConditions(t *testing.T) {
	params := parseParams(`{"COUNT":9007199254740993}`)
	if params["COUNT"] != json.Number("9007199254740993") {
		t.Fatalf("number lost precision: %#v", params)
	}
	matched, err := EvaluateJenkinsWhen(`params.COUNT == 9007199254740993`, params)
	if err != nil || !matched {
		t.Fatalf("matched=%t error=%v", matched, err)
	}
	if parseParams(`{"COUNT":1} {"COUNT":2}`) != nil {
		t.Fatal("accepted trailing JSON")
	}
}
