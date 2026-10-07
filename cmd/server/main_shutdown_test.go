package main

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestShutdownDrainsBuildsBeforeHTTPAndReportsBothErrors(t *testing.T) {
	var order []string
	buildErr, httpErr := errors.New("build"), errors.New("http")
	err := stopServerServices(context.Background(), func(context.Context) error { order = append(order, "builds"); return buildErr }, func(context.Context) error { order = append(order, "http"); return httpErr })
	if !reflect.DeepEqual(order, []string{"builds", "http"}) || !errors.Is(err, buildErr) || !errors.Is(err, httpErr) {
		t.Fatalf("order=%v error=%v", order, err)
	}
}
