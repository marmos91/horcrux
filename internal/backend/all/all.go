// Package all imports all backend implementations to register them.
package all

import (
	_ "github.com/marmos91/horcrux/internal/backend/azure"
	_ "github.com/marmos91/horcrux/internal/backend/dropbox"
	_ "github.com/marmos91/horcrux/internal/backend/ftp"
	_ "github.com/marmos91/horcrux/internal/backend/gdrive"
	_ "github.com/marmos91/horcrux/internal/backend/local"
	_ "github.com/marmos91/horcrux/internal/backend/s3"
)
