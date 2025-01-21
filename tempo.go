package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httputil"
	"strings"
	"time"

	"github.com/pkg/errors"
)

const tempoAPI = "https://api.tempo.io/4"
const tempoDateLayout = time.RFC3339

// TempoClient for tempo API defined in https://apidocs.tempo.io/
type TempoClient struct {
	token         string
	jiraAccountID string
}

// NewTempoClient returns a tempo API Client
func NewTempoClient(token, jiraAccountID string) *TempoClient {
	return &TempoClient{
		token:         token,
		jiraAccountID: jiraAccountID,
	}
}

func (c *TempoClient) Do(method, resource string, body io.Reader) (*http.Response, error) {
	wrap := func(err error) error {
		return errors.Wrap(err, fmt.Sprintf("while doing request (%s %s)", method, resource))
	}

	if !strings.HasPrefix(resource, "/") {
		resource = "/" + resource
	}

	endpoint := fmt.Sprintf("%s%s", tempoAPI, resource)
	req, err := http.NewRequest(method, endpoint, body)
	if err != nil {
		return nil, wrap(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", c.token))

	if debug {
		reqDump, _ := httputil.DumpRequest(req, true)
		fmt.Printf("------ Tempo request: \n%s\n", string(reqDump))
	}

	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, wrap(err)
	}

	success := res.StatusCode >= 200 && res.StatusCode < 300
	if !success {
		resDump, _ := httputil.DumpResponse(res, true)
		if debug {
			fmt.Printf("------ Tempo response: \n%s\n", string(resDump))
		}
		err = fmt.Errorf("unsuccessful status (%d). \nResponse: \n%s", res.StatusCode, string(resDump))
		return nil, wrap(err)
	}
	return res, nil
}

func (c *TempoClient) LogDay(date time.Time, hours int, jiraIssueKey string) error {
	loggedSeconds := hours * 60 * 60

	w := NewWorklog{
		IssueKey:         jiraIssueKey,
		TimeSpentSeconds: loggedSeconds,
		BillableSeconds:  loggedSeconds,
		StartDate:        date.Format(tempoDateLayout),
		Description:      fmt.Sprintf("Working on issue %s.", jiraIssueKey),
		AuthorAccountID:  c.jiraAccountID,
		Attributes:       nil,
	}

	body, err := json.Marshal(w)
	if err != nil {
		return err
	}
	_, err = c.Do("POST", "/worklogs", bytes.NewReader(body))
	return err
}

func (c *TempoClient) GetLoggedHours(jiraIssueKey string) (int, error) {
	searchBody := map[string]interface{}{
		"from":  "1970-01-01",
		"to":    "2100-12-31",
		"issue": []string{jiraIssueKey},
	}

	body, err := json.Marshal(searchBody)
	if err != nil {
		return 0, fmt.Errorf("while marshalling search body: %v", err)
	}

	res, err := c.Do("POST", "/worklogs/search", bytes.NewReader(body))
	if err != nil {
		return 0, fmt.Errorf("while searching worklogs: %v", err)
	}

	defer res.Body.Close()

	bodyBytes, err := io.ReadAll(res.Body)
	if err != nil {
		return 0, err
	}

	if debug {
		fmt.Printf("------ Tempo response body: \n%s\n", string(bodyBytes))
	}

	// Create a new reader from the bytes for json decoding
	bodyReader := bytes.NewReader(bodyBytes)

	var worklogs WorklogsRes
	if err := json.NewDecoder(bodyReader).Decode(&worklogs); err != nil {
		return 0, err
	}

	// var worklogs WorklogsRes
	// if err := json.NewDecoder(res.Body).Decode(&worklogs); err != nil {
	// 	return 0, err
	// }

	totalSeconds := 0
	for _, w := range worklogs.Results {
		totalSeconds += w.TimeSpentSeconds
	}

	return totalSeconds / 3600, nil
}

type NewWorklog struct {
	IssueKey         string `json:"issueKey"`
	TimeSpentSeconds int    `json:"timeSpentSeconds"`
	BillableSeconds  int    `json:"billableSeconds"`
	StartDate        string `json:"startDate"`
	Description      string `json:"description"`
	AuthorAccountID  string `json:"authorAccountId"`
	Attributes       []struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	} `json:"attributes,omitempty"`
}

type Worklog struct {
	Self           string `json:"self"`
	TempoWorklogID int    `json:"tempoWorklogId"`
	Issue          struct {
		Self string `json:"self"`
		ID   int    `json:"id"`
	} `json:"issue"`
	TimeSpentSeconds int       `json:"timeSpentSeconds"`
	BillableSeconds  int       `json:"billableSeconds"`
	StartDate        string    `json:"startDate"`
	Description      string    `json:"description"`
	CreatedAt        time.Time `json:"createdAt"`
	UpdatedAt        time.Time `json:"updatedAt"`
	Author           struct {
		AccountID string `json:"accountId"`
		Self      string `json:"self"`
	} `json:"author"`
	Attributes struct {
		Self   string `json:"self"`
		Values []struct {
			Key   string `json:"key"`
			Value string `json:"value"`
		} `json:"values"`
	} `json:"attributes"`
}

type WorklogsRes struct {
	Results  []Worklog `json:"results"`
	Metadata struct {
		Count  int `json:"count"`
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
	} `json:"metadata"`
}
