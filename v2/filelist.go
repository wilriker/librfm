package librfm

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

type localTime struct {
	Time time.Time
}

func (lt *localTime) UnmarshalJSON(b []byte) (err error) {
	// Parse date string in local time (it does not provide any timezone information)
	lt.Time, err = time.ParseInLocation(`"`+TimeFormat+`"`, string(b), time.Local)
	return err
}

// File resembles the JSON object returned in the files property of the rr_filelist response
type File struct {
	// Type of file - can be file or directory
	Type string
	// Name of file
	Name string
	// Size of file in bytes
	Size uint64
	// Timestamp corresponds to last modification date
	Timestamp localTime `json:"date"`
}

// Date returns the last modification date of a file/directory
func (f *File) Date() time.Time {
	return f.Timestamp.Time
}

// IsDir returns true if the File instance is a directory, false otherwise
func (f *File) IsDir() bool {
	return f.Type == typeDirectory
}

// IsFile returns true if the File instance is a file, false otherwise
func (f *File) IsFile() bool {
	return f.Type == typeFile
}

// ErrDirectoryNotFound is the error returned if a directory was not found
var ErrDirectoryNotFound = errors.New("Directory not found")

// ErrDriveNotMounted is the error returned if the requested drive is not mounted
var ErrDriveNotMounted = errors.New("Drive not mounted")

// Filelist resembled the JSON object in rr_filelist
type Filelist struct {
	Dir     string
	Files   []File
	Next    uint64
	Err     uint64
	Subdirs []*Filelist
	once    sync.Once
	index   map[string]bool
}

// Contains checks for a path to exist in this filelist
func (f *Filelist) Contains(path string) bool {
	f.once.Do(f.buildIndex)
	return f.index[path]
}

func (f *Filelist) buildIndex() {
	f.index = make(map[string]bool)

	// Init index of subdirs
	for _, subdir := range f.Subdirs {
		subdir.buildIndex()
		for k, v := range subdir.index {
			f.index[k] = v
		}
		f.index[subdir.Dir] = true
	}
	for _, file := range f.Files {
		if file.IsDir() {
			continue
		}
		f.index[fmt.Sprintf("%s/%s", f.Dir, file.Name)] = true
	}
	f.index[f.Dir] = true
}
