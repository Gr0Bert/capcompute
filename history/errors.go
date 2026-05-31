package history

import "fmt"

// NotFoundError means a requested run does not exist in the store.
type NotFoundError struct {
	RunID string
}

func (e NotFoundError) Error() string {
	return fmt.Sprintf("run %q not found", e.RunID)
}

// VersionConflictError means a write used a stale run history version.
type VersionConflictError struct {
	RunID    string
	Expected int64
	Got      int64
}

func (e VersionConflictError) Error() string {
	return fmt.Sprintf("history version mismatch for run %q: expected %d got %d", e.RunID, e.Expected, e.Got)
}

// UnsupportedSchemaError means persisted history is newer than this library can read.
type UnsupportedSchemaError struct {
	Entity  string
	Version int
	Current int
}

func (e UnsupportedSchemaError) Error() string {
	return fmt.Sprintf("unsupported %s schema version %d: current %d", e.Entity, e.Version, e.Current)
}
