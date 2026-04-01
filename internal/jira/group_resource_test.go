package jira_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestAccGroupResource_basic(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/group":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"groupId": "test-group-id-123",
				"name":    body["name"],
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			groupName := r.URL.Query().Get("groupname")
			if groupID == "test-group-id-123" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": "test-group-id-123",
					"name":    "tf-test-group",
				})
			} else if groupName != "" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": "test-group-id-123",
					"name":    groupName,
				})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			if groupID == "test-group-id-123" {
				w.WriteHeader(http.StatusNoContent)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
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
				Config: `resource "atlassian_jira_group" "test" { name = "tf-test-group" }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_group.test", "name", "tf-test-group"),
					resource.TestCheckResourceAttr("atlassian_jira_group.test", "group_id", "test-group-id-123"),
				),
			},
		},
	})
}

func TestAccGroupResource_Read_NotFound(t *testing.T) {
	callCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/group":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"groupId": "test-group-id-456",
				"name":    "tf-test-vanishing-group",
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			if groupID != "test-group-id-456" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			callCount++
			if callCount <= 1 {
				// First read after create succeeds
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": "test-group-id-456",
					"name":    "tf-test-vanishing-group",
				})
			} else {
				// Subsequent reads return 404 (deleted out-of-band)
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/group":
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
		Steps: []resource.TestStep{
			{
				Config: `resource "atlassian_jira_group" "test" { name = "tf-test-vanishing-group" }`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_group.test", "name", "tf-test-vanishing-group"),
				),
			},
			// Second step: Read will 404 -> resource removed from state -> plan shows recreation
			{
				Config:             `resource "atlassian_jira_group" "test" { name = "tf-test-vanishing-group" }`,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccGroupResource_Import(t *testing.T) {
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/group":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"groupId": "imported-group-id",
				"name":    "tf-test-import-group",
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			groupName := r.URL.Query().Get("groupname")
			if groupID == "imported-group-id" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": "imported-group-id",
					"name":    "tf-test-import-group",
				})
			} else if groupName == "tf-test-import-group" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": "imported-group-id",
					"name":    "tf-test-import-group",
				})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			if groupID == "imported-group-id" {
				w.WriteHeader(http.StatusNoContent)
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
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
				Config: `resource "atlassian_jira_group" "test" { name = "tf-test-import-group" }`,
			},
			{
				ResourceName:                         "atlassian_jira_group.test",
				ImportState:                          true,
				ImportStateId:                        "tf-test-import-group",
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "group_id",
			},
		},
	})
}
