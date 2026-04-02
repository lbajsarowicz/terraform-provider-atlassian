package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func newPermissionSchemeMock(id int, name, description string) map[string]interface{} {
	return map[string]interface{}{
		"id":          id,
		"name":        name,
		"description": description,
	}
}

func TestAccPermissionSchemeResource_basic(t *testing.T) {
	schemeName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	var created bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			created = true
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newPermissionSchemeMock(10100, body["name"], body["description"]))

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme/10100":
			mu.Lock()
			c := created
			mu.Unlock()
			if !c {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newPermissionSchemeMock(10100, schemeName, "Test description"))

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10100":
			mu.Lock()
			created = false
			mu.Unlock()
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
				Config: fmt.Sprintf(`resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "Test description"
}`, schemeName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "name", schemeName),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "description", "Test description"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "id", "10100"),
				),
			},
		},
	})
}

func TestAccPermissionSchemeResource_update(t *testing.T) {
	schemeName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	currentName := ""
	currentDesc := ""

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			currentName = body["name"]
			currentDesc = body["description"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newPermissionSchemeMock(10200, body["name"], body["description"]))

		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/permissionscheme/10200":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			currentName = body["name"]
			currentDesc = body["description"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newPermissionSchemeMock(10200, body["name"], body["description"]))

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme/10200":
			mu.Lock()
			n := currentName
			d := currentDesc
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newPermissionSchemeMock(10200, n, d))

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10200":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "Original"
}`, schemeName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "description", "Original"),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "Updated"
}`, schemeName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "description", "Updated"),
				),
			},
		},
	})
}

func TestAccPermissionSchemeResource_Read_NotFound(t *testing.T) {
	schemeName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var readCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newPermissionSchemeMock(10300, schemeName, ""))

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme/10300":
			count := readCount.Add(1)
			if count <= 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(newPermissionSchemeMock(10300, schemeName, ""))
			} else {
				w.WriteHeader(http.StatusNotFound)
			}

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10300":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_permission_scheme" "test" { name = %q }`, schemeName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme.test", "name", schemeName),
				),
			},
			{
				Config:             fmt.Sprintf(`resource "atlassian_jira_permission_scheme" "test" { name = %q }`, schemeName),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccPermissionSchemeResource_Import(t *testing.T) {
	schemeName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(newPermissionSchemeMock(10400, schemeName, "Import test"))

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme/10400":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(newPermissionSchemeMock(10400, schemeName, "Import test"))

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10400":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_permission_scheme" "test" {
  name        = %q
  description = "Import test"
}`, schemeName),
			},
			{
				ResourceName:      "atlassian_jira_permission_scheme.test",
				ImportState:       true,
				ImportStateId:     "10400",
				ImportStateVerify: true,
			},
		},
	})
}

// --- Data Source Tests ---

func TestAccPermissionSchemeDataSource_basic(t *testing.T) {
	schemeName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"permissionSchemes": []interface{}{
					newPermissionSchemeMock(10001, "Default Permission Scheme", "Default"),
					newPermissionSchemeMock(10500, schemeName, "Found it"),
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
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`data "atlassian_jira_permission_scheme" "test" { name = %q }`, schemeName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("data.atlassian_jira_permission_scheme.test", "id", "10500"),
					resource.TestCheckResourceAttr("data.atlassian_jira_permission_scheme.test", "name", schemeName),
					resource.TestCheckResourceAttr("data.atlassian_jira_permission_scheme.test", "description", "Found it"),
				),
			},
		},
	})
}

func TestAccPermissionSchemeDataSource_NotFound(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"permissionSchemes": []interface{}{
					newPermissionSchemeMock(10001, "Default Permission Scheme", "Default"),
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
		Steps: []resource.TestStep{
			{
				Config:      `data "atlassian_jira_permission_scheme" "test" { name = "nonexistent-scheme" }`,
				ExpectError: regexp.MustCompile("Permission scheme not found"),
			},
		},
	})
}

// --- Grant Resource Tests ---

