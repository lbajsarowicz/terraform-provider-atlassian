resource "atlassian_jira_project_permission_scheme" "example" {
  project_key = atlassian_jira_project.example.key
  scheme_id   = atlassian_jira_permission_scheme.example.id
}
