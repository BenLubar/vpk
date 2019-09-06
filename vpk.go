// Package vpk implements file operations on Valve Software's VPK format.
package vpk

import (
	"bufio"
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type entrysort []entrypath

func (s entrysort) Len() int      { return len(s) }
func (s entrysort) Swap(i, j int) { s[i], s[j] = s[j], s[i] }
func (s entrysort) Less(i, j int) bool {
	return s.less(s[i], s[j])
}
func (s entrysort) less(x, y entrypath) bool {
	// radix sort on ext > dir > base
	if x.ext < y.ext {
		return true
	}
	if x.ext > y.ext {
		return false
	}
	if x.dir < y.dir {
		return true
	}
	if x.dir > y.dir {
		return false
	}
	return x.base < y.base
}
func (s entrysort) find(dir, base, ext string) *entrypath {
	e := entrypath{dir: dir, base: base, ext: ext}
	if i := sort.Search(len(s), func(i int) bool {
		return !s.less(s[i], e)
	}); i < len(s) && s[i].dir == dir && s[i].base == base && s[i].ext == ext {
		return &s[i]
	}

	return nil
}
func splitPath(rel string) (dir, base, ext string) {
	rel = strings.ToLower(rel)
	dir = filepath.ToSlash(filepath.Dir(rel))
	base = filepath.Base(rel)
	ext = filepath.Ext(rel)

	base = base[:len(base)-len(ext)]
	if ext != "" {
		ext = ext[1:]
	}

	if dir == "" || dir == "." {
		dir = " "
	}
	if base == "" {
		base = " "
	}
	if ext == "" {
		ext = " "
	}

	return
}

type entrypath struct {
	// The filename is of the format dir/base.ext. If any component is empty
	// in the actual filename, it is represented by a single space here.
	dir  string
	base string
	ext  string

	vpk *vpkentry
	pre []byte
	ent Entry
}

type vpkentry struct {
	// An IEEE 32-bit CRC checksum of the entire file.
	CRC uint32
	// The number of bytes of data from the file that are included directly
	// after this structure in the directory tree.
	PreloadBytes uint16
	// The index of the archive this file is stored in. 0x7fff is this file,
	// negative numbers are not allowed.
	ArchiveIndex int16
	// Offset is either relative to the end of the directory tree
	// (ArchiveIndex == 0x7fff) or the beginning of a data-only archive
	// (ArchiveIndex != 0x7fff).
	Offset uint32
	// Length is the amount of data starting from Offset. It does not
	// include preloaded data, which is part of the directory tree.
	Length uint32
	// Terminator is always 0xffff.
	Terminator uint16
}

type VPK struct {
	opener     Opener
	version    uint32
	treeLength uint32
	entries    entrysort
	modtime    time.Time
}

type vpkFileEntry struct {
	o Opener
	l uint32
	r string
	e vpkentry
	p []byte
}

func (e *vpkFileEntry) Rel() string {
	return e.r
}

func (e *vpkFileEntry) Open() (io.ReadCloser, error) {
	if e.e.Length == 0 {
		return crcReader(bytes.NewReader(e.p), func() error { return nil }, e.e.CRC), nil
	}

	var f File
	var err error
	if e.e.ArchiveIndex == 0x7fff {
		f, err = e.o.Main()
		if err != nil {
			if f != nil {
				f.Close()
			}
			return nil, err
		}
		_, err = f.Seek(12+int64(e.l), os.SEEK_CUR)
	} else {
		f, err = e.o.Archive(e.e.ArchiveIndex)
	}
	if err != nil {
		if f != nil {
			f.Close()
		}
		return nil, err
	}

	_, err = f.Seek(int64(e.e.Offset), os.SEEK_CUR)

	return crcReader(io.MultiReader(bytes.NewReader(e.p), io.LimitReader(f, int64(e.e.Length))), f.Close, e.e.CRC), nil
}

// Entry returns the file with the given relative path, or nil if no such file
// exists. The Close method of the io.ReadCloser returned by Entry.Open verifies
// the CRC of the file.
func (v *VPK) Entry(rel string) Entry {
	e := v.entries.find(splitPath(rel))
	if e == nil {
		return nil
	}

	return &vpkFileEntry{v.opener, v.treeLength, rel, *e.vpk, e.pre}
}

// Paths returns a slice containing the relative paths of all files in the VPK.
func (v *VPK) Paths() []string {
	paths := make([]string, len(v.entries))

	for i, e := range v.entries {
		if e.dir != " " {
			paths[i] += e.dir + "/"
		}
		if e.base != " " {
			paths[i] += e.base
		}
		if e.ext != " " {
			paths[i] += "." + e.ext
		}
	}

	return paths
}

type Entry interface {
	// The relative path to this file.
	Rel() string

	// Open the file for reading. Exactly one of the return values must be
	// non-nil. The contents of the io.ReadCloser should be the same each
	// time Open is called.
	Open() (io.ReadCloser, error)
}

type File interface {
	io.Reader
	io.Seeker
	io.Closer
	Stat() (os.FileInfo, error)
}

func Open(o Opener) (*VPK, error) {
	var vpk VPK

	vpk.opener = o

	r, err := o.Main()
	if err != nil {
		return nil, err
	}
	defer r.Close()

	fi, err := r.Stat()
	if err != nil {
		return nil, err
	}

	vpk.modtime = fi.ModTime()

	br := bufio.NewReader(r)

	var magic uint32
	err = binary.Read(br, binary.LittleEndian, &magic)
	if err != nil {
		return nil, err
	}

	if magic != 0x55aa1234 {
		return nil, ErrInvalidMagic
	}

	err = binary.Read(br, binary.LittleEndian, &vpk.version)
	if err != nil {
		return nil, err
	}

	if vpk.version != 1 {
		return nil, ErrUnsupportedVersion(vpk.version)
	}

	// TODO: verify treeLength
	err = binary.Read(br, binary.LittleEndian, &vpk.treeLength)
	if err != nil {
		return nil, err
	}

	for {
		ext, err := br.ReadString(0)
		if err != nil {
			return nil, err
		}
		ext = ext[:len(ext)-1]
		if ext == "" {
			break
		}
		for {
			dir, err := br.ReadString(0)
			if err != nil {
				return nil, err
			}
			dir = dir[:len(dir)-1]
			if dir == "" {
				break
			}
			for {
				base, err := br.ReadString(0)
				if err != nil {
					return nil, err
				}
				base = base[:len(base)-1]
				if base == "" {
					break
				}

				var e vpkentry
				err = binary.Read(br, binary.LittleEndian, &e)
				if err != nil {
					return nil, err
				}

				if e.ArchiveIndex < 0 || e.Terminator != 0xffff {
					return nil, ErrInvalidEntry{
						Dir:  dir,
						Base: base,
						Ext:  ext,
					}
				}

				var pre []byte
				if e.PreloadBytes != 0 {
					pre = make([]byte, e.PreloadBytes)
					_, err = io.ReadFull(br, pre)
					if err != nil {
						return nil, err
					}
				}

				vpk.entries = append(vpk.entries, entrypath{
					dir:  dir,
					base: base,
					ext:  ext,

					vpk: &e,
					pre: pre,
				})
			}
		}
	}

	sort.Sort(vpk.entries)

	return &vpk, nil
}

func Create(c Creator, contents []Entry, maxSize int64) (err error) {
	var entries []entrypath

	hash := crc32.NewIEEE()

	var archive int16
	var offset uint32
	if maxSize < 0 {
		archive = 0x7fff
	}
	for _, c := range contents {
		var e vpkentry
		e.ArchiveIndex = archive
		e.Offset = offset
		e.Terminator = 0xffff

		r, err := c.Open()
		if err != nil {
			return err
		}

		hash.Reset()
		length, err := io.Copy(hash, r)
		if err != nil {
			r.Close()
			return err
		}

		err = r.Close()
		if err != nil {
			return err
		}

		if length != int64(uint32(length)) {
			return ErrFileTooBig
		}

		e.CRC = hash.Sum32()
		e.Length = uint32(length)
		if offset+uint32(length) < offset {
			return ErrFileTooBig
		}
		offset += uint32(length)
		if maxSize >= 0 && int64(offset) >= maxSize {
			offset = 0
			archive++
		}

		dir, base, ext := splitPath(c.Rel())
		entries = append(entries, entrypath{
			dir:  dir,
			base: base,
			ext:  ext,

			ent: c,
			vpk: &e,
		})
	}

	sorted := make(entrysort, len(entries))
	copy(sorted, entries)
	sort.Sort(sorted)
	sorted = append(sorted, entrypath{})

	var buf bytes.Buffer

	writeString := func(s string) {
		if err != nil {
			return
		}
		_, err = buf.WriteString(s)
		if err != nil {
			return
		}
		err = buf.WriteByte(0)
	}
	writeString(sorted[0].ext)
	writeString(sorted[0].dir)
	writeString(sorted[0].base)
	if err != nil {
		return
	}

	for i, e := range sorted[:len(entries)] {
		err = binary.Write(&buf, binary.LittleEndian, e.vpk)
		if err != nil {
			return
		}

		next := sorted[i+1]
		if e.dir != next.dir || e.ext != next.ext {
			writeString("")
			if e.ext != next.ext {
				writeString("")
				writeString(next.ext)
				if next.ext == "" {
					break
				}
			}
			writeString(next.dir)
		}
		writeString(next.base)
	}
	if err != nil {
		return
	}
	if int64(uint32(buf.Len())) != int64(buf.Len()) {
		return ErrFileTooBig
	}

	f, err := c.Main()
	if err != nil {
		return
	}
	defer func() {
		if e := f.Close(); err == nil {
			err = e
		}
	}()

	w := bufio.NewWriter(f)
	defer func() {
		if e := w.Flush(); err == nil {
			err = e
		}
	}()

	err = binary.Write(w, binary.LittleEndian, uint32(0x55aa1234)) // magic
	if err != nil {
		return
	}
	err = binary.Write(w, binary.LittleEndian, uint32(0x1)) // version
	if err != nil {
		return
	}
	err = binary.Write(w, binary.LittleEndian, uint32(buf.Len()))
	if err != nil {
		return
	}
	_, err = buf.WriteTo(w)
	if err != nil {
		return
	}

	copyFile := func(w io.Writer, e entrypath) error {
		r, err := e.ent.Open()
		if err != nil {
			return err
		}
		r = crcReader(r, r.Close, e.vpk.CRC)

		length, err := io.Copy(w, r)
		if err != nil {
			r.Close()
			return err
		}

		if length != int64(e.vpk.Length) {
			r.Close()
			return io.ErrUnexpectedEOF
		}

		return r.Close()
	}

	if maxSize < 0 {
		for _, e := range entries {
			if err = copyFile(w, e); err != nil {
				return
			}
		}
	} else {
		var a io.WriteCloser
		var i int16
		for _, e := range entries {
			if i != e.vpk.ArchiveIndex {
				if a != nil {
					if err = a.Close(); err != nil {
						return
					}
					a = nil
				}
				i = e.vpk.ArchiveIndex
			}
			if a == nil {
				if a, err = c.Archive(i); err != nil {
					return
				}
			}
			if err = copyFile(a, e); err != nil {
				return
			}
		}
	}

	return
}
