// find the project root at runtime
//
// kinda a single use package, but it's useful everywhere so the low friction
// is nice.
package projectpath

import (
	"path"
	"path/filepath"
	"runtime"
)

var (
	_, b, _, _ = runtime.Caller(0)

	// Root folder of this project
	Root = filepath.Join(filepath.Dir(b), "../..")
)

// paths arg will be joined against the project root. like [path.Join]
func Join(paths ...string) string {
	paths = append([]string{Root}, paths...)

	return path.Join(paths...)
}
