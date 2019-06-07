package librfm

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"sort"
	"time"
)

const (
	connectURL    = "%s/rr_connect?password=%s&time=%s"
	filelistURL   = "%s/rr_filelist?dir=%s"
	fileinfoURL   = "%s/rr_fileinfo?name=%s"
	mkdirURL      = "%s/rr_mkdir?dir=%s"
	uploadURL     = "%s/rr_upload?name=%s&time=%s"
	moveURL       = "%s/rr_move?old=%s&new=%s"
	downloadURL   = "%s/rr_download?name=%s"
	deleteURL     = "%s/rr_delete?name=%s"
	typeDirectory = "d"
	typeFile      = "f"
	// TimeFormat is the format of timestamps used by RRF
	TimeFormat = "2006-01-02T15:04:05"
)

type errorResponse struct {
	Err uint64
}

type rrffm struct {
	httpClient *http.Client
	baseURL    string
}

// New creates a new instance of RRFFileManager
func New(domain string, port uint64) RRFFileManager {
	tr := &http.Transport{DisableCompression: true}
	return &rrffm{
		httpClient: &http.Client{Transport: tr},
		baseURL:    fmt.Sprintf("http://%s:%d", domain, port),
	}
}

// download will perform a GET request on the given URL and return
// the content of the response, a duration on how long it took (including
// setup of connection) or an error in case something went wrong
func (rrffm *rrffm) doGetRequest(url string) ([]byte, *time.Duration, error) {
	start := time.Now()
	resp, err := rrffm.httpClient.Get(url)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	duration := time.Since(start)
	if err != nil {
		return nil, nil, err
	}
	return body, &duration, nil
}

func (rrffm *rrffm) doPostRequest(url string, content io.Reader, contentType string) ([]byte, *time.Duration, error) {
	start := time.Now()
	resp, err := rrffm.httpClient.Post(url, contentType, content)
	if err != nil {
		return nil, nil, err
	}
	defer resp.Body.Close()

	body, err := ioutil.ReadAll(resp.Body)
	duration := time.Since(start)
	if err != nil {
		return nil, nil, err
	}
	return body, &duration, nil
}

func (rrffm *rrffm) checkError(action string, resp []byte, err error) error {
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

func (rrffm *rrffm) getTimestamp() string {
	return time.Now().Format(TimeFormat)
}

func (rrffm *rrffm) Connect(password string) error {
	_, _, err := rrffm.doGetRequest(fmt.Sprintf(connectURL, rrffm.baseURL, url.QueryEscape(password), url.QueryEscape(rrffm.getTimestamp())))
	return err
}

func (rrffm *rrffm) Filelist(dir string) (*Filelist, error) {
	return rrffm.getFileListRecursively(url.QueryEscape(dir), 0)
}

func (rrffm *rrffm) Fileinfo(path string) (*Fileinfo, error) {
	body, _, err := rrffm.doGetRequest(fmt.Sprintf(fileinfoURL, rrffm.baseURL, url.QueryEscape(path)))
	if err != nil {
		return nil, err
	}

	var f Fileinfo
	err = json.Unmarshal(body, &f)
	if err != nil {
		return nil, err
	}

	if f.Err != 0 {
		return nil, FileNotFoundError
	}

	return &f, nil
}

func (rrffm *rrffm) getFileListRecursively(dir string, first uint64) (*Filelist, error) {

	body, _, err := rrffm.doGetRequest(fmt.Sprintf(filelistURL, rrffm.baseURL, dir))
	if err != nil {
		return nil, err
	}

	var fl Filelist
	err = json.Unmarshal(body, &fl)
	if err != nil {
		return nil, err
	}

	// If the response signals there is more to fetch do it recursively
	if fl.next > 0 {
		moreFiles, err := rrffm.getFileListRecursively(dir, fl.next)
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
	return &fl, nil
}

func (rrffm *rrffm) Download(filepath string) ([]byte, *time.Duration, error) {
	return rrffm.doGetRequest(fmt.Sprintf(downloadURL, rrffm.baseURL, url.QueryEscape(filepath)))
}

func (rrffm *rrffm) Mkdir(path string) error {
	resp, _, err := rrffm.doGetRequest(fmt.Sprintf(mkdirURL, rrffm.baseURL, url.QueryEscape(path)))
	return rrffm.checkError(fmt.Sprintf("Mkdir %s", path), resp, err)
}

func (rrffm *rrffm) Move(oldpath, newpath string) error {
	resp, _, err := rrffm.doGetRequest(fmt.Sprintf(moveURL, rrffm.baseURL, url.QueryEscape(oldpath), url.QueryEscape(newpath)))
	return rrffm.checkError(fmt.Sprintf("Rename %s to %s", oldpath, newpath), resp, err)
}

func (rrffm *rrffm) MoveOverwrite(oldpath, newpath string) error {
	if _, err := rrffm.Fileinfo(newpath); err == nil {
		if err := rrffm.Delete(newpath); err != nil {
			return err
		}
	}
	return rrffm.Move(oldpath, newpath)
}

func (rrffm *rrffm) Delete(path string) error {
	resp, _, err := rrffm.doGetRequest(fmt.Sprintf(deleteURL, rrffm.baseURL, url.QueryEscape(path)))
	return rrffm.checkError(fmt.Sprintf("Delete %s", path), resp, err)
}

func (rrffm *rrffm) DeleteRecursive(path string) error {
	fl, err := rrffm.Filelist(path)
	if err != nil {
		return err
	}
	for _, f := range fl.Files {
		if !f.IsDir() {

			// Directories come first so once we get here we can skip the remaining
			break
		}
		if err = rrffm.DeleteRecursive(fmt.Sprintf("%s/%s", fl.Dir, f.Name)); err != nil {
			return err
		}
	}
	for _, f := range fl.Files {
		rrffm.Delete(f.Name)
	}
	return nil
}

func (rrffm *rrffm) Upload(path string, content io.Reader) (*time.Duration, error) {
	resp, duration, err := rrffm.doPostRequest(fmt.Sprintf(uploadURL, rrffm.baseURL, url.QueryEscape(path), url.QueryEscape(rrffm.getTimestamp())), content, "application/octet-stream")
	return duration, rrffm.checkError(fmt.Sprintf("Uploading file to %s", path), resp, err)
}
