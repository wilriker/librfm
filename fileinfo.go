package librfm

import (
	"errors"
	"time"
)

// ErrFileNotFound is the error returned in case a call to
// the rr_fileinfo interface was successful but returned err != 0
var ErrFileNotFound = errors.New("File not found")

// Fileinfo is the structure returned at rr_fileinfo interface
type Fileinfo struct {
	// Err holds a numeric error code where 0 means no error
	Err uint64
	// Size is the size of a file in bytes (0 for directories)
	Size      uint64
	Timestamp localTime `json:"lastModified"`
	// Height in mm for a job file
	Height float64
	// FirstLayerHeight in mm for a job file
	FirstLayerHeight float64
	// LayerHeight in mm for a job file
	LayerHeight float64
	// PrintTime in seconds for a job file
	PrintTime uint64
	// Filament contains an array of used filaments in mm
	Filament []float64
	// GeneratedBy returns the string which application created the job file
	GeneratedBy string
}

// LastModified returns the last modification time of this file
func (f *Fileinfo) LastModified() time.Time {
	return f.Timestamp.Time
}
