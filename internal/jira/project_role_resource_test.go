package jira_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/acctest"
	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestAccProjectRoleResource_basic(t *testing.T) {
	roleName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	createdRole := map[string]interface{}{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/role":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			createdRole["id"] = 10100
			createdRole["name"] = body["name"]
			createdRole["description"] = body["description"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          10100,
				"name":        body["name"],
				"description": body["description"],
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/role/10100":
			mu.Lock()
			role := make(map[string]interface{})
			for k, v := range createdRole {
				role[k] = v
			}
			mu.Unlock()
			if role["name"] == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(role)
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/role/10100":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			createdRole["name"] = body["name"]
			createdRole["description"] = body["description"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          10100,
				"name":        body["name"],
				"description": body["description"],
			})
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/role/10100":
			mu.Lock()
			delete(createdRole, "name")
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_role" "test" {
					name        = %q
					description = "Test role description"
				}`, roleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "name", roleName),
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "description", "Test role description"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "id", "10100"),
				),
			},
		},
	})
}

func TestAccProjectRoleResource_update(t *testing.T) {
	roleName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))
	updatedName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	createdRole := map[string]interface{}{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/role":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			createdRole["id"] = 10101
			createdRole["name"] = body["name"]
			createdRole["description"] = body["description"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          10101,
				"name":        body["name"],
				"description": body["description"],
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/role/10101":
			mu.Lock()
			role := make(map[string]interface{})
			for k, v := range createdRole {
				role[k] = v
			}
			mu.Unlock()
			if role["name"] == nil {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(role)
		case r.Method == "PUT" && r.URL.Path == "/rest/api/3/role/10101":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			createdRole["name"] = body["name"]
			createdRole["description"] = body["description"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          10101,
				"name":        body["name"],
				"description": body["description"],
			})
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/role/10101":
			mu.Lock()
			delete(createdRole, "name")
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_role" "test" {
					name        = %q
					description = "Initial description"
				}`, roleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "name", roleName),
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "description", "Initial description"),
				),
			},
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_project_role" "test" {
					name        = %q
					description = "Updated description"
				}`, updatedName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "name", updatedName),
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "description", "Updated description"),
				),
			},
		},
	})
}

func TestAccProjectRoleResource_Read_NotFound(t *testing.T) {
	roleName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var readCount atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/role":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          10102,
				"name":        roleName,
				"description": "",
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/role/10102":
			readCount.Add(1)
			if readCount.Load() <= 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":          10102,
					"name":        roleName,
					"description": "",
				})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/role/10102":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_role" "test" { name = %q }`, roleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "name", roleName),
				),
			},
			{
				Config:             fmt.Sprintf(`resource "atlassian_jira_project_role" "test" { name = %q }`, roleName),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccProjectRoleResource_Import(t *testing.T) {
	roleName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	createdRole := map[string]interface{}{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/role":
			mu.Lock()
			createdRole["id"] = 10103
			createdRole["name"] = roleName
			createdRole["description"] = "Import test"
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          10103,
				"name":        roleName,
				"description": "Import test",
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/role/10103":
			mu.Lock()
			role := make(map[string]interface{})
			for k, v := range createdRole {
				role[k] = v
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(role)
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/role/10103":
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
				Config: fmt.Sprintf(`resource "atlassian_jira_project_role" "test" {
					name        = %q
					description = "Import test"
				}`, roleName),
			},
			{
				ResourceName:      "atlassian_jira_project_role.test",
				ImportState:       true,
				ImportStateId:     "10103",
				ImportStateVerify: true,
			},
		},
	})
}

func TestAccProjectRoleResource_Delete_NotFound(t *testing.T) {
	roleName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var deleteReturns404 atomic.Bool

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/role":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          10104,
				"name":        roleName,
				"description": "",
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/role/10104":
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":          10104,
				"name":        roleName,
				"description": "",
			})
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/role/10104":
			if deleteReturns404.Load() {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	t.Setenv("ATLASSIAN_URL", mockServer.URL)
	t.Setenv("ATLASSIAN_USER", "test@test.com")
	t.Setenv("ATLASSIAN_TOKEN", "test-token")

	// Make DELETE return 404 to simulate already-deleted resource
	deleteReturns404.Store(true)

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testutil.ProtoV6ProviderFactories,
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_project_role" "test" { name = %q }`, roleName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role.test", "id", "10104"),
				),
			},
		},
	})
}
