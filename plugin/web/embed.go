package web

import _ "embed"

// Page embeds the generated Web UI. Edit plugin/web-ui/src instead; build the
// Web UI before Go compilation to refresh dist/index.html.
//
//go:embed dist/index.html
var Page []byte
