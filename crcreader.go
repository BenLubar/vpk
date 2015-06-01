package vpk

import (
	"hash/crc32"
	"io"
)

// crcReader returns an io.ReadCloser where the Read method delegates to r and
// the Close method calls close and returns ErrCRCMismatch if the IEEE CRC32
// checksum of the data read from r does not match the crc given as an argument.
func crcReader(r io.Reader, close func() error, crc uint32) io.ReadCloser {
	hash := crc32.NewIEEE()
	r = io.TeeReader(r, hash)
	close = func(close func() error) func() error {
		return func() error {
			if err := close(); err != nil {
				return err
			}

			if actual := hash.Sum32(); actual != crc {
				return ErrCRCMismatch{Actual: actual, Expected: crc}
			}

			return nil
		}
	}(close)

	return readerCloser{r, close}
}

type readerCloser struct {
	io.Reader
	close func() error
}

func (r readerCloser) Close() error {
	return r.close()
}
