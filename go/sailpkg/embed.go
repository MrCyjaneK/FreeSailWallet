package sailpkg

import "embed"

// RPMFileName is the unmodified SailWallet installation package.
const RPMFileName = "harbour-sailwallet-1.1.0-1.aarch64.rpm"

//go:embed harbour-sailwallet-1.1.0-1.aarch64.rpm
var rpmBytes []byte

//go:embed patches
var patchFS embed.FS
