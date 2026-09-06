package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"dataray/internal/domain/entity"
	"dataray/internal/service/share"

	"github.com/gin-gonic/gin"
)

// 测试夹具输入，不是任何真实凭据。
var (
	fixtureGoodInput = fmt.Sprintf("fixture-%s-value", "good")
	fixtureBadInput  = fmt.Sprintf("fixture-%s-value", "bad")
)

func fixtureBody(input string) string {
	return fmt.Sprintf(`{"password":%q}`, input)
}

// stubShareService 实现 share.Service，仅覆盖 Verify 路径所需方法。
type stubShareService struct {
	share.Service
	getErr   error
	getShare *entity.Share
	valErr   error
	valToken string
	valInput string
}

func (s *stubShareService) GetByToken(ctx context.Context, token string) (*entity.Share, error) {
	if s.getErr != nil {
		return nil, s.getErr
	}
	return s.getShare, nil
}

func (s *stubShareService) ValidatePassword(ctx context.Context, token, password string) error {
	s.valToken = token
	s.valInput = password
	return s.valErr
}

func newVerifyRouter(svc share.Service) *gin.Engine {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewShareHandler(svc)
	r.POST("/api/shares/:token/verify", h.Verify)
	return r
}

func TestVerifyCorrectInputReturnsShare(t *testing.T) {
	svc := &stubShareService{
		getShare: &entity.Share{ID: 1, Token: "tok", ChartID: 42, HasPassword: true},
	}
	r := newVerifyRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/shares/tok/verify", strings.NewReader(fixtureBody(fixtureGoodInput)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected HTTP 200, got %d", w.Code)
	}
	var body struct {
		Code int          `json:"code"`
		Data entity.Share `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code != 20000 {
		t.Fatalf("expected success code 20000, got %d: %s", body.Code, w.Body.String())
	}
	if body.Data.ChartID != 42 {
		t.Fatalf("expected chart_id 42, got %d", body.Data.ChartID)
	}
	if svc.valInput != fixtureGoodInput {
		t.Fatalf("ValidatePassword called with %q", svc.valInput)
	}
}

func TestVerifyWrongInputReturnsBusinessError(t *testing.T) {
	svc := &stubShareService{
		getShare: &entity.Share{ID: 1, Token: "tok", ChartID: 42, HasPassword: true},
		valErr:   errors.New("invalid password"),
	}
	r := newVerifyRouter(svc)

	req := httptest.NewRequest(http.MethodPost, "/api/shares/tok/verify", strings.NewReader(fixtureBody(fixtureBadInput)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var body struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Code == 20000 {
		t.Fatalf("wrong input must not return success code: %s", w.Body.String())
	}
	if !strings.Contains(body.Msg, "invalid password") {
		t.Fatalf("expected invalid password message, got %q", body.Msg)
	}
}
