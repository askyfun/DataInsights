package datasource

import "testing"

func TestSafeIdentifierRejectsInjection(t *testing.T) {
	cases := map[string]bool{
		"region":       true,
		"bi_orders.id": true,
		"a;b":          false,
		"a'b":          false,
		"a\"b":         false,
		"a-b":          false,
		"\"\")(; DROP": false,
		"":             false,
		"scoreavg":     true,
	}
	for in, want := range cases {
		if got := IsValidIdentifier(in); got != want {
			t.Errorf("IsValidIdentifier(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestNewDriverUnknownTypeReturnsError(t *testing.T) {
	d, err := NewDriver(DriverType("oracle"))
	if err == nil {
		t.Fatal("expected error for unknown driver type")
	}
	if d != nil {
		t.Fatalf("expected nil driver, got %v", d)
	}
}

// TestConvertValueDecimalNotBase64 复现 StarRocks DECIMAL 列被 JSON 序列化为 base64 的缺陷。
// go-sql-driver/mysql 对 DECIMAL 返回 []byte，而 TextTypeNames 白名单只含字符类型，
// 导致数值列走 "二进制类型保持 []byte" 分支 → 客户端拿到 "MTAzLjUxMjUwMDA=" 而非 103.5125。
func TestConvertValueDecimalNotBase64(t *testing.T) {
	if got := convertValue([]byte("103.5125000"), "DECIMAL"); got != "103.5125000" {
		t.Errorf("DECIMAL: got %#v, want %q", got, "103.5125000")
	}
	if got := convertValue([]byte("2026-08-01"), "DATE"); got != "2026-08-01" {
		t.Errorf("DATE: got %#v, want %q", got, "2026-08-01")
	}
	if got := convertValue([]byte("2026-08-01 12:00:00"), "DATETIME"); got != "2026-08-01 12:00:00" {
		t.Errorf("DATETIME: got %#v, want %q", got, "2026-08-01 12:00:00")
	}
	if got := convertValue([]byte("北京"), "VARCHAR"); got != "北京" {
		t.Errorf("VARCHAR: got %#v, want %q", got, "北京")
	}
}

func TestConvertValueKeepsBinaryAsBase64(t *testing.T) {
	raw := []byte{0x01, 0x02, 0xff}
	if got := convertValue(raw, "BLOB"); got == nil {
		t.Error("BLOB: got nil, want raw []byte")
	} else if b, ok := got.([]byte); !ok || len(b) != len(raw) {
		t.Errorf("BLOB: got %#v, want raw []byte preserved", got)
	}
	if got := convertValue(raw, int32(ByteaOID)); got == nil {
		t.Error("bytea: got nil, want raw []byte")
	} else if _, ok := got.([]byte); !ok {
		t.Errorf("bytea: got %#v, want raw []byte preserved", got)
	}
}

func TestConvertValueNonByteValuesUnchanged(t *testing.T) {
	if got := convertValue(int64(8), "BIGINT"); got != int64(8) {
		t.Errorf("BIGINT int64: got %#v, want int64(8)", got)
	}
	if got := convertValue(nil, "DECIMAL"); got != nil {
		t.Errorf("nil: got %#v, want nil", got)
	}
}
