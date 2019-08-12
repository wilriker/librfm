package librfm

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"net/http/httputil"
	"net/url"
	"sort"
	"time"
)

const (
	connectURL           = "%s/rr_connect?password=%s&time=%s"
	filelistURL          = "%s/rr_filelist?dir=%s"
	fileinfoURL          = "%s/rr_fileinfo?name=%s"
	mkdirURL             = "%s/rr_mkdir?dir=%s"
	uploadURL            = "%s/rr_upload?name=%s&time=%s"
	moveURL              = "%s/rr_move?old=%s&new=%s"
	downloadURL          = "%s/rr_download?name=%s"
	deleteURL            = "%s/rr_delete?name=%s"
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

type rrffm struct {
	httpClient *http.Client
	baseURL    string
	debug      bool
}

// New creates a new instance of RRFFileManager
func New(domain string, port uint64, debug bool) RRFFileManager {
	tr := &http.Transport{DisableCompression: true}
	return &rrffm{
		httpClient: &http.Client{Transport: tr},
		baseURL:    fmt.Sprintf("http://%s:%d", domain, port),
		debug:      debug,
	}
}

// doGetRequest will perform a GET request on the given URL and return
// the content of the response, a duration on how long it took (including
// setup of connection) or an error in case something went wrong
func (r *rrffm) doGetRequest(url string) ([]byte, *time.Duration, error) {
	if r.debug {
		log.Printf("Doing GET request to %s", url)
	}
	start := time.Now()

	req, err := http.NewRequest(http.MethodGet, url, nil)
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

	body, err := ioutil.ReadAll(resp.Body)
	duration := time.Since(start)
	if r.debug {
		log.Printf("Received response\n%s", string(body))
	}
	if err != nil {
		return nil, nil, err
	}
	return body, &duration, nil
}

// doPostRequest will perform a POST request on the given URL and return
// the content of the response, a duration on long it tool (including
// setup of connection) or an error in case something went wrong
func (r *rrffm) doPostRequest(url string, content io.Reader, contentType string) ([]byte, *time.Duration, error) {
	if r.debug {
		log.Printf("Doing POST request to %s", url)
	}
	start := time.Now()

	req, err := http.NewRequest(http.MethodPost, url, content)
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

	body, err := ioutil.ReadAll(resp.Body)
	duration := time.Since(start)
	if r.debug {
		log.Printf("Received response\n%s", string(body))
	}
	if err != nil {
		return nil, nil, err
	}
	return body, &duration, nil
}

func (r *rrffm) checkError(action string, resp []byte, err error) error {
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

func (r *rrffm) getTimestamp() string {
	return time.Now().Format(TimeFormat)
}

func (r *rrffm) Connect(password string) error {
	_, _, err := r.doGetRequest(fmt.Sprintf(connectURL, r.baseURL, url.QueryEscape(password), url.QueryEscape(r.getTimestamp())))
	return err
}

func (r *rrffm) Fileinfo(path string) (*Fileinfo, error) {
	body, _, err := r.doGetRequest(fmt.Sprintf(fileinfoURL, r.baseURL, url.QueryEscape(path)))
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

func (r *rrffm) Filelist(dir string, recursive bool) (*Filelist, error) {
	fl, err := r.getFullFilelist(url.QueryEscape(dir), 0)
	if err != nil {
		return nil, err
	}
	if recursive {
		for _, f := range fl.Files {
			if !f.IsDir() {

				// Directories come first so once we get here we can skip the remaining
				break
			}
			subfl, err := r.Filelist(fmt.Sprintf("%s/%s", fl.Dir, f.Name), true)
			if err != nil {
				return nil, err
			}
			fl.Subdirs = append(fl.Subdirs, subfl)
		}
	}
	return fl, nil
}

func (r *rrffm) getFullFilelist(dir string, first uint64) (*Filelist, error) {

	body, _, err := r.doGetRequest(fmt.Sprintf(filelistURL, r.baseURL, dir))
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
		moreFiles, err := r.getFullFilelist(dir, fl.Next)
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

func (r *rrffm) Download(filepath string) ([]byte, *time.Duration, error) {
	return r.doGetRequest(fmt.Sprintf(downloadURL, r.baseURL, url.QueryEscape(filepath)))
}

func (r *rrffm) Mkdir(path string) error {
	resp, _, err := r.doGetRequest(fmt.Sprintf(mkdirURL, r.baseURL, url.QueryEscape(path)))
	return r.checkError(fmt.Sprintf("Mkdir %s", path), resp, err)
}

func (r *rrffm) Move(oldpath, newpath string) error {
	resp, _, err := r.doGetRequest(fmt.Sprintf(moveURL, r.baseURL, url.QueryEscape(oldpath), url.QueryEscape(newpath)))
	return r.checkError(fmt.Sprintf("Rename %s to %s", oldpath, newpath), resp, err)
}

func (r *rrffm) Delete(path string) error {
	resp, _, err := r.doGetRequest(fmt.Sprintf(deleteURL, r.baseURL, url.QueryEscape(path)))
	return r.checkError(fmt.Sprintf("Delete %s", path), resp, err)
}

func (r *rrffm) Upload(path string, content io.Reader) (*time.Duration, error) {
	resp, duration, err := r.doPostRequest(fmt.Sprintf(uploadURL, r.baseURL, url.QueryEscape(path), url.QueryEscape(r.getTimestamp())), content, "application/octet-stream")
	return duration, r.checkError(fmt.Sprintf("Uploading file to %s", path), resp, err)
}
