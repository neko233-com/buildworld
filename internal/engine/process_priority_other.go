//go:build !windows

package engine

// Process priority APIs differ substantially across Unix hosts and lowering a
// nice value often cannot be reversed without elevated privileges. Concurrency
// and child-tool thread budgets remain fully hot-reloadable on these systems.
func applyProcessBackgroundMode(_ bool) error {
	return nil
}
