package main

import (
	"bytes"
	"crypto/sha1"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/BenLubar/vpk"
	"github.com/petar/GoLLRB/llrb"
)

var whitelist = map[string]bool{
	"addonimage.jpg": true,
	"addoninfo.txt":  true,
}

func init() {
	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: vpkcollision [file1.vpk] [file2.vpk] [file3.vpk]\n\n")
		flag.PrintDefaults()
		os.Exit(2)
	}
}

type file struct {
	path string
	vpks []*vpk.VPK
}

func (f *file) Less(item llrb.Item) bool {
	return f.path < item.(*file).path
}

func main() {
	skipSame := flag.Bool("skip-same", false, "don't warn about the exact same file being in multiple VPKs.")

	flag.Parse()

	if flag.NArg() == 0 {
		flag.Usage()
	}

	reverse := make(map[*vpk.VPK]string)
	files := llrb.New()

	for _, name := range flag.Args() {
		var opener vpk.Opener
		if strings.HasSuffix(name, "_dir.vpk") {
			opener = vpk.MultiVPK(name[:len(name)-len("_dir.vpk")])
		} else {
			opener = vpk.SingleVPK(name)
		}

		v, err := vpk.Open(opener)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", name, err)
			os.Exit(2)
		}

		reverse[v] = name

		for _, path := range v.Paths() {
			f := &file{path: path}
			if item := files.Get(f); item != nil {
				f = item.(*file)
			} else {
				files.InsertNoReplace(f)
			}
			f.vpks = append(f.vpks, v)
		}
	}

	exitStatus := 0
	files.AscendGreaterOrEqual(files.Min(), func(item llrb.Item) bool {
		f := item.(*file)
		if whitelist[f.path] {
			return true
		}
		if len(f.vpks) == 1 {
			return true
		}

		hashes := make([][]byte, 0, len(f.vpks))
		for _, v := range f.vpks {
			hash, err := doHash(v.Entry(f.path))
			if err != nil {
				fmt.Fprintf(os.Stderr, "%s: %s: %v\n", reverse[v], f.path, err)
				os.Exit(2)
			}
			hashes = append(hashes, hash)
		}

		if *skipSame {
			any := false
			for _, h := range hashes[1:] {
				if !bytes.Equal(hashes[0], h) {
					any = true
					break
				}
			}
			if !any {
				return true
			}
		}

		fmt.Printf("%s:\n", f.path)
		for i, h := range hashes {
			fmt.Printf("%s: %x\n", reverse[f.vpks[i]], h)
		}
		fmt.Printf("\n")
		exitStatus = 1

		return true
	})

	os.Exit(exitStatus)
}

func doHash(e vpk.Entry) (hash []byte, err error) {
	r, err := e.Open()
	if err != nil {
		return
	}
	defer func() {
		if e := r.Close(); err == nil {
			err = e
		}
	}()

	h := sha1.New()
	_, err = io.Copy(h, r)
	if err != nil {
		return
	}

	return h.Sum(nil), nil
}
