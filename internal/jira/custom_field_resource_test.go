package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

const (
	testFieldID         = "customfield_10100"
	testFieldType       = "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
	testFieldSearcher   = "com.atlassian.jira.plugin.system.customfieldtypes:textsearcher"
	testFieldCustomID   = int64(10100)
)

type customFieldState struct {
	mu          sync.Mutex
	ID          string
	Name        string
	Description string
	Type        string
	SearcherKey string
}

func (s *customFieldState) set(id, name, description, fieldType, searcherKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ID = id
	s.Name = name
	s.Description = description
	s.Type = fieldType
	s.SearcherKey = searcherKey
}

func (s *customFieldState) get() (id, name, description, fieldType, searcherKey string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ID, s.Name, s.Description, s.Type, s.SearcherKey
}

func (s *customFieldState) apiResponse() map[string]interface{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	return map[string]interface{}{
		"id":          s.ID,
		"name":        s.Name,
		"custom":      true,
		"description": s.Description,
		"searcherKey": s.SearcherKey,
		"schema": map[string]interface{}{
			"type":     "string",
			"custom":   s.Type,
			"customId": testFieldCustomID,
		},
	}
}

func newCustomFieldMockServer(state *customFieldState, readCount *atomic.Int32) *httptest.Server {
	var deleted atomic.Int32

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/field":
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Type        string `json:"type"`
				SearcherKey string `json:"searcherKey"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			searcherKey := body.SearcherKey
			if searcherKey == "" {
				searcherKey = testFieldSearcher
			}
			state.set(testFieldID, body.Name, body.Description, body.Type, searcherKey)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(state.apiResponse()) //nolint:errcheck

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/field":
			if readCount != nil {
				readCount.Add(1)
			}
			id, _, _, _, _ := state.get()
			if id == "" || deleted.Load() > 0 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]interface{}{}) //nolint:errcheck
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]interface{}{state.apiResponse()}) //nolint:errcheck

		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/field/"+testFieldID:
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			id, _, _, fieldType, searcherKey := state.get()
			if id == "" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			state.set(testFieldID, body.Name, body.Description, fieldType, searcherKey)
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(state.apiResponse()) //nolint:errcheck

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/field/"+testFieldID:
			if deleted.Load() > 0 {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			deleted.Add(1)
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccCustomFieldResource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &customFieldState{}

	mockServer := newCustomFieldMockServer(state, nil)
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_custom_field" "test" {
  name        = %q
  description = "A test custom field"
  type        = %q
}`, name, testFieldType),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "description", "A test custom field"),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "type", testFieldType),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "id", testFieldID),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "searcher_key", testFieldSearcher),
				),
			},
		},
	})
}

func TestAccCustomFieldResource_update(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	updatedName := fmt.Sprintf("tf-test-%s-updated", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &customFieldState{}

	mockServer := newCustomFieldMockServer(state, nil)
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_custom_field" "test" {
  name        = %q
  description = "Original description"
  type        = %q
}`, name, testFieldType),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "description", "Original description"),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_custom_field" "test" {
  name        = %q
  description = "Updated description"
  type        = %q
}`, updatedName, testFieldType),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "name", updatedName),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "description", "Updated description"),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "id", testFieldID),
				),
			},
		},
	})
}

