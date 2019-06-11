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

// file resembles the JSON object returned in the files property of the rr_filelist response
type file struct {
	Type      string
	Name      string
	Size      uint64
	Timestamp localTime `json:"date"`
}

func (f *file) Date() time.Time {
	return f.Timestamp.Time
}

func (f *file) IsDir() bool {
	return f.Type == typeDirectory
}

func (f *file) IsFile() bool {
	return f.Type == typeFile
}

var DirectoryNotFoundError = errors.New("Directory not found")
var DriveNotMountedError = errors.New("Drive not mounted")

// Filelist resembled the JSON object in rr_filelist
type Filelist struct {
	Dir     string
	Files   []file
	next    uint64
	err     uint64
	Subdirs []Filelist
	once    sync.Once
	index   map[string]struct{}
}

// Contains checks for a path to exist in this filelist
func (f *Filelist) Contains(path string) bool {
	f.once.Do(f.buildIndex)
	if _, found := f.index[path]; found {
		return true
	}
	return false
}

func (f *Filelist) buildIndex() {
	f.index = make(map[string]struct{})

	// Init index of subdirs
	for _, subdir := range f.Subdirs {
		subdir.buildIndex()
		for k, v := range subdir.index {
			f.index[k] = v
		}
	}
	for _, file := range f.Files {
		if file.IsDir() {
			continue
		}
		f.index[fmt.Sprintf("%s/%s", f.Dir, file.Name)] = struct{}{}
	}
}
