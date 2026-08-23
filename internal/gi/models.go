package gi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// Variable is a Ghost Inspector suite variable.
type Variable struct {
	Name    string `json:"name"`
	Value   string `json:"value"`
	Private bool   `json:"private,omitempty"`
}

// Schedule is a Ghost Inspector suite schedule.
type Schedule struct {
	Enabled  bool   `json:"enabled"`
	Interval string `json:"interval,omitempty"`
	Time     string `json:"time,omitempty"`
}

// Suite is a Ghost Inspector suite. Pointer fields are omitted from update
// payloads when nil, which preserves Ghost Inspector's "inherit / default"
// semantics for unset values.
type Suite struct {
	ID                         string                 `json:"_id,omitempty"`
	Name                       string                 `json:"name"`
	Description                string                 `json:"description,omitempty"`
	Folder                     string                 `json:"folder,omitempty"`
	Organization               interface{}            `json:"organization,omitempty"`
	Browser                    *string                `json:"browser,omitempty"`
	Region                     *string                `json:"region,omitempty"`
	UserAgent                  *string                `json:"userAgent,omitempty"`
	Geolocation                *string                `json:"geolocation,omitempty"`
	MaxWaitDelay               *int64                 `json:"maxWaitDelay,omitempty"`
	MaxAjaxDelay               *int64                 `json:"maxAjaxDelay,omitempty"`
	GlobalStepDelay            *int64                 `json:"globalStepDelay,omitempty"`
	FinalDelay                 *int64                 `json:"finalDelay,omitempty"`
	AutoRetry                  *bool                  `json:"autoRetry,omitempty"`
	ScreenshotCompareEnabled   *bool                  `json:"screenshotCompareEnabled,omitempty"`
	ScreenshotCompareThreshold *float64               `json:"screenshotCompareThreshold,omitempty"`
	FailOnJavaScriptError      *bool                  `json:"failOnJavaScriptError,omitempty"`
	Schedule                   *Schedule              `json:"schedule,omitempty"`
	Variables                  []Variable             `json:"variables,omitempty"`
	Extra                      map[string]interface{} `json:"-"`
}

// Test is a Ghost Inspector test. Steps are kept as raw maps because the
// API returns per-step shapes that vary by command.
type Test struct {
	ID                         string                   `json:"_id,omitempty"`
	Name                       string                   `json:"name"`
	Suite                      interface{}              `json:"suite,omitempty"`
	StartURL                   *string                  `json:"startUrl,omitempty"`
	Browser                    *string                  `json:"browser,omitempty"`
	Region                     *string                  `json:"region,omitempty"`
	UserAgent                  *string                  `json:"userAgent,omitempty"`
	Geolocation                *string                  `json:"geolocation,omitempty"`
	MaxWaitDelay               *int64                   `json:"maxWaitDelay,omitempty"`
	MaxAjaxDelay               *int64                   `json:"maxAjaxDelay,omitempty"`
	GlobalStepDelay            *int64                   `json:"globalStepDelay,omitempty"`
	FinalDelay                 *int64                   `json:"finalDelay,omitempty"`
	AutoRetry                  *bool                    `json:"autoRetry,omitempty"`
	ScreenshotCompareEnabled   *bool                    `json:"screenshotCompareEnabled,omitempty"`
	ScreenshotCompareThreshold *float64                 `json:"screenshotCompareThreshold,omitempty"`
	FailOnJavaScriptError      *bool                    `json:"failOnJavaScriptError,omitempty"`
	ImportOnly                 *bool                    `json:"importOnly,omitempty"`
	Steps                      []map[string]interface{} `json:"steps,omitempty"`
}

// Folder is a Ghost Inspector folder.
type Folder struct {
	ID           string      `json:"_id,omitempty"`
	Name         string      `json:"name"`
	Organization interface{} `json:"organization,omitempty"`
}

// extractID normalizes the API's inconsistent ID fields, which arrive as a
// bare string on list endpoints and as an object on detail/create endpoints.
func extractID(v interface{}) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]interface{}:
		if id, ok := t["_id"].(string); ok {
			return id
		}
	}
	return ""
}

// OrganizationID returns the owning organization ID regardless of response shape.
func (s *Suite) OrganizationID() string { return extractID(s.Organization) }

