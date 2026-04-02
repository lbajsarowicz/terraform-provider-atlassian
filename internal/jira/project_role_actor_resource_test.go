package jira_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"
	"github.com/lbajsarowicz/terraform-provider-atlassian/internal/testutil"
)

func TestAccProjectRoleActorResource_user(t *testing.T) {
	var mu sync.Mutex
	actors := []map[string]interface{}{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
			var body map[string][]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			if users, ok := body["user"]; ok {
				for _, u := range users {
					actors = append(actors, map[string]interface{}{
						"id":          1001,
						"displayName": "Test User",
						"type":        "atlassian-user-role-actor",
						"actorUser":   map[string]string{"accountId": u},
					})
				}
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			mu.Lock()
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     10100,
				"name":   "Developers",
				"actors": actors,
			})
			mu.Unlock()
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
			mu.Lock()
			a := make([]map[string]interface{}, len(actors))
			copy(a, actors)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     10100,
				"name":   "Developers",
				"actors": a,
			})
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
			userID := r.URL.Query().Get("user")
			mu.Lock()
			newActors := []map[string]interface{}{}
			for _, a := range actors {
				if a["type"] == "atlassian-user-role-actor" {
					if au, ok := a["actorUser"].(map[string]string); ok && au["accountId"] == userID {
						continue
					}
				}
				newActors = append(newActors, a)
			}
			actors = newActors
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
				Config: `resource "atlassian_jira_project_role_actor" "test" {
					project_key = "TEST"
					role_id     = "10100"
					actor_type  = "atlassianUser"
					actor_value = "5b10ac8d82e05b22cc7d4ef5"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "id", "TEST/10100/atlassianUser/5b10ac8d82e05b22cc7d4ef5"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "project_key", "TEST"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "role_id", "10100"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "actor_type", "atlassianUser"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "actor_value", "5b10ac8d82e05b22cc7d4ef5"),
				),
			},
		},
	})
}

func TestAccProjectRoleActorResource_group(t *testing.T) {
	var mu sync.Mutex
	actors := []map[string]interface{}{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/PROJ/role/10200"):
			var body map[string][]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			mu.Lock()
			if groups, ok := body["group"]; ok {
				for _, g := range groups {
					actors = append(actors, map[string]interface{}{
						"id":          2001,
						"displayName": g,
						"type":        "atlassian-group-role-actor",
						"actorGroup": map[string]string{
							"displayName": g,
							"name":        g,
							"groupId":     "group-id-123",
						},
					})
				}
			}
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			mu.Lock()
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     10200,
				"name":   "Administrators",
				"actors": actors,
			})
			mu.Unlock()
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/PROJ/role/10200"):
			mu.Lock()
			a := make([]map[string]interface{}, len(actors))
			copy(a, actors)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     10200,
				"name":   "Administrators",
				"actors": a,
			})
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/PROJ/role/10200"):
			groupName := r.URL.Query().Get("group")
			mu.Lock()
			newActors := []map[string]interface{}{}
			for _, a := range actors {
				if a["type"] == "atlassian-group-role-actor" {
					if ag, ok := a["actorGroup"].(map[string]string); ok && ag["name"] == groupName {
						continue
					}
				}
				newActors = append(newActors, a)
			}
			actors = newActors
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
				Config: `resource "atlassian_jira_project_role_actor" "test" {
					project_key = "PROJ"
					role_id     = "10200"
					actor_type  = "atlassianGroup"
					actor_value = "jira-administrators"
				}`,
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "id", "PROJ/10200/atlassianGroup/jira-administrators"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "actor_type", "atlassianGroup"),
					resource.TestCheckResourceAttr("atlassian_jira_project_role_actor.test", "actor_value", "jira-administrators"),
				),
			},
		},
	})
}

func TestAccProjectRoleActorResource_Read_NotFound(t *testing.T) {
	var readCount atomic.Int32

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":   10100,
				"name": "Developers",
				"actors": []map[string]interface{}{
					{
						"id":        1001,
						"type":      "atlassian-user-role-actor",
						"actorUser": map[string]string{"accountId": "user-abc"},
					},
				},
			})
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
			readCount.Add(1)
			if readCount.Load() <= 1 {
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":   10100,
					"name": "Developers",
					"actors": []map[string]interface{}{
						{
							"id":        1001,
							"type":      "atlassian-user-role-actor",
							"actorUser": map[string]string{"accountId": "user-abc"},
						},
					},
				})
			} else {
				// Actor removed out-of-band: return role but without the actor
				w.Header().Set("Content-Type", "application/json")
				json.NewEncoder(w).Encode(map[string]interface{}{
					"id":     10100,
					"name":   "Developers",
					"actors": []map[string]interface{}{},
				})
			}
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
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
				Config: `resource "atlassian_jira_project_role_actor" "test" {
					project_key = "TEST"
					role_id     = "10100"
					actor_type  = "atlassianUser"
					actor_value = "user-abc"
				}`,
			},
			{
				Config: `resource "atlassian_jira_project_role_actor" "test" {
					project_key = "TEST"
					role_id     = "10100"
					actor_type  = "atlassianUser"
					actor_value = "user-abc"
				}`,
				ExpectNonEmptyPlan: true,
			},
		},
	})
}

func TestAccProjectRoleActorResource_Import(t *testing.T) {
	var mu sync.Mutex
	actors := []map[string]interface{}{}

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
			mu.Lock()
			actors = append(actors, map[string]interface{}{
				"id":        1001,
				"type":      "atlassian-user-role-actor",
				"actorUser": map[string]string{"accountId": "user-import-123"},
			})
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			mu.Lock()
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     10100,
				"name":   "Developers",
				"actors": actors,
			})
			mu.Unlock()
		case r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
			mu.Lock()
			a := make([]map[string]interface{}, len(actors))
			copy(a, actors)
			mu.Unlock()
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]interface{}{
				"id":     10100,
				"name":   "Developers",
				"actors": a,
			})
		case r.Method == "DELETE" && strings.HasPrefix(r.URL.Path, "/rest/api/3/project/TEST/role/10100"):
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
				Config: `resource "atlassian_jira_project_role_actor" "test" {
					project_key = "TEST"
					role_id     = "10100"
					actor_type  = "atlassianUser"
					actor_value = "user-import-123"
				}`,
			},
			{
				ResourceName:      "atlassian_jira_project_role_actor.test",
				ImportState:       true,
				ImportStateId:     "TEST/10100/atlassianUser/user-import-123",
				ImportStateVerify: true,
			},
		},
	})
}
