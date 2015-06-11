package vpk

import (
	"bytes"
	"io"
	"io/ioutil"
	"net/http"
	"os"
	"path"
	"strings"
	"time"
)

var _ http.FileSystem = (*VPK)(nil)

func (vpk *VPK) Open(rel string) (http.File, error) {
	if ent := vpk.Entry(rel); ent != nil {
		return vpk.openFile(ent)
	}
	return vpk.openDir(rel), nil
}

func (vpk *VPK) openFile(ent Entry) (http.File, error) {
	r, err := ent.Open()
	if err != nil {
		return nil, err
	}
	b, err := ioutil.ReadAll(r)
	if err != nil {
		r.Close()
		return nil, err
	}
	err = r.Close()
	if err != nil {
		return nil, err
	}

	return &httpFile{bytes.NewReader(b), httpFileInfo{
		name:    path.Base(ent.Rel()),
		isDir:   false,
		modTime: vpk.modtime,
		size:    int64(len(b)),
	}}, nil
}

type httpFile struct {
	*bytes.Reader
	info httpFileInfo
}

func (f *httpFile) Stat() (os.FileInfo, error) {
	return &f.info, nil
}

func (f *httpFile) Close() error {
	return nil
}

func (f *httpFile) Readdir(n int) ([]os.FileInfo, error) {
	return nil, os.ErrInvalid
}

func (vpk *VPK) openDir(rel string) http.File {
	return &httpDir{vpk, rel, httpFileInfo{
		name:    path.Base(rel),
		isDir:   true,
		modTime: vpk.modtime,
		size:    0,
	}, nil}
}

type httpDir struct {
	vpk   *VPK
	rel   string
	info  httpFileInfo
	files []os.FileInfo
}

func (d *httpDir) Read([]byte) (int, error) {
	return 0, os.ErrInvalid
}

func (d *httpDir) Seek(int64, int) (int64, error) {
	return 0, os.ErrInvalid
}

func (d *httpDir) Stat() (os.FileInfo, error) {
	return &d.info, nil
}

func (d *httpDir) Close() error {
	return nil
}

func (f *httpDir) Readdir(n int) ([]os.FileInfo, error) {
	if n <= 0 {
		return f.readdir()
	}

	if f.files == nil {
		var err error
		f.files, err = f.readdir()
		if err != nil {
			return nil, err
		}
	}

	if len(f.files) < n {
		files := f.files
		f.files = nil
		return files, io.EOF
	}
	files := f.files[:n]
	f.files = f.files[n:]
	return files, nil
}

func (d *httpDir) readdir() ([]os.FileInfo, error) {
	var dirs []string
	var files []os.FileInfo

	dir := d.rel
	if dir == "" {
		dir = " "
	}
	prefix := d.rel + "/"
	if prefix == "/" {
		prefix = ""
	}
	for _, e := range d.vpk.entries {
		if e.dir == d.rel {
			rel := prefix
			if e.base != " " {
				rel += e.base
			}
			if e.ext != " " {
				rel += "." + e.ext
			}
			f, err := d.vpk.openFile(&vpkFileEntry{d.vpk.opener, d.vpk.treeLength, rel, *e.vpk, e.pre})
			if err != nil {
				return nil, err
			}
			fi, err := f.Stat()
			if err != nil {
				return nil, err
			}
			files = append(files, fi)
		} else if strings.HasPrefix(e.dir, prefix) {
			dir := e.dir
			if i := strings.Index(dir[len(prefix):], "/"); i != -1 {
				dir = dir[:len(prefix)+i]
			}
			found := false
			for _, s := range dirs {
				if s == dir {
					found = true
					break
				}
			}
			if !found {
				dirs = append(dirs, dir)
			}
		}
	}

	for _, dir := range dirs {
		fi, err := d.vpk.openDir(dir).Stat()
		if err != nil {
			return nil, err
		}
		files = append(files, fi)
	}

	return files, nil
}

type httpFileInfo struct {
	name    string
	isDir   bool
	modTime time.Time
	size    int64
}

func (fi *httpFileInfo) Name() string {
	return fi.name
}

func (fi *httpFileInfo) IsDir() bool {
	return fi.isDir
}

func (fi *httpFileInfo) ModTime() time.Time {
	return fi.modTime
}

func (fi *httpFileInfo) Size() int64 {
	return fi.size
}

func (fi *httpFileInfo) Mode() os.FileMode {
	mode := os.FileMode(0444)
	if fi.isDir {
		mode |= os.ModeDir
	}
	return mode
}

func (fi *httpFileInfo) Sys() interface{} {
	return nil
}