// OrgID returns the owning organization ID regardless of response shape.
func (f *Folder) OrgID() string { return extractID(f.Organization) }

// SuiteID extracts the owning suite ID from the API's inconsistent `suite`
// field, which is a bare ID string on list endpoints and an object on test
// detail endpoints.
func (t *Test) SuiteID() string {
	switch s := t.Suite.(type) {
	case string:
		return s
	case map[string]interface{}:
		if id, ok := s["_id"].(string); ok {
			return id
		}
	}
	return ""
}

// --- Folders ---------------------------------------------------------------

func (c *Client) ListFolders(ctx context.Context) ([]Folder, error) {
	raw, err := c.list(ctx, "/folders/")
	if err != nil {
		return nil, err
	}
	folders := make([]Folder, 0, len(raw))
	for _, r := range raw {
		var f Folder
		if err := json.Unmarshal(r, &f); err != nil {
			return nil, fmt.Errorf("decode folder: %w", err)
		}
		folders = append(folders, f)
	}
	return folders, nil
}

func (c *Client) FindFolderByName(ctx context.Context, name string) (*Folder, error) {
	folders, err := c.ListFolders(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range folders {
		if f.Name == name {
			out := f
			return &out, nil
		}
	}
	return nil, nil
}

func (c *Client) CreateFolder(ctx context.Context, name, organization string) (*Folder, error) {
	body := map[string]interface{}{"name": name}
	if organization != "" {
		body["organization"] = organization
	}
	data, err := c.do(ctx, http.MethodPost, "/folders/", body)
	if err != nil {
		return nil, err
	}
	var f Folder
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("decode created folder: %w", err)
	}
	return &f, nil
}

func (c *Client) DeleteFolder(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/folders/"+id+"/", nil)
	return err
}

// UpdateFolder renames a folder in place.
func (c *Client) UpdateFolder(ctx context.Context, id, name string) error {
	_, err := c.do(ctx, http.MethodPost, "/folders/"+id+"/", map[string]interface{}{"name": name})
	return err
}

// OrganizationID resolves the organization ID from any existing folder,
// falling back to any existing suite (mirrors the resolution every
// Ghost Inspector automation ends up doing).
func (c *Client) OrganizationID(ctx context.Context) (string, error) {
	orgs, err := c.OrganizationIDs(ctx)
	if err != nil {
		return "", err
	}
	if len(orgs) == 0 {
		return "", nil
	}
	return orgs[0], nil
}

// OrganizationIDs lists every distinct organization ID visible through the
// caller's folders and suites, folders first. Visibility does not imply write
// access; callers creating resources should treat these as candidates.
func (c *Client) OrganizationIDs(ctx context.Context) ([]string, error) {
	seen := map[string]bool{}
	var out []string
	add := func(org string) {
		if org != "" && !seen[org] {
			seen[org] = true
			out = append(out, org)
		}
	}
	folders, err := c.ListFolders(ctx)
	if err != nil {
		return nil, err
	}
	for _, f := range folders {
		add(f.OrgID())
	}
	suites, err := c.ListSuites(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range suites {
		add(s.OrganizationID())
	}
	return out, nil
}

// --- Suites ----------------------------------------------------------------

func (c *Client) ListSuites(ctx context.Context) ([]Suite, error) {
	raw, err := c.list(ctx, "/suites/")
	if err != nil {
		return nil, err
	}
	suites := make([]Suite, 0, len(raw))
	for _, r := range raw {
		var s Suite
		if err := json.Unmarshal(r, &s); err != nil {
			return nil, fmt.Errorf("decode suite: %w", err)
		}
		suites = append(suites, s)
	}
	return suites, nil
}

// FindSuiteByName returns the first suite with the given name, or nil.
func (c *Client) FindSuiteByName(ctx context.Context, name string) (*Suite, error) {
	suites, err := c.ListSuites(ctx)
	if err != nil {
		return nil, err
	}
	for _, s := range suites {
		if s.Name == name {
			out := s
			return &out, nil
		}
	}
	return nil, nil
}

func (c *Client) ListFolderSuites(ctx context.Context, folderID string) ([]Suite, error) {
	raw, err := c.list(ctx, "/folders/"+folderID+"/suites/")
	if err != nil {
		return nil, err
	}
	suites := make([]Suite, 0, len(raw))
	for _, r := range raw {
		var s Suite
		if err := json.Unmarshal(r, &s); err != nil {
			return nil, fmt.Errorf("decode suite: %w", err)
		}
		suites = append(suites, s)
	}
	return suites, nil
}

func (c *Client) GetSuite(ctx context.Context, id string) (*Suite, error) {
	data, err := c.do(ctx, http.MethodGet, "/suites/"+id+"/", nil)
	if err != nil {
		return nil, err
	}
	var s Suite
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode suite: %w", err)
	}
	return &s, nil
}

