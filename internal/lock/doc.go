// Package lock records per-project ownership and observes collisions and drift.
// JSON keys use dot-separated object segments with backslash-escaped literal
// dots and backslashes. Shared-container editors operate on bytes and never
// infer ownership: callers must check Status before edits and supply trusted
// adapter roots to ValidatePath/WriteFile before any filesystem mutation.
//
// Save persists a successful batch atomically. Individual writes are atomic;
// this package does not provide transactions across target and lockfile writes.
package lock
