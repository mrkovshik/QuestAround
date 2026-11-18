package core

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

// Тот же вектор проверяется в приложении (web/src/lib/transfer.ts) — алгоритмы должны совпадать.
func TestTransferCode(t *testing.T) {
	h := sha256.Sum256([]byte("s3cr3t-token-part"))
	hash := hex.EncodeToString(h[:])
	if got := TransferCode(hash); got != "QA-5NW2-CAUX" {
		t.Fatalf("TransferCode = %q", got)
	}
	if TransferCode("") != "" {
		t.Fatal("пустой хэш — пустой код")
	}
}
