// Package apply reconciles manifests through the adapter contract and per-project
// ownership. Prepare and Build are read-only; Plan exposes safe presentation data
// while Reconciliation retains the private payload used by Execute.
//
// Execution publishes deterministic batches, saving ownership after each batch.
// Target publication and lock persistence are not a cross-file transaction. If a
// target write succeeds but saving its ownership fails, retry conservatively
// treats that matching content as unowned and requires explicit adoption.
package apply
