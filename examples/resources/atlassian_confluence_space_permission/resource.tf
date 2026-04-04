resource "atlassian_confluence_space_permission" "example" {
  space_key        = atlassian_confluence_space.example.key
  space_id         = atlassian_confluence_space.example.id
  principal_type   = "group"
  principal_id     = atlassian_jira_group.example.group_id
  operation_key    = "read"
  operation_target = "space"
}
