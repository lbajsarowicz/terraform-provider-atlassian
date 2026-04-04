resource "atlassian_jira_project_workflow_scheme" "example" {
  project_id        = atlassian_jira_project.example.id
  workflow_scheme_id = atlassian_jira_workflow_scheme.example.id
}
