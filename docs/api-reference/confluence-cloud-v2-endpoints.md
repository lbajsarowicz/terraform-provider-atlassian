# Confluence Cloud REST API v2 — Endpoint Reference

Base URL: `https://{site}.atlassian.net/wiki/api/v2`
Auth: Basic (email:api-token) or OAuth 2.0

## Spaces
| Method | Path | Description |
|--------|------|-------------|
| GET | `/spaces` | Get all (cursor-based pagination) |
| POST | `/spaces` | Create. Body: key, name, description |
| GET | `/spaces/{id}` | Get by ID |
| PUT | `/spaces/{id}` | Update |
| DELETE | `/spaces/{id}` | Delete |

## Space Permissions
| Method | Path | Description |
|--------|------|-------------|
| GET | `/spaces/{id}/permissions` | Get assignments |
| POST | `/spaces/{id}/permissions` | Add permission |
| DELETE | `/spaces/{id}/permissions/{permissionId}` | Remove |

**NOTE:** No UPDATE — delete+recreate. Additive model (grant only, no deny).

## Space Properties
| Method | Path | Description |
|--------|------|-------------|
| GET | `/spaces/{id}/properties` | Get |
| POST | `/spaces/{id}/properties` | Create |
| PUT | `/spaces/{id}/properties/{propertyId}` | Update |
| DELETE | `/spaces/{id}/properties/{propertyId}` | Delete |

## Pagination (cursor-based, different from Jira)
```
?cursor={cursor}&limit={limit}
Response includes `_links.next` for next page URL.
```