func (c *Client) CreateSuite(ctx context.Context, s *Suite) (*Suite, error) {
	body := map[string]interface{}{"name": s.Name}
	if s.Folder != "" {
		body["folder"] = s.Folder
	}
	if org := s.OrganizationID(); org != "" {
		body["organization"] = org
	}
	data, err := c.do(ctx, http.MethodPost, "/suites/", body)
	if err != nil {
		return nil, err
	}
	var out Suite
	if err := json.Unmarshal(data, &out); err != nil {
		return nil, fmt.Errorf("decode created suite: %w", err)
	}
	return &out, nil
}

// UpdateSuite posts the given fields to the suite. Only non-nil fields on the
// struct are sent; callers build the struct to express exactly what changed.
func (c *Client) UpdateSuite(ctx context.Context, id string, fields map[string]interface{}) error {
	_, err := c.do(ctx, http.MethodPost, "/suites/"+id+"/", fields)
	return err
}

func (c *Client) DeleteSuite(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/suites/"+id+"/", nil)
	return err
}

// --- Tests -----------------------------------------------------------------

func (c *Client) ListSuiteTests(ctx context.Context, suiteID string) ([]Test, error) {
	raw, err := c.list(ctx, "/suites/"+suiteID+"/tests/")
	if err != nil {
		return nil, err
	}
	tests := make([]Test, 0, len(raw))
	for _, r := range raw {
		var t Test
		if err := json.Unmarshal(r, &t); err != nil {
			return nil, fmt.Errorf("decode test: %w", err)
		}
		tests = append(tests, t)
	}
	return tests, nil
}

func (c *Client) GetTest(ctx context.Context, id string) (*Test, error) {
	data, err := c.do(ctx, http.MethodGet, "/tests/"+id+"/", nil)
	if err != nil {
		return nil, err
	}
	var t Test
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("decode test: %w", err)
	}
	return &t, nil
}

// UpdateTestMetadata updates a test's settings in place. The Ghost Inspector
// API does not support updating steps through this endpoint.
func (c *Client) UpdateTestMetadata(ctx context.Context, id string, fields map[string]interface{}) error {
	_, err := c.do(ctx, http.MethodPost, "/tests/"+id+"/", fields)
	return err
}

func (c *Client) DeleteTest(ctx context.Context, id string) error {
	_, err := c.do(ctx, http.MethodDelete, "/tests/"+id+"/", nil)
	return err
}

// ExportTest returns the full portable definition of a test (the same JSON
// the UI export produces), used to preserve unmanaged settings across a
// step replacement.
func (c *Client) ExportTest(ctx context.Context, id string) (map[string]interface{}, error) {
	data, err := c.do(ctx, http.MethodGet, "/tests/"+id+"/export/json/", nil)
	if err != nil {
		// The export endpoint returns the bare document on some API versions.
		return nil, err
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("decode exported test: %w", err)
	}
	if _, ok := doc["steps"]; !ok {
		return nil, fmt.Errorf("exported test has no steps array")
	}
	return doc, nil
}

// ImportTest imports a test definition into a suite and returns the new test.
func (c *Client) ImportTest(ctx context.Context, suiteID string, doc map[string]interface{}) (*Test, error) {
	data, err := c.do(ctx, http.MethodPost, "/suites/"+suiteID+"/import-test/json/", doc)
	if err != nil {
		return nil, err
	}
	var t Test
	if err := json.Unmarshal(data, &t); err != nil {
		return nil, fmt.Errorf("decode imported test: %w", err)
	}
	if t.ID == "" {
		return nil, fmt.Errorf("import did not return a test ID")
	}
	return &t, nil
}
