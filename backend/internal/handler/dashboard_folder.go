package handler

import (
	"data-insights/internal/domain/entity"
	"data-insights/internal/router"
	"data-insights/internal/service/dashboard"
)

// DashboardFolderHandler handles the dashboard archive folder HTTP surface: the
// flat tree read plus CRUD. It is a separate handler from DashboardHandler
// because the two resources have different lifecycles (a folder carries the
// cycle / non-empty guards), mirroring the two service interfaces.
type DashboardFolderHandler struct {
	svc dashboard.FolderService
}

// NewDashboardFolderHandler creates a new DashboardFolderHandler
func NewDashboardFolderHandler(svc dashboard.FolderService) *DashboardFolderHandler {
	return &DashboardFolderHandler{svc: svc}
}

// dashboardFolderPathIn is the In shape for routes that carry only the {id}
// path param: nothing to bind, the id is read from req.Ctx.
type dashboardFolderPathIn struct{}

// dashboardFolderListIn is the In shape of GET /api/dashboard-folders. The
// first phase has no pagination and no parent filter, so there is nothing to
// bind — the type exists because the generic router needs one.
type dashboardFolderListIn struct{}

// dashboardFolderCreateIn is the JSON body of POST /api/dashboard-folders. It
// mirrors entity.DashboardFolderCreateRequest one-to-one (guarded by
// contract_parity_test.go) with form:"-" on every field so the router's
// ShouldBindQuery pass cannot read query params into the body struct.
//
// ParentID binds as a plain string: absent and "" both mean root level at
// creation, so the two states do not need to be distinguished here.
type dashboardFolderCreateIn struct {
	Name     string `json:"name" form:"-"`
	ParentID string `json:"parent_id" form:"-"`
}

// dashboardFolderUpdateIn is the JSON body of PUT /api/dashboard-folders/:id.
// Like entity.DashboardFolderUpdateRequest every field is a pointer: absent (or
// null) means "keep the stored value". ParentID's three states (nil / "" /
// UUID) are the contract's move semantics — see the entity doc comment.
type dashboardFolderUpdateIn struct {
	Name     *string `json:"name" form:"-"`
	ParentID *string `json:"parent_id" form:"-"`
}

// List handles GET /api/dashboard-folders.
//
// The payload is deliberately flat: the tree is assembled by the caller from
// parent_id, so no nesting is invented here and a move stays a one-row write.
func (h *DashboardFolderHandler) List(
	req router.Request[dashboardFolderListIn], res *router.Response[[]entity.DashboardFolder],
) error {
	folders, err := h.svc.ListFolders(req.Ctx.Request.Context())
	if err != nil {
		return err
	}
	if folders == nil {
		folders = []entity.DashboardFolder{}
	}
	res.Out = folders
	return nil
}

// Get handles GET /api/dashboard-folders/:id
func (h *DashboardFolderHandler) Get(
	req router.Request[dashboardFolderPathIn], res *router.Response[*entity.DashboardFolder],
) error {
	id, err := parseDashboardID(req.Ctx.Param("id"))
	if err != nil {
		return err
	}

	result, err := h.svc.GetFolder(req.Ctx.Request.Context(), id)
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Create handles POST /api/dashboard-folders. The id is server-generated and
// an empty parent_id means root level.
func (h *DashboardFolderHandler) Create(
	req router.Request[dashboardFolderCreateIn], res *router.Response[*entity.DashboardFolder],
) error {
	result, err := h.svc.CreateFolder(req.Ctx.Request.Context(), entity.DashboardFolderCreateRequest{
		Name:     req.In.Name,
		ParentID: req.In.ParentID,
	})
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Update handles PUT /api/dashboard-folders/:id (rename and/or move).
func (h *DashboardFolderHandler) Update(
	req router.Request[dashboardFolderUpdateIn], res *router.Response[*entity.DashboardFolder],
) error {
	id, err := parseDashboardID(req.Ctx.Param("id"))
	if err != nil {
		return err
	}

	result, err := h.svc.UpdateFolder(req.Ctx.Request.Context(), id, entity.DashboardFolderUpdateRequest{
		Name:     req.In.Name,
		ParentID: req.In.ParentID,
	})
	if err != nil {
		return err
	}
	res.Out = result
	return nil
}

// Delete handles DELETE /api/dashboard-folders/:id (soft delete, empty only).
// The "folder not empty" rejection is a 20400 business error produced by the
// service, so the handler stays a thin binding layer.
func (h *DashboardFolderHandler) Delete(
	req router.Request[dashboardFolderPathIn], res *router.Response[dashboardStatusOut],
) error {
	id, err := parseDashboardID(req.Ctx.Param("id"))
	if err != nil {
		return err
	}

	if err := h.svc.DeleteFolder(req.Ctx.Request.Context(), id); err != nil {
		return err
	}
	res.Out = dashboardStatusOut{Status: "ok"}
	return nil
}
