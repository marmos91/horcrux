package version

// Version is the application version, overridable at build time via:
//
//	go build -ldflags "-X github.com/marmos91/horcrux/internal/version.Version=v1.0.0"
var Version = "dev"
