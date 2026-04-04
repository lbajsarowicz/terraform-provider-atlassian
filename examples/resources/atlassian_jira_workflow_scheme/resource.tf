resource "atlassian_jira_workflow_scheme" "example" {
  name             = "Example Workflow Scheme"
  default_workflow = atlassian_jira_workflow.example.name
}
