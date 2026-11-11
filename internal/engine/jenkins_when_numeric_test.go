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

// Expected outcomes were checked with Groovy on the installed Jenkins controller.
func TestJenkinsDecimalLiteralsPreserveGroovySemantics(t *testing.T) {
	for _, expression := range []string{
		`1.20 == 1.2`,
		`-0.50 < -0.1`,
		`1e3 == 1000`,
		`1.234567890123456789 > 1.234567890123456788`,
		`(1.00e3).toString() == "1.00E+3"`,
		`(0.0000000).toString() == "0E-7"`,
		`(-0.00).toString() == "0.00"`,
		`(0.000001).toString() == "0.000001"`,
		`params.THRESHOLD >= 0.125`,
	} {
		got, err := EvaluateJenkinsWhen(expression, map[string]interface{}{"THRESHOLD": json.Number("0.125")})
		if err != nil || !got {
			t.Errorf("%s: got=%t error=%v", expression, got, err)
		}
	}
	for _, expression := range []string{`0x1p2 == 4`, `.5 == 0.5`, `1. == 1`, `1e1001 == 1`} {
		if err := ValidateJenkinsWhen(expression); err == nil {
			t.Errorf("accepted unsupported Groovy decimal %s", expression)
		}
	}
}

func TestJenkinsDecimalStringMatchesGroovy(t *testing.T) {
	for _, tc := range []struct{ literal, want string }{
		{"00.30", "0.30"}, {"01.2", "1.2"}, {"100.000", "100.000"},
		{"0.0000001", "1E-7"}, {"0.0000010", "0.0000010"}, {"1000e-3", "1.000"},
		{"10.0e+1", "100"}, {"10.0e-8", "1.00E-7"}, {"0e5", "0E+5"},
		{"0.00e3", "0E+1"}, {"0.000e-3", "0.000000"}, {"-0.0000001", "-1E-7"},
		{"-12.3400e+7", "-1.23400E+8"}, {"0000.0010", "0.0010"},
		{"9.999e-6", "0.000009999"}, {"999.99e-8", "0.0000099999"},
	} {
		value, err := parseJenkinsDecimal(tc.literal)
		if err != nil || value.String() != tc.want {
			t.Errorf("%s: got %q want %q (err=%v)", tc.literal, value.String(), tc.want, err)
		}
	}
}
