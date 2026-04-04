resource "atlassian_jira_project" "example" {
  key              = "EXAMPLE"
  name             = "Example Project"
  project_type_key = "software"
  lead_account_id  = "5a1234abcdef567890123456"
}
