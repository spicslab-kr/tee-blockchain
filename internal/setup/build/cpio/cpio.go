// Package cpio writes the application archive that a confidential VM's initrd carries, so that the same input gives
// the same bytes on every machine.
//
// GNU cpio cannot be used for this. Version 2.13 (Amazon Linux) and 2.15 (Ubuntu) pack the same files into different
// bytes because they count directory links differently, and both copy the files' times, owners and inode numbers
// from the disk. Here every header field that is not the content is fixed: entries are sorted by name (as bytes),
// times are 0, owner and group are 0, inode numbers are the 1-based position in the sorted list, directories are
// 0755 with two links, files are 0644 or 0755 with one link. The format is "newc" (magic 070701), the one the Linux
// kernel unpacks from an initrd.
//
// The output is byte-for-byte what mkcpio.py, the packing script of the October-November SEV-SNP runs, wrote for the
// same tree. That script no longer exists; the two archives it wrote are kept in testdata/mkcpio-golden-*.cpio, and
// cpio_test.go holds this package to them (testdata/README.md).
//
// The role does not matter, so this package is shared by every binary that builds an image.
package cpio

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Entry is one directory or regular file in the archive. Name is relative to the archive root, with "/" separators;
// the root itself is ".".
type Entry struct {
	Name       string
	Dir        bool
	Executable bool // file mode 0755 instead of 0644; ignored for directories
	Data       []byte
}

const (
	modeDir     = 0o040755
	modeExec    = 0o100755
	modeFile    = 0o100644
	newcMagic   = "070701"
	headerSize  = 110 // magic plus thirteen 8-digit hex fields
	trailerName = "TRAILER!!!"
	blockSize   = 512
)

// Write writes entries as a newc archive with fixed metadata, then the trailer, padded to 512 bytes. The order of
// entries does not matter: they are sorted by name first. Two entries with the same name are an error.
func Write(w io.Writer, entries []Entry) error {
	sorted := append([]Entry(nil), entries...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Name < sorted[j].Name }) // Go compares strings as bytes
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Name == sorted[i-1].Name {
			return fmt.Errorf("cpio: %q appears twice", sorted[i].Name)
		}
	}
	var buf bytes.Buffer
	for i, e := range sorted {
		if e.Name == "" || strings.Contains(e.Name, "\x00") || strings.Contains(e.Name, `\`) {
			return fmt.Errorf("cpio: bad entry name %q", e.Name)
		}
		mode, nlink, data := uint32(modeFile), uint32(1), e.Data
		switch {
		case e.Dir:
			mode, nlink, data = modeDir, 2, nil
		case e.Executable:
			mode = modeExec
		}
		writeEntry(&buf, uint32(i+1), mode, nlink, e.Name, data)
	}
	writeEntry(&buf, 0, 0, 1, trailerName, nil)
	buf.Write(make([]byte, pad(buf.Len(), blockSize)))
	_, err := w.Write(buf.Bytes())
	return err
}

// Bytes is Write into memory.
func Bytes(entries []Entry) ([]byte, error) {
	var buf bytes.Buffer
	if err := Write(&buf, entries); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// writeEntry writes one header, the NUL-terminated name and the data, each padded to 4 bytes. The fields after the
// magic are, in order: ino, mode, uid, gid, nlink, mtime, filesize, devmajor, devminor, rdevmajor, rdevminor,
// namesize, check.
func writeEntry(buf *bytes.Buffer, ino, mode, nlink uint32, name string, data []byte) {
	nameSize := uint32(len(name) + 1)
	fmt.Fprintf(buf, "%s%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X%08X",
		newcMagic, ino, mode, 0, 0, nlink, 0, uint32(len(data)), 0, 0, 0, 0, nameSize, 0)
	buf.WriteString(name)
	buf.WriteByte(0)
	buf.Write(make([]byte, pad(headerSize+int(nameSize), 4)))
	buf.Write(data)
	buf.Write(make([]byte, pad(len(data), 4)))
}

func pad(n, to int) int { return (to - n%to) % to }

// FromDir reads a directory tree into entries: the root as ".", every directory below it, and every regular file. A
// file is executable when any of its execute bits is set. Symbolic links, devices and other special files are
// refused rather than followed or skipped, so what is on disk and what is archived cannot quietly differ.
//
// Windows has no execute bits, so on Windows every file reads as 0644; build the entries by hand there.
func FromDir(root string) ([]Entry, error) {
	var out []Entry
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.IsDir():
			out = append(out, Entry{Name: name, Dir: true})
		case info.Mode().IsRegular():
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			out = append(out, Entry{Name: name, Executable: info.Mode().Perm()&0o111 != 0, Data: data})
		default:
			return fmt.Errorf("cpio: %s is not a regular file or directory (%s)", name, info.Mode().Type())
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(out) == 0 {
		return nil, errors.New("cpio: empty tree")
	}
	return out, nil
}

// Placement says where the application archive goes relative to the base initrd. The kernel unpacks every archive
// in the initrd in turn, so both orders put /opt/app in the initial root file system; they differ in what boots.
type Placement int

const (
	// AppFirst puts the application archive before the base initrd. This is what the Intel TDX fixtures were recorded
	// with (no padding needed: the archive is padded to 512 bytes, so the base starts aligned). An unpadded append
	// failed on TDX in 2026-10 (docs/build/tdx.md, "Before or after the base initrd"); that was not repeated, and the alignment reason under
	// AppLast, measured on SEV-SNP with the same base initrd, is the likely cause.
	AppFirst Placement = iota
	// AppLast appends the application archive after the base initrd, zero-padding the base to a multiple of 4 bytes
	// first. The kernel only recognises an uncompressed archive after a compressed segment at a 4-byte-aligned offset
	// (init/initramfs.c). Measured on GCP SEV-SNP, 2026-11-11: with the Ubuntu base initrd (length mod 4 = 3) and no
	// padding, the kernel logged "Initramfs unpacking failed: invalid magic at start of compressed archive", dropped the
	// archive and still booted; with the padding the archive was unpacked. (Record: the old out-of-repository scripts, no longer
	// available, appended without padding: cat i.bin app.cpio.)
	AppLast
)

// alignment is the boundary the kernel requires an uncompressed initramfs archive to start on.
const alignment = 4

// Join concatenates the base initrd and the application archive in the given order. The result is what gets measured
// (RTMR2 on TDX, a vTPM PCR on SEV-SNP), so the order and the padding are part of the measurement. With AppLast the base
// is followed by zero bytes up to a multiple of 4 (the kernel skips NUL bytes between segments). With AppFirst no
// padding is needed: Write pads the archive to 512 bytes, so the base starts aligned.
func Join(base, app []byte, where Placement) []byte {
	out := make([]byte, 0, len(base)+alignment+len(app))
	if where == AppFirst {
		return append(append(out, app...), base...)
	}
	out = append(out, base...)
	out = append(out, make([]byte, pad(len(base), alignment))...)
	return append(out, app...)
}
