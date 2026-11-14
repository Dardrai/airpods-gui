//go:build !windows

package winpath

import "errors"

// Update is for Windows only.
func Update(dir string, add bool) error {
	return errors.New("the PATH entry is only kept on Windows")
}
