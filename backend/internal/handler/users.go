package handler

import (
	"strconv"

	"data-insights/internal/domain/entity"
	"data-insights/internal/listpage"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/auth"
)

// UserHandler 提供最小用户管理端点（issue #202）。全部路由挂
// Bearer(allowPAT=false) + RequireAdmin：机器令牌不得改人，非管理员也不可读写。
// 开放注册保持关闭——这里是新增账号唯一的入口。
type UserHandler struct {
	svc auth.Service
}

func NewUserHandler(svc auth.Service) *UserHandler { return &UserHandler{svc: svc} }

type userListIn struct {
	Limit   string `form:"limit"`
	Offset  string `form:"offset"`
	Sort    string `form:"sort"`
	Order   string `form:"order"`
	Keyword string `form:"keyword"`
}

type userCreateIn struct {
	Username    string `json:"username" form:"-"`
	DisplayName string `json:"display_name" form:"-"`
	Password    string `json:"password" form:"-"`
	Role        string `json:"role" form:"-"`
}

// userUpdateIn 用指针承载「未提供则保留」：省略字段保留存量，显式 false/空串才覆盖。
type userUpdateIn struct {
	DisplayName *string `json:"display_name" form:"-"`
	Role        *string `json:"role" form:"-"`
	Disabled    *bool   `json:"disabled" form:"-"`
}

type userPasswordIn struct {
	Password string `json:"password" form:"-"`
}

type userStatusOut struct {
	Status string `json:"status"`
}

// List handles GET /api/users
func (h *UserHandler) List(req router.Request[userListIn], res *router.Response[listpage.Result[entity.User]]) error {
	result, err := h.svc.ListUsers(req.Ctx.Request.Context(), listParams(req.In.Limit, req.In.Offset, req.In.Sort, req.In.Order, req.In.Keyword))
	if err != nil {
		return err
	}
	if result.Items == nil {
		result.Items = []entity.User{}
	}
	res.Out = result
	return nil
}

// Create handles POST /api/users
func (h *UserHandler) Create(req router.Request[userCreateIn], res *router.Response[*entity.User]) error {
	u, err := h.svc.CreateUser(req.Ctx.Request.Context(), auth.CreateUserInput{
		Username:    req.In.Username,
		DisplayName: req.In.DisplayName,
		Password:    req.In.Password,
		Role:        req.In.Role,
	})
	if err != nil {
		return err
	}
	res.Out = u
	return nil
}

// Update handles PUT /api/users/:id
func (h *UserHandler) Update(req router.Request[userUpdateIn], res *router.Response[*entity.User]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}
	u, err := h.svc.UpdateUser(req.Ctx.Request.Context(), id, auth.UpdateUserInput{
		DisplayName: req.In.DisplayName,
		Role:        req.In.Role,
		Disabled:    req.In.Disabled,
	})
	if err != nil {
		return err
	}
	res.Out = u
	return nil
}

// ResetPassword handles POST /api/users/:id/password
func (h *UserHandler) ResetPassword(req router.Request[userPasswordIn], res *router.Response[*userStatusOut]) error {
	id, err := strconv.Atoi(req.Ctx.Param("id"))
	if err != nil {
		return router.NewBusinessError(response.CodeBadRequest, "invalid id")
	}
	if err := h.svc.ResetPassword(req.Ctx.Request.Context(), id, req.In.Password); err != nil {
		return err
	}
	res.Out = &userStatusOut{Status: "ok"}
	return nil
}