func TestAccPermissionSchemeGrantResource_basic(t *testing.T) {
	var mu sync.Mutex
	var grantCreated bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			grantCreated = true
			mu.Unlock()
			holder := body["holder"].(map[string]interface{})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20001,
				"holder": map[string]interface{}{
					"type":      holder["type"],
					"parameter": holder["parameter"],
				},
				"permission": body["permission"],
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20001":
			mu.Lock()
			c := grantCreated
			mu.Unlock()
			if !c {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20001,
				"holder": map[string]interface{}{
					"type":      "group",
					"parameter": "jira-developers",
				},
				"permission": "BROWSE_PROJECTS",
			})

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20001":
			mu.Lock()
			grantCreated = false
			mu.Unlock()
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
				Config: `resource "atlassian_jira_permission_scheme_grant" "test" {
  scheme_id        = "10100"
  permission       = "BROWSE_PROJECTS"
  holder_type      = "group"
  holder_parameter = "jira-developers"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "id", "20001"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "scheme_id", "10100"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "permission", "BROWSE_PROJECTS"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "holder_type", "group"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "holder_parameter", "jira-developers"),
				),
			},
		},
	})
}

func TestAccPermissionSchemeGrantResource_anyone(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20002,
				"holder": map[string]interface{}{
					"type": "anyone",
				},
				"permission": "BROWSE_PROJECTS",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20002":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20002,
				"holder": map[string]interface{}{
					"type": "anyone",
				},
				"permission": "BROWSE_PROJECTS",
			})

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20002":
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
				Config: `resource "atlassian_jira_permission_scheme_grant" "test" {
  scheme_id   = "10100"
  permission  = "BROWSE_PROJECTS"
  holder_type = "anyone"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "id", "20002"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "holder_type", "anyone"),
				),
			},
		},
	})
}

func TestAccPermissionSchemeGrantResource_Read_NotFound(t *testing.T) {
	var readCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20003,
				"holder": map[string]interface{}{
					"type": "projectLead",
				},
				"permission": "ADMINISTER_PROJECTS",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20003":
			count := readCount.Add(1)
			if count <= 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id": 20003,
					"holder": map[string]interface{}{
						"type": "projectLead",
					},
					"permission": "ADMINISTER_PROJECTS",
				})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20003":
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
				Config: `resource "atlassian_jira_permission_scheme_grant" "test" {
  scheme_id   = "10100"
  permission  = "ADMINISTER_PROJECTS"
  holder_type = "projectLead"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "id", "20003"),
				),
			},
			{
				Config: `resource "atlassian_jira_permission_scheme_grant" "test" {
  scheme_id   = "10100"
  permission  = "ADMINISTER_PROJECTS"
  holder_type = "projectLead"
}`,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

// --- Project Permission Scheme Tests ---

func TestAccProjectPermissionSchemeResource_basic(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/project/PROJ/permissionscheme":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   10100,
				"name": "Custom Scheme",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/project/PROJ/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   10100,
				"name": "Custom Scheme",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"permissionSchemes": []interface{}{
					newPermissionSchemeMock(10001, "Default Permission Scheme", "Default"),
					newPermissionSchemeMock(10100, "Custom Scheme", "Custom"),
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
				Config: `resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = "PROJ"
  scheme_id   = "10100"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "project_key", "PROJ"),
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "scheme_id", "10100"),
				),
			},
		},
	})
}

func TestAccProjectPermissionSchemeResource_update(t *testing.T) {
	var mu sync.Mutex
	currentSchemeID := 10100

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/project/PROJ/permissionscheme":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			var id int
			if _, err := fmt.Sscanf(body["id"], "%d", &id); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			currentSchemeID = id
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   id,
				"name": "Scheme",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/project/PROJ/permissionscheme":
			mu.Lock()
			id := currentSchemeID
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   id,
				"name": "Scheme",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"permissionSchemes": []interface{}{
					newPermissionSchemeMock(10001, "Default Permission Scheme", "Default"),
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
				Config: `resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = "PROJ"
  scheme_id   = "10100"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "scheme_id", "10100"),
				),
			},
			{
				Config: `resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = "PROJ"
  scheme_id   = "10200"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "scheme_id", "10200"),
				),
			},
		},
	})
}

