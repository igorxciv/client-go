package rp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"time"

	"github.com/pkg/errors"
)

const (
	TestItemSuite        = "SUITE"
	TestItemStory        = "STORY"
	TestItemTest         = "TEST"
	TestItemScenario     = "SCENARIO"
	TestItemStep         = "STEP"
	TestItemBeforeClass  = "BEFORE_CLASS"
	TestItemBeforeGroups = "BEFORE_GROUPS"
	TestItemBeforeMethod = "BEFORE_METHOD"
	TestItemBeforeSuite  = "BEFORE_SUITE"
	TestItemBeforeTest   = "BEFORE_TEST"
	TestItemAfterClass   = "AFTER_CLASS"
	TestItemAfterGroups  = "AFTER_GROUPS"
	TestItemAfterMethod  = "AFTER_METHOD"
	TestItemAfterSuite   = "AFTER_SUITE"
	TestItemAfterTest    = "AFTER_TEST"
)

// TestItem defines test item structure
type TestItem struct {
	Id          string
	Name        string
	Description string
	Parent      *TestItem
	Parameters  []struct {
		Key   string
		Value string
	}
	Retry     bool
	StartTime time.Time
	Tags      []string
	Type      string

	client *Client
	launch *Launch
}

// Attachment defines attachment for log request with file
type Attachment struct {
	Name     string
	Data     io.Reader
	MimeType string
}

// fileInfo defines file structure for json request part
type fileInfo struct {
	Name string `json:"name"`
}

// jsonRequestPart defines request object for request with attachment
type jsonRequestPart []struct {
	File    *fileInfo `json:"file"`
	ItemId  string    `json:"item_id"`
	Level   string    `json:"level"`
	Message string    `json:"message"`
	Time    int64     `json:"time"`
}

// NewTestItem creates new test item
func NewTestItem(launch *Launch, name, description, itemType string, tags []string, parent *TestItem) *TestItem {
	return &TestItem{
		Name:        name,
		Description: description,
		Parent:      parent,
		Tags:        tags,
		Type:        itemType,
		launch:      launch,
		client:      launch.client,
	}
}

