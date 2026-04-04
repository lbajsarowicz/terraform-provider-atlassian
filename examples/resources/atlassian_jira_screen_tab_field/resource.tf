resource "atlassian_jira_screen_tab_field" "example" {
  screen_id = atlassian_jira_screen.example.id
  tab_id    = atlassian_jira_screen_tab.example.id
  field_id  = "summary"
}
