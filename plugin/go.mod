module stickyproxy/native-plugin

go 1.25.0

require golang.org/x/net v0.57.0

// The plugin deliberately avoids importing the CLIProxyAPI Go module. Its
// native C ABI and JSON protocol remain compatible with CPA while keeping the
// implementation independently buildable.
