resource "atlassian_jira_workflow" "example" {
  name = "Example Workflow"
  statuses = [
    atlassian_jira_status.example.id,
  ]
}
