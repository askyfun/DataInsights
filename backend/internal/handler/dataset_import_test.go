package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"data-insights/internal/domain/entity"
	"data-insights/internal/response"
	"data-insights/internal/router"
	"data-insights/internal/service/dataset"
)

/**
 * 本地文件上传端点（issue #138）的 HTTP 契约：
 *
 *   POST /api/datasets/import          multipart: file（必填）、name（可选）
 *   POST /api/datasets/:id/replace     multipart: file（必填）
 *
 * 两个端点都以裸 *gin.Context handler 注册（multipart 绑定走不了泛型路由的
 * ShouldBindJSON，循 /health 与 share View 的手工路由先例）。响应是标准 Envelope：
 * 成功 data 为数据集实体；校验失败 code=20100；业务错误（未配置抽取存储、
 * 非抽取数据集等）按 service 层的 BusinessError 透传。
 */

// buildMultipart 生成一条 multipart/form-data 请求体与 Content-Type。
func buildMultipart(t *testing.T, fieldname, filename, content string, extra map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	if filename != "" {
		fw, err := w.CreateFormFile(fieldname, filename)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := fw.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	for k, v := range extra {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func TestImportFilePassesMultipartToService(t *testing.T) {
	var gotReq dataset.ImportRequest
	mock := &mockDatasetService{
		importFunc: func(ctx context.Context, req dataset.ImportRequest) (*entity.Dataset, error) {
			gotReq = req
			return &entity.Dataset{ID: 7, Name: "销量", Mode: "extract"}, nil
		},
	}
	r := newDatasetTestRouter(NewDatasetHandler(mock))
	body, ctype := buildMultipart(t, "file", "sales.csv", "city,amount\n北京,10\n", map[string]string{"name": "销量"})
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/import", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Code int            `json:"code"`
		Msg  string         `json:"msg"`
		Data entity.Dataset `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != response.CodeSuccess {
		t.Fatalf("code = %d, msg = %s", envelope.Code, envelope.Msg)
	}
	if envelope.Data.ID != 7 || envelope.Data.Mode != "extract" {
		t.Fatalf("unexpected dataset: %+v", envelope.Data)
	}
	if gotReq.Filename != "sales.csv" {
		t.Fatalf("filename = %q", gotReq.Filename)
	}
	if gotReq.Name != "销量" {
		t.Fatalf("name = %q", gotReq.Name)
	}
	if !bytes.Equal(gotReq.Data, []byte("city,amount\n北京,10\n")) {
		t.Fatalf("data = %q", gotReq.Data)
	}
}

func TestImportFileRequiresFileField(t *testing.T) {
	mock := &mockDatasetService{
		importFunc: func(ctx context.Context, req dataset.ImportRequest) (*entity.Dataset, error) {
			t.Fatal("service must not be called when the file field is missing")
			return nil, nil
		},
	}
	r := newDatasetTestRouter(NewDatasetHandler(mock))
	body, ctype := buildMultipart(t, "", "", "", map[string]string{"name": "x"})
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/import", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != response.CodeBadRequest {
		t.Fatalf("code = %d, msg = %s", envelope.Code, envelope.Msg)
	}
}

func TestImportFileMapsBusinessError(t *testing.T) {
	mock := &mockDatasetService{
		importFunc: func(ctx context.Context, req dataset.ImportRequest) (*entity.Dataset, error) {
			return nil, router.NewBusinessError(response.CodeBadRequest,
				"extract storage is not configured: set EXTRACT_DATASOURCE_ID")
		},
	}
	r := newDatasetTestRouter(NewDatasetHandler(mock))
	body, ctype := buildMultipart(t, "file", "a.csv", "a\n1\n", nil)
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/import", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var envelope struct {
		Code int    `json:"code"`
		Msg  string `json:"msg"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != response.CodeBadRequest || envelope.Msg == "" {
		t.Fatalf("code = %d, msg = %q", envelope.Code, envelope.Msg)
	}
}

func TestReplaceFilePassesIDAndFileToService(t *testing.T) {
	var gotID int
	var gotReq dataset.ImportRequest
	mock := &mockDatasetService{
		replaceFunc: func(ctx context.Context, id int, req dataset.ImportRequest) (*entity.Dataset, error) {
			gotID = id
			gotReq = req
			return &entity.Dataset{ID: id, Mode: "extract"}, nil
		},
	}
	r := newDatasetTestRouter(NewDatasetHandler(mock))
	body, ctype := buildMultipart(t, "file", "sales_v2.csv", "city,amount\n上海,20\n", nil)
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/7/replace", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", w.Code, w.Body.String())
	}
	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != response.CodeSuccess {
		t.Fatalf("code = %d", envelope.Code)
	}
	if gotID != 7 || gotReq.Filename != "sales_v2.csv" {
		t.Fatalf("id = %d, filename = %q", gotID, gotReq.Filename)
	}
}

func TestReplaceFileRejectsInvalidID(t *testing.T) {
	mock := &mockDatasetService{
		replaceFunc: func(ctx context.Context, id int, req dataset.ImportRequest) (*entity.Dataset, error) {
			t.Fatal("service must not be called for a non-numeric id")
			return nil, nil
		},
	}
	r := newDatasetTestRouter(NewDatasetHandler(mock))
	body, ctype := buildMultipart(t, "file", "a.csv", "a\n1\n", nil)
	req := httptest.NewRequest(http.MethodPost, "/api/datasets/abc/replace", body)
	req.Header.Set("Content-Type", ctype)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var envelope struct {
		Code int `json:"code"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if envelope.Code != response.CodeBadRequest {
		t.Fatalf("code = %d", envelope.Code)
	}
}
