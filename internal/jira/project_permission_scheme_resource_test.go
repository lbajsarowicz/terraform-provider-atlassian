package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func newProjectPermissionSchemeMockServer(projectKey string, schemeID int) *httptest.Server {
	var mu sync.Mutex
	currentSchemeID := schemeID

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		expectedPath := fmt.Sprintf("/rest/api/3/project/%s/permissionscheme", projectKey)

		switch {
		// Assign or update scheme
		case r.Method == "PUT" && r.URL.Path == expectedPath:
			var body struct {
				ID int `json:"id"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			currentSchemeID = body.ID
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   body.ID,
				"name": "Test Scheme",
			})

		// Read current scheme
		case r.Method == "GET" && r.URL.Path == expectedPath:
			mu.Lock()
			sid := currentSchemeID
			mu.Unlock()
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   sid,
				"name": "Test Scheme",
			})

		// List all schemes (used by Delete to find default)
		// NOTE: response key is "permissionSchemes", NOT "values"
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"permissionSchemes": []interface{}{
					map[string]interface{}{"id": 10000, "name": "Default Permission Scheme"},
					map[string]interface{}{"id": schemeID, "name": "Test Scheme"},
				},
			})

		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestAccProjectPermissionSchemeResourceHelper_basic(t *testing.T) {
	projectKey := "TESTPROJ"
	schemeID := 10200

	mockServer := newProjectPermissionSchemeMockServer(projectKey, schemeID)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = %q
  scheme_id   = "%d"
}`, projectKey, schemeID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "project_key", projectKey),
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "scheme_id", fmt.Sprintf("%d", schemeID)),
				),
			},
		},
	})
}

func TestAccProjectPermissionSchemeResourceHelper_import(t *testing.T) {
	projectKey := "IMPPROJ"
	schemeID := 10201

	mockServer := newProjectPermissionSchemeMockServer(projectKey, schemeID)
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = %q
  scheme_id   = "%d"
}`, projectKey, schemeID),
			},
			{
				ResourceName:                         "atlassian_jira_project_permission_scheme.test",
				ImportState:                          true,
				ImportStateId:                        projectKey,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "project_key",
			},
		},
	})
}

func TestAccProjectPermissionSchemeResourceHelper_notFound(t *testing.T) {
	projectKey := "NFPROJ"
	schemeID := 10202

	var mu sync.Mutex
	found := true

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		expectedPath := fmt.Sprintf("/rest/api/3/project/%s/permissionscheme", projectKey)

		switch {
		case r.Method == "PUT" && r.URL.Path == expectedPath:
			json.NewEncoder(w).Encode(map[string]interface{}{"id": schemeID, "name": "Test"})

		case r.Method == "GET" && r.URL.Path == expectedPath:
			mu.Lock()
			f := found
			mu.Unlock()
			if !f {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			json.NewEncoder(w).Encode(map[string]interface{}{"id": schemeID, "name": "Test"})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme":
			json.NewEncoder(w).Encode(map[string]interface{}{
				"permissionSchemes": []interface{}{
					map[string]interface{}{"id": 10000, "name": "Default Permission Scheme"},
				},
			})

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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = %q
  scheme_id   = "%d"
}`, projectKey, schemeID),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "project_key", projectKey),
				),
			},
			{
				// Remove from mock state to simulate out-of-band deletion
				PreConfig: func() {
					mu.Lock()
					found = false
					mu.Unlock()
				},
				Config: fmt.Sprintf(`resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = %q
  scheme_id   = "%d"
}`, projectKey, schemeID),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}
