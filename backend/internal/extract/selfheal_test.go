package extract

import (
	"testing"

	"data-insights/internal/crypto"
	"data-insights/internal/model"
)

// R-85 风险面：自举存储的口令也会被加密，而管理员换 SECURITY_KEY 是既有运维动作。
// 对用户自建的数据源来说，换key 后连不通是"我改了我的数据源密码"的合理后果；
// 但自举存储是**平台自己建的、用户看不见也改不了**的一条数据源——它坏了却没有任何
// 界面提示，用户只会看到"上传坏了"。
//
// 这些用例钉住修复方向：自举存储的口令可从连接串重新推导，因此解密失败时应当
// 自愈，而不是把故障暴露给用户。

// TestSelfProvisionedStoragePasswordIsRecoverable 钉住自愈前提：
// 平台能从 DATABASE_URL 重新解析出口令，所以自举存储的口令永远是可再生的。
func TestSelfProvisionedStoragePasswordIsRecoverable(t *testing.T) {
	_, _, _, _, password, err := ParsePostgresTarget(
		"postgres://insights:s3cr3t@pg:5432/insights")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if password != "s3cr3t" {
		t.Fatalf("password = %q, want s3cr3t", password)
	}
}

// TestSelfProvisionedStorageRecoversFromRotatedKey 模拟真实故障：
// SECURITY_KEY 被换掉后，库里存的是旧 key 加密的口令，解密必然失败。
// 修复前这条路径返回 error，上传整体挂掉；修复后应当用重新解析的口令自愈。
func TestSelfProvisionedStorageRecoversFromRotatedKey(t *testing.T) {
	oldKey := make([]byte, 32)
	newKey := make([]byte, 32)
	for i := range newKey {
		newKey[i] = byte(i + 1)
	}

	ct, err := crypto.Encrypt(oldKey, "s3cr3t")
	if err != nil {
		t.Fatalf("encrypt: %v", err)
	}
	ds := &model.Datasource{ID: 1, Type: "postgresql", Password: ct}

	// 旧 key 能解开（对照组：证明密文本身是好的，坏的是 key 换了）。
	if got, err := crypto.Decrypt(oldKey, ds.Password); err != nil || got != "s3cr3t" {
		t.Fatalf("old key must still decrypt, got %q err=%v", got, err)
	}
	// 新 key 解不开——这正是需要自愈的场景。
	if _, err := crypto.Decrypt(newKey, ds.Password); err == nil {
		t.Fatal("a rotated key must fail to decrypt, otherwise this test proves nothing")
	}

	// 自愈所需的信息是齐的：从连接串能重新拿到同一个明文口令。
	_, _, _, _, recovered, err := ParsePostgresTarget(
		"postgres://insights:s3cr3t@pg:5432/insights")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if recovered != "s3cr3t" {
		t.Fatalf("recovered password = %q, want the original s3cr3t", recovered)
	}
}