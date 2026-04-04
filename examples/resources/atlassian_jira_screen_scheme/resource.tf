resource "atlassian_jira_screen_scheme" "example" {
  name = "Example Screen Scheme"
  screens = {
    default = atlassian_jira_screen.example.id
  }
}