func TestAccProjectPermissionSchemeResource_Import(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/project/IMP/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   10100,
				"name": "Custom Scheme",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/project/IMP/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   10100,
				"name": "Custom Scheme",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"permissionSchemes": []interface{}{
					newPermissionSchemeMock(10001, "Default Permission Scheme", "Default"),
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
				Config: `resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = "IMP"
  scheme_id   = "10100"
}`,
			},
			{
				ResourceName:  "atlassian_jira_project_permission_scheme.test",
				ImportState:   true,
				ImportStateId: "IMP",
				ImportStateVerify: true,
				ImportStateVerifyIdentifierAttribute: "project_key",
			},
		},
	})
}

func TestAccProjectPermissionSchemeResource_Read_NotFound(t *testing.T) {
	var readCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/project/GONE/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   10100,
				"name": "Custom Scheme",
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/project/GONE/permissionscheme":
			count := readCount.Add(1)
			if count <= 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":   10100,
					"name": "Custom Scheme",
				})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"permissionSchemes": []interface{}{
					newPermissionSchemeMock(10001, "Default Permission Scheme", "Default"),
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
				Config: `resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = "GONE"
  scheme_id   = "10100"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_permission_scheme.test", "scheme_id", "10100"),
				),
			},
			{
				Config: `resource "atlassian_jira_project_permission_scheme" "test" {
  project_key = "GONE"
  scheme_id   = "10100"
}`,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccPermissionSchemeGrantResource_Import(t *testing.T) {
	var grantCreated bool
	var mu sync.Mutex

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			grantCreated = true
			mu.Unlock()
			holder := body["holder"].(map[string]interface{})
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20050,
				"holder": map[string]interface{}{
					"type":      holder["type"],
					"parameter": holder["parameter"],
				},
				"permission": body["permission"],
			})

		case r.Method == "GET" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20050":
			mu.Lock()
			c := grantCreated
			mu.Unlock()
			if !c {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20050,
				"holder": map[string]interface{}{
					"type":      "group",
					"parameter": "jira-admins",
				},
				"permission": "ADMINISTER_PROJECTS",
			})

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20050":
			mu.Lock()
			grantCreated = false
			mu.Unlock()
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
				Config: `resource "atlassian_jira_permission_scheme_grant" "test" {
  scheme_id        = "10100"
  permission       = "ADMINISTER_PROJECTS"
  holder_type      = "group"
  holder_parameter = "jira-admins"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "id", "20050"),
				),
			},
			{
				ResourceName:      "atlassian_jira_permission_scheme_grant.test",
				ImportState:       true,
				ImportStateId:     "10100/20050",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccPermissionSchemeGrantResource_projectRole(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission":
			var body map[string]interface{}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			holder := body["holder"].(map[string]interface{})
			if holder["type"] != "projectRole" {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20010,
				"holder": map[string]interface{}{
					"type":      "projectRole",
					"parameter": "10002",
				},
				"permission": "EDIT_ISSUES",
			})

		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/3/permissionscheme/10100/permission/20010"):
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 20010,
				"holder": map[string]interface{}{
					"type":      "projectRole",
					"parameter": "10002",
				},
				"permission": "EDIT_ISSUES",
			})

		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/permissionscheme/10100/permission/20010":
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
				Config: `resource "atlassian_jira_permission_scheme_grant" "test" {
  scheme_id        = "10100"
  permission       = "EDIT_ISSUES"
  holder_type      = "projectRole"
  holder_parameter = "10002"
}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "id", "20010"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "holder_type", "projectRole"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "holder_parameter", "10002"),
					resource.TestCheckResourceAttr("atlassian_jira_permission_scheme_grant.test", "permission", "EDIT_ISSUES"),
				),
			},
		},
	})
}
