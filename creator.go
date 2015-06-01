package vpk

import (
	"fmt"
	"io"
	"os"
)

type Creator interface {
	// Main creates the main VPK file (*_dir.vpk or no suffix)
	Main() (io.WriteCloser, error)
	// Archive creates a data-only VPK file (*_###.vpk where # is a digit)
	Archive(index int16) (io.WriteCloser, error)
}

type singleVPKCreator string

// SingleVPKCreator implements a Creator for a single-part VPK on the OS
// filesystem.
func SingleVPKCreator(path string) Creator {
	return singleVPKCreator(path)
}

func (o singleVPKCreator) Main() (io.WriteCloser, error) {
	return os.Create(string(o))
}

func (o singleVPKCreator) Archive(index int16) (io.WriteCloser, error) {
	return nil, os.ErrPermission
}

type multiVPKCreator string

// MultiVPKCreator implements a Creator for a multi-part VPK on the OS
// filesystem. prefix should be the part before "_dir.vpk".
func MultiVPKCreator(prefix string) Creator {
	return multiVPKCreator(prefix)
}

func (o multiVPKCreator) Main() (io.WriteCloser, error) {
	return os.Create(string(o) + "_dir.vpk")
}

func (o multiVPKCreator) Archive(index int16) (io.WriteCloser, error) {
	return os.Create(fmt.Sprintf("%s_%03d.vpk", string(o), index))
}
