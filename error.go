package vpk

import (
	"errors"
	"fmt"
)

var ErrInvalidMagic = errors.New("vpk: invalid magic number")

type ErrUnsupportedVersion uint32

func (err ErrUnsupportedVersion) Error() string {
	return fmt.Sprintf("vpk: unsupported VPK version: %d", uint32(err))
}

type ErrCRCMismatch struct {
	Actual, Expected uint32
}

func (err ErrCRCMismatch) Error() string {
	return fmt.Sprintf("vpk: CRC mismatch: %08x (expected %08x)", err.Actual, err.Expected)
}

type ErrInvalidEntry struct {
	Dir, Base, Ext string
}

func (err ErrInvalidEntry) Error() string {
	return fmt.Sprintf("vpk: entry for %s/%s.%s is corrupt", err.Dir, err.Base, err.Ext)
}

var ErrFileTooBig = errors.New("vpk: file too big")
