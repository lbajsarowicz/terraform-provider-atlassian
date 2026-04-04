resource "atlassian_jira_project_role_actor" "example" {
  project_key = atlassian_jira_project.example.key
  role_id     = atlassian_jira_project_role.example.id
  actor_type  = "atlassianGroup"
  actor_value = atlassian_jira_group.example.name
}