// Start starts specified test item
func (ti *TestItem) Start() error {
	var url string
	if ti.Parent != nil {
		url = fmt.Sprintf("%s/%s/item/%s", ti.client.Endpoint, ti.client.Project, ti.Parent.Id)
	} else {
		url = fmt.Sprintf("%s/%s/item", ti.client.Endpoint, ti.client.Project)
	}
	data := struct {
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
		StartTime   int64    `json:"start_time"`
		LaunchId    string   `json:"launch_id"`
		Type        string   `json:"type"`
		Parameters  []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"parameters"`
	}{
		Name:        ti.Name,
		Description: ti.Description,
		Tags:        ti.Tags,
		StartTime:   toTimestamp(time.Now()),
		LaunchId:    ti.launch.Id,
		Type:        ti.Type,
	}

	b, err := json.Marshal(&data)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal object %v", data)
	}

	r := bytes.NewReader(b)
	req, err := http.NewRequest(http.MethodPost, url, r)
	if err != nil {
		return errors.Wrapf(err, "failed to create POST request to %s", url)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := doRequest(req, ti.client.Token)
	defer resp.Body.Close()
	if err != nil {
		return errors.Wrapf(err, "failed to execute POST request %s", req.URL)
	}
	if resp.StatusCode != http.StatusCreated {
		return errors.Errorf("failed with status %s", resp.Status)
	}

	v := struct {
		Id string `json:"id"`
	}{}
	if err := json.NewDecoder(resp.Body).Decode(&v); err != nil {
		return errors.Wrapf(err, "failed to decode response from %s", req.URL)
	}
	ti.Id = v.Id
	return nil
}

// Finish finishes specified test item
func (ti *TestItem) Finish(status string) error {
	url := fmt.Sprintf("%s/%s/item/%s", ti.client.Endpoint, ti.client.Project, ti.Id)
	data := struct {
		EndTime int64  `json:"end_time"`
		Status  string `json:"status"`
	}{toTimestamp(time.Now()), status}

	b, err := json.Marshal(&data)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal request data %v", data)
	}

	r := bytes.NewReader(b)
	req, err := http.NewRequest(http.MethodPut, url, r)
	if err != nil {
		return errors.Wrapf(err, "failed to create PUT request to %s", url)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := doRequest(req, ti.client.Token)
	defer resp.Body.Close()
	if err != nil {
		return errors.Wrapf(err, "failed to execute PUT request to %s", req.URL)
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed with status %s", resp.Status)
	}
	return nil
}

// Log sends log for specified test item
func (ti *TestItem) Log(message, level string, attachment *Attachment) error {
	var req *http.Request
	var err error
	if attachment != nil {
		req, err = ti.getReqForLogWithAttach(message, level, attachment)
	} else {
		req, err = ti.getReqForLog(message, level)
	}
	if err != nil {
		return err
	}

	resp, err := doRequest(req, ti.client.Token)
	defer resp.Body.Close()
	if err != nil {
		return errors.Wrapf(err, "failed to execute POST request %s", req.URL)
	}
	if resp.StatusCode != http.StatusCreated {
		return errors.Errorf("failed with status %s", resp.Status)
	}
	return nil
}

// Update updates launch
func (ti *TestItem) Update(description string, tags []string) error {
	url := fmt.Sprintf("%s/%s/item/%s/update", ti.client.Endpoint, ti.client.Project, ti.Id)
	data := struct {
		Description string   `json:"description"`
		Tags        []string `json:"tags"`
	}{description, tags}

	b, err := json.Marshal(&data)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal request data %v", data)
	}

	r := bytes.NewReader(b)
	req, err := http.NewRequest(http.MethodPut, url, r)
	if err != nil {
		return errors.Wrapf(err, "failed to create PUT request to %s", url)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := doRequest(req, ti.client.Token)
	defer resp.Body.Close()
	if err != nil {
		return errors.Wrapf(err, "failed to execute PUT request to %s", req.URL)
	}
	if resp.StatusCode != http.StatusOK {
		return errors.Errorf("failed with status %s", resp.Status)
	}
	ti.Description = description
	ti.Tags = tags
	return nil
}

// Get activities for test item
func (ti *TestItem) GetActivity() (*Activity, error) {
	// TODO: Implement this
	return nil, nil
}

// getReqForLogWithAttach creates request to perform log request with message and attachment
func (ti *TestItem) getReqForLogWithAttach(message, level string, attachment *Attachment) (*http.Request, error) {
	url := fmt.Sprintf("%s/%s/log", ti.client.Endpoint, ti.client.Project)
	bodyBuf := &bytes.Buffer{}
	bodyWriter := multipart.NewWriter(bodyBuf)

	// json request part
	h := make(textproto.MIMEHeader)
	h.Set("Content-Disposition", `form-data; name="json_request_part"`)
	h.Set("Content-Type", "application/json")
	reqWriter, err := bodyWriter.CreatePart(h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create form file")
	}

	f := &fileInfo{attachment.Name}
	jsonReqPart := &jsonRequestPart{
		{f, ti.Id, level, message, toTimestamp(time.Now())},
	}
	bs, err := json.Marshal(&jsonReqPart)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal to JSON: %v", jsonReqPart)
	}
	reqReader := bytes.NewReader(bs)

	_, err = io.Copy(reqWriter, reqReader)
	if err != nil {
		return nil, errors.Wrap(err, "failed to copy reader")
	}

	// file
	h = make(textproto.MIMEHeader)
	h.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, "file", attachment.Name))
	h.Set("Content-Type", attachment.MimeType)

	fileWriter, err := bodyWriter.CreatePart(h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create form file")
	}

	_, err = io.Copy(fileWriter, attachment.Data)
	if err != nil {
		return nil, errors.Wrap(err, "failed to copy file writer")
	}

	bodyWriter.Close()

	req, err := http.NewRequest(http.MethodPost, url, bodyBuf)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create POST request to %s", url)
	}

	req.Header.Set("Content-Type", bodyWriter.FormDataContentType())
	return req, nil
}

// getReqForLog creates request to perform log request with message
func (ti *TestItem) getReqForLog(message, level string) (*http.Request, error) {
	url := fmt.Sprintf("%s/%s/log", ti.client.Endpoint, ti.client.Project)
	data := struct {
		ItemId  string `json:"item_id"`
		Message string `json:"message"`
		Level   string `json:"level"`
		Time    int64  `json:"time"`
	}{ti.Id, message, level, toTimestamp(time.Now())}

	b, err := json.Marshal(&data)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to marshal object, %v", data)
	}

	r := bytes.NewReader(b)
	req, err := http.NewRequest(http.MethodPost, url, r)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to create POST request to %s", url)
	}

	req.Header.Set("Content-Type", "application/json")

	return req, nil
}
