package entity

// DashboardFolder is the outward entity of a dashboard archive folder
// (migration 00007). The tree is assembled by the caller from the flat list —
// the backend never returns nested children, so a rename or a move touches one
// row and no derived structure has to be rewritten.
type DashboardFolder struct {
	ID       string  `json:"id"`
	Name     string  `json:"name"`
	ParentID *string `json:"parent_id"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// DashboardFolderCreateRequest is the business input of
// POST /api/dashboard-folders. FolderID-equivalent ParentID may be empty: both
// "" and nil mean "root level" here, because at creation time "no parent" is
// the only thing an absent value can mean.
type DashboardFolderCreateRequest struct {
	Name     string `json:"name"`
	ParentID string `json:"parent_id"`
}

// DashboardFolderUpdateRequest is the business input of
// PUT /api/dashboard-folders/{id}. Both fields follow the repository's
// "unprovided fields are preserved" convention.
//
// ParentID is three-state and the middle state is the whole point:
//   - nil  → keep the current parent (a pure rename sends this).
//   - ""   → move to root level (JSON cannot tell "absent" from "null", so
//     clearing needs an explicit sentinel; this is the repo's only string
//     sentinel and the dashboard-side folder_id uses the same one).
//   - UUID → move under that folder, subject to the cycle guard.
type DashboardFolderUpdateRequest struct {
	Name     *string `json:"name"`
	ParentID *string `json:"parent_id"`
}
