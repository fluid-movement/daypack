// Package adapters merges pure renderers through the shared contract. Field
// capability failures are returned as Unsupported diagnostics without items for
// that pair; unexpected errors abort with redacted messages. Selected adapters'
// notes are available even when a project has no entries (for example, Pi's
// project trust requirement). Rendering does not inspect or grant harness trust.
package adapters
