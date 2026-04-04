resource "atlassian_jira_project_issue_type_scheme" "example" {
  project_id         = atlassian_jira_project.example.id
  issue_type_scheme_id = atlassian_jira_issue_type_scheme.example.id
}
