// Package modelrouter holds the data files shipped with promari-model-router.
//
// The files under data/ and the agent definitions under agents/ are
// embedded, so the binary works without the plugin directory, while the
// files stay readable for anyone who wants to see why a prompt was routed.
package modelrouter

import "embed"

// Data holds the TOML configuration and tables, the JSONL evaluation set and
// the JSON artifact.
//
//go:embed data/*.toml data/*.jsonl data/*.json
var Data embed.FS

// Agents holds the fixed-tier agent definitions (checked by `pmr lint`).
//
//go:embed agents/*.md
var Agents embed.FS
