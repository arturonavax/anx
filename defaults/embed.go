package defaults

import "embed"

// EmbeddedFS bundles minimal, universal ANX engine defaults so the binary
// compiles, starts, runs diagnostics, and manages its own installation
// without requiring an external env repository checkout.
//
//go:embed *.yaml
var EmbeddedFS embed.FS
