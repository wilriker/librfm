package librfm

import (
	"io"
	"time"
)

// RRFFileManager provides means to interact with SD card contents on a machine
// using RepRapFirmware (RRF). It will communicate through its HTTP interface.
type RRFFileManager interface {
	// Connect establishes a connection to RepRapFirmware
	Connect(password string) error

	// Filelist will download a list of all files (also including directories) for the given path.
	// If recursive is true it will also populate the field Subdirs of Filelist to contain the full
	// tree.
	Filelist(path string, recursive bool) (*Filelist, error)

	// Fileinfo returns information on a given file or an error if the file does not exist
	Fileinfo(path string) (*Fileinfo, error)

	// GetFile downloads a file with the given path also returning the duration of this action
	Download(filepath string) ([]byte, *time.Duration, error)

	// Mkdir creates a new directory with the given path
	Mkdir(path string) error

	// Move renames or moves a file or directory (only within the same SD card)
	Move(oldpath, newpath string) error

	// MoveOverwrite will delete the target file first and thus overwriting it
	MoveOverwrite(oldpath, newpath string) error

	// Delete removes the given path. It will fail for non-empty directories.
	Delete(path string) error

	// DeleteRecursive removes the given path recursively. This will also delete directories with all their contents.
	DeleteRecursive(path string) error

	// Upload uploads a new file to the given path on the SD card
	Upload(path string, content io.Reader) (*time.Duration, error)
}