func TestAccCustomFieldResource_Read_NotFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &customFieldState{}

	var readCount atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/field":
			var body struct {
				Name        string `json:"name"`
				Description string `json:"description"`
				Type        string `json:"type"`
				SearcherKey string `json:"searcherKey"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			state.set(testFieldID, body.Name, body.Description, body.Type, testFieldSearcher)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(state.apiResponse()) //nolint:errcheck

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/field":
			readCount.Add(1)
			if readCount.Load() <= 1 {
				// First read after create succeeds
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]interface{}{state.apiResponse()}) //nolint:errcheck
			} else {
				// Subsequent reads return empty list (deleted out-of-band)
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode([]interface{}{}) //nolint:errcheck
			}

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/field/"+testFieldID:
			w.WriteHeader(http.StatusNoContent)

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_custom_field" "test" {
  name = %q
  type = %q
}`, name, testFieldType),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "name", name),
				),
			},
			// Second step: Read will return empty list → resource removed from state → plan shows recreation
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_custom_field" "test" {
  name = %q
  type = %q
}`, name, testFieldType),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccCustomFieldResource_Import(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &customFieldState{}

	mockServer := newCustomFieldMockServer(state, nil)
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_custom_field" "test" {
  name        = %q
  description = "Import test"
  type        = %q
}`, name, testFieldType),
			},
			{
				ResourceName:      "atlassian_jira_custom_field.test",
				ImportState:       true,
				ImportStateId:     testFieldID,
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccCustomFieldResource_WithSearcherKey(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))
	state := &customFieldState{}

	mockServer := newCustomFieldMockServer(state, nil)
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_custom_field" "test" {
  name         = %q
  type         = %q
  searcher_key = %q
}`, name, testFieldType, testFieldSearcher),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "name", name),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "type", testFieldType),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "searcher_key", testFieldSearcher),
					resource.TestCheckResourceAttr("atlassian_jira_custom_field.test", "id", testFieldID),
				),
			},
		},
	})
}

// TestAccCustomFieldDataSource_basic verifies looking up a custom field by name.
func TestAccCustomFieldDataSource_basic(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/field" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{ //nolint:errcheck
				{
					"id":          testFieldID,
					"name":        name,
					"custom":      true,
					"description": "A data source test field",
					"searcherKey": testFieldSearcher,
					"schema": map[string]interface{}{
						"type":     "string",
						"custom":   testFieldType,
						"customId": testFieldCustomID,
					},
				},
				{
					"id":     "issuetype",
					"name":   "Issue Type",
					"custom": false,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "atlassian_jira_custom_field" "test" { name = %q }`, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_custom_field.test", "name", name),
					resource.TestCheckResourceAttr("data.atlassian_jira_custom_field.test", "id", testFieldID),
					resource.TestCheckResourceAttr("data.atlassian_jira_custom_field.test", "description", "A data source test field"),
					resource.TestCheckResourceAttr("data.atlassian_jira_custom_field.test", "type", testFieldType),
					resource.TestCheckResourceAttr("data.atlassian_jira_custom_field.test", "searcher_key", testFieldSearcher),
				),
			},
		},
	})
}

// TestAccCustomFieldDataSource_NotFound verifies an error is returned when no field matches the name.
func TestAccCustomFieldDataSource_NotFound(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/field" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{ //nolint:errcheck
				{
					"id":     "issuetype",
					"name":   "Issue Type",
					"custom": false,
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(`data "atlassian_jira_custom_field" "test" { name = %q }`, name),
				ExpectError: regexp.MustCompile("Custom field not found"),
			},
		},
	})
}

// TestAccCustomFieldDataSource_AmbiguousName verifies an error is returned when multiple fields share the same name.
func TestAccCustomFieldDataSource_AmbiguousName(t *testing.T) {
	name := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(10, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" && r.URL.Path == "/rest/api/3/field" {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode([]map[string]interface{}{ //nolint:errcheck
				{
					"id":     "customfield_10100",
					"name":   name,
					"custom": true,
					"schema": map[string]interface{}{"type": "string", "custom": testFieldType, "customId": testFieldCustomID},
				},
				{
					"id":     "customfield_10101",
					"name":   name,
					"custom": true,
					"schema": map[string]interface{}{"type": "string", "custom": testFieldType, "customId": int64(10101)},
				},
			})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				Config:      fmt.Sprintf(`data "atlassian_jira_custom_field" "test" { name = %q }`, name),
				ExpectError: regexp.MustCompile("Ambiguous custom field name"),
			},
		},
	})
}
