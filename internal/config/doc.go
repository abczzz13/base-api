// Package config loads and validates application configuration.
//
// Configuration is loaded from environment variables with strict fail-fast
// validation. Every supported key also accepts a companion <KEY>_FILE
// variable whose value is read from a file path.
package config
