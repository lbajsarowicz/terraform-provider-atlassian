resource "atlassian_jira_permission_scheme_grant" "example" {
  scheme_id        = atlassian_jira_permission_scheme.example.id
  permission       = "BROWSE_PROJECTS"
  holder_type      = "group"
  holder_parameter = atlassian_jira_group.example.name
}
