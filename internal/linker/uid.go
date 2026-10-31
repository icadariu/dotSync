package linker

import "os"

// Getuid is the uid that owns trusted files; a test seam.
var Getuid = os.Getuid
