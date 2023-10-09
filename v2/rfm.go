package librfm

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"io"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	connectURL           = "%s/rr_connect?%s"
	filelistURL          = "%s/rr_filelist?%s"
	fileinfoURL          = "%s/rr_fileinfo?%s"
	mkdirURL             = "%s/rr_mkdir?%s"
	uploadURL            = "%s/rr_upload?%s"
	moveURL              = "%s/rr_move?%s"
	downloadURL          = "%s/rr_download?%s"
	deleteURL            = "%s/rr_delete?%s"
	typeDirectory        = "d"
	typeFile             = "f"
	errDriveNotMounted   = 1
	errDirectoryNotExist = 2
	// TimeFormat is the format of timestamps used by RRF
	TimeFormat = "2006-01-02T15:04:05"
)

type errorResponse struct {
	Err uint64
}

// RRFFileManager provides means to interact with SD card contents on a machine
// using RepRapFirmware (RRF). It will communicate through its HTTP interface.
type RRFFileManager struct {
	httpClient *http.Client
	baseURL    string
	debug      bool
}

// New creates a new instance of RRFFileManager
func New(domain string, port uint64, debug bool) *RRFFileManager {
	tr := &http.Transport{DisableCompression: true}
	return &RRFFileManager{
		httpClient: &http.Client{Transport: tr},
		baseURL:    fmt.Sprintf("http://%s:%d", domain, port),
		debug:      debug,
	}
}

// doGetRequest will perform a GET request on the given URL and return
// the content of the response, a duration on how long it took (including
// setup of connection) or an error in case something went wrong
func (r *RRFFileManager) doGetRequest(ctx context.Context, url string) ([]byte, *time.Duration, error) {
	if r.debug {
		log.Printf("Doing GET request to %s", url)
	}
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, nil, err
	}
	if r.debug {
		dump, _ := httputil.DumpRequestOut(req, false)
		log.Println(string(dump))
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	duration := time.Since(start)
	if r.debug {
		log.Printf("Received response\n%s\n%s", printHeaders(resp), printableBody(body))
	}
	if err != nil {
		return nil, nil, err
	}
	return body, &duration, nil
}

// doPostRequest will perform a POST request on the given URL and return
// the content of the response, a duration on long it tool (including
// setup of connection) or an error in case something went wrong
func (r *RRFFileManager) doPostRequest(ctx context.Context, url string, content io.Reader, contentType string) ([]byte, *time.Duration, error) {
	if r.debug {
		log.Printf("Doing POST request to %s", url)
	}
	start := time.Now()

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, content)
	if err != nil {
		return nil, nil, err
	}
	req.Header.Set("Content-Type", contentType)
	if r.debug {
		dump, _ := httputil.DumpRequestOut(req, true)
		log.Println(string(dump))
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	duration := time.Since(start)
	if r.debug {
		log.Printf("Received response\n%s\n%s", printHeaders(resp), printableBody(body))
	}
	if err != nil {
		return nil, nil, err
	}
	return body, &duration, nil
}

func printHeaders(resp *http.Response) string {
	var sb strings.Builder
	for k, v := range resp.Header {
		sb.WriteString(fmt.Sprintf("%s: %s\n", k, v))
	}
	return sb.String()
}

func printableBody(body []byte) string {
	contentType := http.DetectContentType(body)
	if !isText(contentType) {
		return fmt.Sprintf("Content-Type (inferred): %s (binary data)", contentType)
	}
	return fmt.Sprintf("Content-Type (inferred): %s\n\n%s", contentType, string(body))
}

func isText(contentType string) bool {
	return strings.HasPrefix(contentType, "text/")
}

func (r *RRFFileManager) checkError(action string, resp []byte, err error) error {
	if err != nil {
		return err
	}

	var errResp errorResponse
	err = json.Unmarshal(resp, &errResp)
	if err != nil {
		return err
	}
	if errResp.Err != 0 {
		return fmt.Errorf("Failed to perform: %s", action)
	}

	return nil
}

func (r *RRFFileManager) getTimestamp() string {
	return time.Now().Format(TimeFormat)
}

// Connect establishes a connection to RepRapFirmware
func (r *RRFFileManager) Connect(ctx context.Context, password string) error {
	vals := url.Values{}
	vals.Set("password", password)
	vals.Set("time", r.getTimestamp())
	_, _, err := r.doGetRequest(ctx, fmt.Sprintf(connectURL, r.baseURL, vals.Encode()))
	return err
}

// Fileinfo returns information on a given file or an error if the file does not exist
func (r *RRFFileManager) Fileinfo(ctx context.Context, path string) (*Fileinfo, error) {
	vals := url.Values{}
	vals.Set("name", path)
	body, _, err := r.doGetRequest(ctx, fmt.Sprintf(fileinfoURL, r.baseURL, vals.Encode()))
	if err != nil {
		return nil, err
	}

	var f Fileinfo
	err = json.Unmarshal(body, &f)
	if err != nil {
		return nil, err
	}

	if f.Err != 0 {
		return nil, ErrFileNotFound
	}

	return &f, nil
}

