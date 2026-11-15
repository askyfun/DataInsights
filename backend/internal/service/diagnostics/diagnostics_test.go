package diagnostics

import (
	"encoding/json"
	"strings"
	"testing"
)

// 脱敏红线：配置快照只出键名与「是否已设置」，序列化后不得出现任何值或 KEY= 形态。
func TestConfigSnapshotNoValues(t *testing.T) {
	t.Setenv("DATABASE_URL", "postgres://u:supersecretpw@h/db")
	t.Setenv("SECURITY_KEY", "topsecretkeyvalue")
	snap := ConfigSnapshot()
	raw, err := json.Marshal(snap)
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, forbid := range []string{"supersecretpw", "topsecretkeyvalue", "DATABASE_URL=", "SECURITY_KEY="} {
		if strings.Contains(s, forbid) {
			t.Fatalf("config snapshot leaked %q: %s", forbid, s)
		}
	}
	found := false
	for _, it := range snap {
		if it.Name == "DATABASE_URL" {
			found = true
			if !it.Set {
				t.Fatalf("DATABASE_URL should be marked set=true")
			}
		}
	}
	if !found {
		t.Fatalf("DATABASE_URL key missing from snapshot")
	}
}
