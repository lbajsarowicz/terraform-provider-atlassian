# Jira Cloud Platform REST API v3 — Endpoint Reference

Base URL: `https://{site}.atlassian.net/rest/api/3`
Auth: Basic (email:api-token) or OAuth 2.0
Rate limits: Per-user per-endpoint, HTTP 429 with Retry-After header

## Groups
| Method | Path | Description |
|--------|------|-------------|
| POST | `/group` | Create group. Body: `{"name": "group-name"}` |
| GET | `/group?groupId={id}` | Get group by ID |
| GET | `/group?groupname={name}` | Get group by name |
| GET | `/group/bulk` | Get all groups (paginated) |
| DELETE | `/group?groupId={id}` | Delete group |
| GET | `/group/member?groupId={id}` | Get group members (paginated) |
| POST | `/group/user?groupId={id}` | Add user. Body: `{"accountId": "..."}` |
| DELETE | `/group/user?groupId={id}&accountId={aid}` | Remove user |

## Projects
| Method | Path | Description |
|--------|------|-------------|
| POST | `/project` | Create. Body: key, name, projectTypeKey, leadAccountId |
| GET | `/project/{projectIdOrKey}` | Get project |
| PUT | `/project/{projectIdOrKey}` | Update project |
| DELETE | `/project/{projectIdOrKey}` | Delete (moves to trash) |
| GET | `/project/search` | Search (paginated) |

**Note:** POST returns partial response (`{self, id, key}` only). Must preserve plan values.

## Permission Schemes
| Method | Path | Description |
|--------|------|-------------|
| GET | `/permissionscheme` | Get all |
| POST | `/permissionscheme` | Create |
| GET | `/permissionscheme/{schemeId}` | Get by ID |
| PUT | `/permissionscheme/{schemeId}` | Update |
| DELETE | `/permissionscheme/{schemeId}` | Delete |

## Permission Scheme Grants
| Method | Path | Description |
|--------|------|-------------|
| GET | `/permissionscheme/{schemeId}/permission` | List grants |
| POST | `/permissionscheme/{schemeId}/permission` | Create grant |
| GET | `/permissionscheme/{schemeId}/permission/{permissionId}` | Get grant |
| DELETE | `/permissionscheme/{schemeId}/permission/{permissionId}` | Delete grant |

Holder types: `group`, `projectRole`, `user`, `applicationRole`, `reporter`, `projectLead`, `assignee`, `anyone`

## Project Permission Scheme Association
| Method | Path | Description |
|--------|------|-------------|
| GET | `/project/{projectKeyOrId}/permissionscheme` | Get scheme for project |
| PUT | `/project/{projectKeyOrId}/permissionscheme` | Assign scheme. Body: `{"id": schemeId}` |

## Issue Types
| Method | Path | Description |
|--------|------|-------------|
| GET | `/issuetype` | Get all |
| POST | `/issuetype` | Create |
| GET | `/issuetype/{id}` | Get |
| PUT | `/issuetype/{id}` | Update |
| DELETE | `/issuetype/{id}` | Delete |

## Issue Type Schemes
| Method | Path | Description |
|--------|------|-------------|
| GET | `/issuetypescheme` | Get all (paginated) |
| POST | `/issuetypescheme` | Create |
| PUT | `/issuetypescheme/{id}` | Update |
| DELETE | `/issuetypescheme/{id}` | Delete |
| PUT | `/issuetypescheme/project` | Assign to project |

## Custom Fields
| Method | Path | Description |
|--------|------|-------------|
| GET | `/field` | Get all fields |
| POST | `/field` | Create custom field |
| PUT | `/field/{fieldId}` | Update |
| DELETE | `/field/{fieldId}` | Delete |

## Custom Field Contexts
| Method | Path | Description |
|--------|------|-------------|
| GET | `/field/{fieldId}/context` | Get contexts (paginated) |
| POST | `/field/{fieldId}/context` | Create |
| PUT | `/field/{fieldId}/context/{contextId}` | Update |
| DELETE | `/field/{fieldId}/context/{contextId}` | Delete |

## Custom Field Options
| Method | Path | Description |
|--------|------|-------------|
| GET | `/field/{fieldId}/context/{contextId}/option` | Get (paginated) |
| POST | `/field/{fieldId}/context/{contextId}/option` | Create |
| PUT | `/field/{fieldId}/context/{contextId}/option` | Update |
| DELETE | `/field/{fieldId}/context/{contextId}/option/{optionId}` | Delete |
| PUT | `/field/{fieldId}/context/{contextId}/option/move` | Reorder |

## Statuses
| Method | Path | Description |
|--------|------|-------------|
| GET | `/statuses/search` | Search (paginated) |
| POST | `/statuses` | Create (bulk) |
| PUT | `/statuses` | Update (bulk) |
| DELETE | `/statuses?id={id}` | Delete |

## Workflows
| Method | Path | Description |
|--------|------|-------------|
| POST | `/workflows/create` | Create (statuses + transitions) |
| POST | `/workflows/update` | Update |
| GET | `/workflow/search` | Search (paginated) |
| DELETE | `/workflow/{entityId}` | Delete |

**LIMITATION:** Conditions, validators, post-functions NOT fully exposed.

## Workflow Schemes
| Method | Path | Description |
|--------|------|-------------|
| GET | `/workflowscheme` | Get all (paginated) |
| POST | `/workflowscheme` | Create |
| GET | `/workflowscheme/{id}` | Get |
| PUT | `/workflowscheme/{id}` | Update (creates draft if active) |
| DELETE | `/workflowscheme/{id}` | Delete |

**CRITICAL:** Draft/publish lifecycle on active projects. Publish via `POST /workflowscheme/{id}/draft/publish`, poll task.

## Screens
| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/screens` | List/Create |
| PUT/DELETE | `/screens/{screenId}` | Update/Delete |

## Screen Schemes
| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/screenscheme` | List/Create |
| PUT/DELETE | `/screenscheme/{id}` | Update/Delete |

## Issue Type Screen Schemes
| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/issuetypescreenscheme` | List/Create |
| PUT/DELETE | `/issuetypescreenscheme/{id}` | Update/Delete |
| PUT | `/issuetypescreenscheme/project` | Assign to project |

## Notification Schemes
| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/notificationscheme` | List/Create |
| GET/PUT/DELETE | `/notificationscheme/{id}` | Get/Update/Delete |

**NOTE:** Update individual notifications = delete+recreate. IDs change.

## Field Configurations
| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/fieldconfiguration` | List/Create |
| PUT/DELETE | `/fieldconfiguration/{id}` | Update/Delete |

## Project Roles
| Method | Path | Description |
|--------|------|-------------|
| GET/POST | `/role` | List/Create |
| GET/PUT/DELETE | `/role/{id}` | Get/Update/Delete |
| GET/POST/DELETE | `/project/{key}/role/{id}` | Actors |

## Pagination Pattern
```
?startAt={offset}&maxResults={limit}
Response: {"startAt":0, "maxResults":50, "total":123, "isLast":false, "values":[...]}
```
