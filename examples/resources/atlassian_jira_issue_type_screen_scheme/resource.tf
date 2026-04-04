resource "atlassian_jira_issue_type_screen_scheme" "example" {
  name = "Example Issue Type Screen Scheme"
  mappings = {
    default = atlassian_jira_screen_scheme.example.id
  }
}
