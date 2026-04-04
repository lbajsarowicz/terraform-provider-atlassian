resource "atlassian_jira_screen_tab" "example" {
  screen_id = atlassian_jira_screen.example.id
  name      = "Example Tab"
}
