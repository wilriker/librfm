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
	Err              uint64
	Size             uint64
	Timestamp        localTime `json:"lastModified"`
	Height           float64
	FirstLayerHeight float64
	LayerHeight      float64
	PrintTime        uint64
	Filament         []float64
	GeneratedBy      string
}

// LastModified returns the last modification time of this file
func (f *Fileinfo) LastModified() time.Time {
	return f.Timestamp.Time
}
