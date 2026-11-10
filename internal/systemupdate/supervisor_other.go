//go:build !darwin

package systemupdate

import "context"

func checkUpdateSupervisor(context.Context) error { return nil }
