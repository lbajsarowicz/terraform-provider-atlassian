resource "atlassian_jira_custom_field" "example" {
  name = "Example Custom Field"
  type = "com.atlassian.jira.plugin.system.customfieldtypes:textfield"
}
