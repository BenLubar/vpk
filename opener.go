package vpk

import (
	"fmt"
	"os"
)

type Opener interface {
	// Main opens the main VPK file (*_dir.vpk or no suffix)
	Main() (File, error)
	// Archive opens a data-only VPK file (*_###.vpk where # is a digit)
	Archive(index int16) (File, error)
}

type singleVPKOpener string

// SingleVPK implements an Opener for a single-part VPK on the OS filesystem.
func SingleVPK(path string) Opener {
	return singleVPKOpener(path)
}

func (o singleVPKOpener) Main() (File, error) {
	return os.Open(string(o))
}

func (o singleVPKOpener) Archive(index int16) (File, error) {
	return nil, os.ErrNotExist
}

type multiVPKOpener string

// MultiVPK implements an Opener for a multi-part VPK on the OS filesystem.
// prefix should be the part before "_dir.vpk".
func MultiVPK(prefix string) Opener {
	return multiVPKOpener(prefix)
}

func (o multiVPKOpener) Main() (File, error) {
	return os.Open(string(o) + "_dir.vpk")
}

func (o multiVPKOpener) Archive(index int16) (File, error) {
	return os.Open(fmt.Sprintf("%s_%03d.vpk", string(o), index))
}
