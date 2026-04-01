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

func TestAccGroupResource_basic(t *testing.T) {
	groupName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	createdGroup := map[string]string{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/group":
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			createdGroup["groupId"] = "test-group-id-123"
			createdGroup["name"] = body["name"]
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"groupId": "test-group-id-123",
				"name":    body["name"],
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			groupQName := r.URL.Query().Get("groupname")
			mu.Lock()
			g := make(map[string]string)
			for k, v := range createdGroup {
				g[k] = v
			}
			mu.Unlock()
			if groupID == g["groupId"] {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(g)
			} else if groupQName != "" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": g["groupId"],
					"name":    groupQName,
				})
			} else {
				w.WriteHeader(http.StatusNotFound)
			}
		case r.Method == "DELETE" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			mu.Lock()
			gID := createdGroup["groupId"]
			mu.Unlock()
			if groupID == gID {
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
		// TODO: query real API when running against live instance
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_group" "test" { name = %q }`, groupName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_group.test", "name", groupName),
					resource.TestCheckResourceAttr("atlassian_jira_group.test", "group_id", "test-group-id-123"),
				),
			},
		},
	})
}

func TestAccGroupResource_Read_NotFound(t *testing.T) {
	groupName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var callCount atomic.Int32
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/group":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"groupId": "test-group-id-456",
				"name":    groupName,
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			if groupID != "test-group-id-456" {
				w.WriteHeader(http.StatusNotFound)
				return
			}
			callCount.Add(1)
			if callCount.Load() <= 1 {
				// First read after create succeeds
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": "test-group-id-456",
					"name":    groupName,
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
		// TODO: query real API when running against live instance
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_group" "test" { name = %q }`, groupName),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_group.test", "name", groupName),
				),
			},
			// Second step: Read will 404 -> resource removed from state -> plan shows recreation
			{
				Config:             fmt.Sprintf(`resource "atlassian_jira_group" "test" { name = %q }`, groupName),
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccGroupResource_Import(t *testing.T) {
	groupName := fmt.Sprintf("tf-test-%s", acctest.RandStringFromCharSet(8, acctest.CharSetAlphaNum))

	var mu sync.Mutex
	createdGroup := map[string]string{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && r.URL.Path == "/rest/api/3/group":
			mu.Lock()
			createdGroup["groupId"] = "imported-group-id"
			createdGroup["name"] = groupName
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"groupId": "imported-group-id",
				"name":    groupName,
			})
		case r.Method == "GET" && r.URL.Path == "/rest/api/3/group":
			groupID := r.URL.Query().Get("groupId")
			groupQName := r.URL.Query().Get("groupname")
			mu.Lock()
			g := make(map[string]string)
			for k, v := range createdGroup {
				g[k] = v
			}
			mu.Unlock()
			if groupID == "imported-group-id" {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": "imported-group-id",
					"name":    g["name"],
				})
			} else if groupQName == g["name"] {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]string{
					"groupId": "imported-group-id",
					"name":    g["name"],
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
		// TODO: query real API when running against live instance
		CheckDestroy: func(s *terraform.State) error {
			return nil
		},
		Steps: []resource.TestStep{
			{
				Config: fmt.Sprintf(`resource "atlassian_jira_group" "test" { name = %q }`, groupName),
			},
			{
				ResourceName:                         "atlassian_jira_group.test",
				ImportState:                          true,
				ImportStateId:                        groupName,
				ImportStateVerify:                    true,
				ImportStateVerifyIdentifierAttribute: "group_id",
			},
		},
	})
}
