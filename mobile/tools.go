//go:build tools

package mobile

// gobind (gomobile) generates the Java bindings from this module, so its
// support package must be a dependency of the module.
import _ "github.com/sagernet/gomobile/bind"