// Filelist will download a list of all files (also including directories) for the given path.
// If recursive is true it will also populate the field Subdirs of Filelist to contain the full
// tree.
func (r *RRFFileManager) Filelist(ctx context.Context, dir string, recursive bool) (*Filelist, error) {
	fl, err := r.getFullFilelist(ctx, dir, 0)
	if err != nil {
		return nil, err
	}
	if recursive {
		for _, f := range fl.Files {
			if !f.IsDir() {

				// Directories come first so once we get here we can skip the remaining
				break
			}
			subfl, err := r.Filelist(ctx, fmt.Sprintf("%s/%s", fl.Dir, f.Name), true)
			if err != nil {
				return nil, err
			}
			fl.Subdirs = append(fl.Subdirs, subfl)
		}
	}
	return fl, nil
}

func (r *RRFFileManager) getFullFilelist(ctx context.Context, dir string, first uint64) (*Filelist, error) {
	vals := url.Values{}
	vals.Set("dir", dir)
	vals.Set("first", strconv.FormatUint(first, 10))
	body, _, err := r.doGetRequest(ctx, fmt.Sprintf(filelistURL, r.baseURL, vals.Encode()))
	if err != nil {
		return nil, err
	}

	var fl Filelist
	err = json.Unmarshal(body, &fl)
	if err != nil {
		return nil, err
	}
	if fl.Err == errDirectoryNotExist {
		return nil, ErrDirectoryNotFound
	}
	if fl.Err == errDriveNotMounted {
		return nil, ErrDriveNotMounted
	}

	// If the response signals there is more to fetch do it recursively
	if fl.Next > 0 {
		moreFiles, err := r.getFullFilelist(ctx, dir, fl.Next)
		if err != nil {
			return nil, err
		}
		fl.Files = append(fl.Files, moreFiles.Files...)
	}

	// Sort folders first and by name
	sort.SliceStable(fl.Files, func(i, j int) bool {

		// Both same type so compare by name
		if fl.Files[i].Type == fl.Files[j].Type {
			return fl.Files[i].Name < fl.Files[j].Name
		}

		// Different types -> sort folders first
		return fl.Files[i].Type == typeDirectory
	})
	fl.Subdirs = make([]*Filelist, 0)
	return &fl, nil
}

// GetFile downloads a file with the given path also returning the duration of this action
func (r *RRFFileManager) Download(ctx context.Context, path string) ([]byte, *time.Duration, error) {
	vals := url.Values{}
	vals.Set("name", path)
	return r.doGetRequest(ctx, fmt.Sprintf(downloadURL, r.baseURL, vals.Encode()))
}

// Mkdir creates a new directory with the given path
func (r *RRFFileManager) Mkdir(ctx context.Context, path string) error {
	vals := url.Values{}
	vals.Set("dir", path)
	resp, _, err := r.doGetRequest(ctx, fmt.Sprintf(mkdirURL, r.baseURL, vals.Encode()))
	return r.checkError(fmt.Sprintf("Mkdir %s", path), resp, err)
}

// Move renames or moves a file or directory (only within the same SD card)
func (r *RRFFileManager) Move(ctx context.Context, oldpath, newpath string) error {
	vals := url.Values{}
	vals.Set("old", oldpath)
	vals.Set("new", newpath)
	resp, _, err := r.doGetRequest(ctx, fmt.Sprintf(moveURL, r.baseURL, vals.Encode()))
	return r.checkError(fmt.Sprintf("Rename %s to %s", oldpath, newpath), resp, err)
}

// Delete removes the given path. It will fail for non-empty directories.
func (r *RRFFileManager) Delete(ctx context.Context, path string) error {
	vals := url.Values{}
	vals.Set("name", path)
	resp, _, err := r.doGetRequest(ctx, fmt.Sprintf(deleteURL, r.baseURL, vals.Encode()))
	return r.checkError(fmt.Sprintf("Delete %s", path), resp, err)
}

// Upload uploads a new file to the given path on the SD card
func (r *RRFFileManager) Upload(ctx context.Context, path string, content io.Reader) (*time.Duration, error) {
	content, crc32, err := getCRC32(content)
	if err != nil {
		return nil, err
	}
	vals := url.Values{}
	vals.Set("name", path)
	vals.Set("time", r.getTimestamp())
	vals.Set("crc32", crc32)
	uri := fmt.Sprintf(uploadURL, r.baseURL, vals.Encode())
	resp, duration, err := r.doPostRequest(ctx, uri, content, "application/octet-stream")
	return duration, r.checkError(fmt.Sprintf("Uploading file to %s", path), resp, err)
}

func getCRC32(content io.Reader) (io.Reader, string, error) {

	// Slurp the io.Reader back into a byte slice
	b, err := io.ReadAll(content)
	if err != nil {
		return nil, "", err
	}
	// Calculate CRC32 with IEEE polynomials
	c := crc32.ChecksumIEEE(b)

	// Create little-endian represenation of CRC32 sum
	le := make([]byte, crc32.Size)
	binary.BigEndian.PutUint32(le, c)

	return bytes.NewReader(b), hex.EncodeToString(le), nil
}
