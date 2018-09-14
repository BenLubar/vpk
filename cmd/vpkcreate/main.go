package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/BenLubar/vpk"
)

func init() {
	flag.Usage = usage
}

func usage() {
	fmt.Fprintf(os.Stderr, "Usage: vpkcreate [vpkname.vpk] [file1] [file2] [file3]\n")
	fmt.Fprintf(os.Stderr, "Usage: vpkcreate -M [size] [vpkname] [file1] [file2] [file3]\n\n")
	flag.PrintDefaults()
	os.Exit(2)
}

func main() {
	multi := flag.Int64("M", -1, "max size for multipart archives (last file can continue past this size)")

	flag.Parse()

	if flag.NArg() <= 1 {
		flag.Usage()
	}

	var creator vpk.Creator
	if *multi < 0 {
		creator = vpk.SingleVPKCreator(flag.Arg(0))
	} else {
		creator = vpk.MultiVPKCreator(flag.Arg(0))
	}

	var contents []vpk.Entry

	for _, name := range flag.Args()[1:] {
		err := filepath.Walk(name, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}

			if !info.IsDir() {
				contents = append(contents, entry(path))
			}

			return nil
		})

		if err != nil {
			fmt.Fprintf(os.Stderr, "%v\n", err)
			os.Exit(1)
		}
	}

	err := vpk.Create(creator, contents, *multi)
	if err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

type entry string

func (e entry) Rel() string                  { return string(e) }
func (e entry) Open() (io.ReadCloser, error) { return os.Open(string(e)) }
