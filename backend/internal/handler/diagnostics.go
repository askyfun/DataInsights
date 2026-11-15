package handler

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"

	"github.com/gin-gonic/gin"

	"tablewave/internal/response"
	"tablewave/internal/service/diagnostics"
)

// DiagnosticsHandler 导出诊断包（R-104 / #229）。仅管理员（allowPAT=false）可调。
type DiagnosticsHandler struct {
	svc *diagnostics.Service
}

func NewDiagnosticsHandler(svc *diagnostics.Service) *DiagnosticsHandler {
	return &DiagnosticsHandler{svc: svc}
}

// Get 默认返回 JSON 信封；?format=tar.gz 返回归档文件流。
func (h *DiagnosticsHandler) Get(c *gin.Context) {
	bundle, err := h.svc.Build(c.Request.Context())
	if err != nil {
		response.InternalErrorErr(c, err)
		return
	}
	if c.Query("format") == "tar.gz" {
		data, err := tarGz(bundle)
		if err != nil {
			response.InternalErrorErr(c, err)
			return
		}
		c.Header("Content-Disposition", "attachment; filename=diagnostics.tar.gz")
		c.Data(200, "application/gzip", data)
		return
	}
	response.Success(c, bundle)
}

// tarGz 把诊断 JSON 打进单文件 tar.gz（不含其它敏感源）。
func tarGz(bundle *diagnostics.Bundle) ([]byte, error) {
	raw, err := json.MarshalIndent(bundle, "", "  ")
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "diagnostics.json", Mode: 0o644, Size: int64(len(raw))}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(raw); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gz.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
