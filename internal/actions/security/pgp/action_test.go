package pgp

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func TestActionEncryptDecrypt(t *testing.T) {
	t.Parallel()

	entity, err := openpgp.NewEntity("Alice Example", "", "alice@example.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}
	privKey, pubKey := exportEntity(t, entity)

	tempDir := t.TempDir()
	cipherPath := filepath.Join(tempDir, "cipher.asc")

	payload := map[string]any{
		"action": "PGP",
		"steps": []any{
			map[string]any{
				"id":        "import.private",
				"operation": "IMPORT_KEY",
				"alias":     "alice",
				"key":       privKey,
			},
			map[string]any{
				"id":        "import.public",
				"operation": "IMPORT_KEY",
				"alias":     "alice_pub",
				"key":       pubKey,
			},
			map[string]any{
				"id":             "encrypt",
				"operation":      "ENCRYPT",
				"recipients":     []string{"alice_pub"},
				"message":        "Hola mundo",
				"signWith":       "alice",
				"outputPath":     cipherPath,
				"armor":          true,
				"fileName":       "message.txt",
				"binary":         false,
				"resultEncoding": "base64",
			},
			map[string]any{
				"id":               "decrypt",
				"operation":        "DECRYPT",
				"messagePath":      cipherPath,
				"keyAliases":       []string{"alice"},
				"requireSignature": true,
				"allowedSigners":   []string{"alice"},
				"outputEncoding":   "utf8",
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}

	result, err := Action{}.Execute(context.Background(), raw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Type != flow.ResultTypeJSON {
		t.Fatalf("unexpected result type: %s", result.Type)
	}

	data, err := json.Marshal(result.Value)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}

	var decoded struct {
		Steps []struct {
			ID        string          `json:"id"`
			Operation string          `json:"operation"`
			Result    json.RawMessage `json:"result"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal decoded result: %v", err)
	}
	if len(decoded.Steps) != 4 {
		t.Fatalf("unexpected step count: %d", len(decoded.Steps))
	}

	var enc encryptResult
	if err := json.Unmarshal(decoded.Steps[2].Result, &enc); err != nil {
		t.Fatalf("Unmarshal encrypt result: %v", err)
	}
	if !enc.Armored {
		t.Fatalf("expected armored ciphertext")
	}
	if enc.SignerAlias != "alice" {
		t.Fatalf("unexpected signer alias: %q", enc.SignerAlias)
	}
	if enc.Ciphertext.Value == "" {
		t.Fatalf("ciphertext value is empty")
	}
	if _, err := os.Stat(cipherPath); err != nil {
		t.Fatalf("expected ciphertext file: %v", err)
	}

	var dec decryptResult
	if err := json.Unmarshal(decoded.Steps[3].Result, &dec); err != nil {
		t.Fatalf("Unmarshal decrypt result: %v", err)
	}
	if !dec.Verified {
		t.Fatalf("expected verified signature")
	}
	if dec.SignerAlias != "alice" {
		t.Fatalf("unexpected decrypt signer alias: %q", dec.SignerAlias)
	}
	if dec.Plaintext.Value != "Hola mundo" {
		t.Fatalf("unexpected plaintext: %q", dec.Plaintext.Value)
	}
}

func TestActionSignVerify(t *testing.T) {
	t.Parallel()

	entity, err := openpgp.NewEntity("Signer", "", "signer@example.com", nil)
	if err != nil {
		t.Fatalf("NewEntity: %v", err)
	}
	privKey, pubKey := exportEntity(t, entity)

	tmpDir := t.TempDir()
	sigPath := filepath.Join(tmpDir, "signature.asc")

	payload := map[string]any{
		"action": "PGP",
		"steps": []any{
			map[string]any{
				"operation": "IMPORT_KEY",
				"alias":     "signer",
				"key":       privKey,
			},
			map[string]any{
				"operation": "IMPORT_KEY",
				"alias":     "signer_pub",
				"key":       pubKey,
			},
			map[string]any{
				"operation":  "SIGN_DETACHED",
				"id":         "sign",
				"signWith":   "signer",
				"message":    "Contenido importante",
				"armor":      true,
				"outputPath": sigPath,
			},
			map[string]any{
				"operation":     "VERIFY_DETACHED",
				"id":            "verify",
				"message":       "Contenido importante",
				"signaturePath": sigPath,
				"keyAliases":    []string{"signer_pub"},
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}

	result, err := Action{}.Execute(context.Background(), raw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Type != flow.ResultTypeJSON {
		t.Fatalf("unexpected result type: %s", result.Type)
	}

	data, err := json.Marshal(result.Value)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}

	var decoded struct {
		Steps []struct {
			ID        string          `json:"id"`
			Operation string          `json:"operation"`
			Result    json.RawMessage `json:"result"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal decoded result: %v", err)
	}
	if len(decoded.Steps) != 4 {
		t.Fatalf("unexpected step count: %d", len(decoded.Steps))
	}

	var signRes signResult
	if err := json.Unmarshal(decoded.Steps[2].Result, &signRes); err != nil {
		t.Fatalf("Unmarshal sign result: %v", err)
	}
	if !signRes.Armored {
		t.Fatalf("expected armored signature")
	}
	if signRes.SignerAlias != "signer" {
		t.Fatalf("unexpected signer alias: %q", signRes.SignerAlias)
	}
	if _, err := os.Stat(sigPath); err != nil {
		t.Fatalf("expected signature file: %v", err)
	}

	var verifyRes verifyResult
	if err := json.Unmarshal(decoded.Steps[3].Result, &verifyRes); err != nil {
		t.Fatalf("Unmarshal verify result: %v", err)
	}
	if !verifyRes.Verified {
		t.Fatalf("expected verified signature")
	}
	if verifyRes.SignerAlias != "signer_pub" && verifyRes.SignerAlias != "signer" {
		t.Fatalf("unexpected verify signer alias: %q", verifyRes.SignerAlias)
	}
}

func TestActionGenerateKeyAndSymmetricEncrypt(t *testing.T) {
	t.Parallel()

	tempDir := t.TempDir()
	fileOne := filepath.Join(tempDir, "uno.txt")
	fileTwo := filepath.Join(tempDir, "dos.txt")
	cipherOne := filepath.Join(tempDir, "uno.txt.pgp")
	cipherTwo := filepath.Join(tempDir, "dos.txt.pgp")
	plainTwo := filepath.Join(tempDir, "dos.recuperado.txt")
	privPath := filepath.Join(tempDir, "demo_private.asc")
	pubPath := filepath.Join(tempDir, "demo_public.asc")

	contentOne := "Primer documento secreto"
	contentTwo := "Segundo documento confidencial"

	if err := os.WriteFile(fileOne, []byte(contentOne), 0o600); err != nil {
		t.Fatalf("Write fileOne: %v", err)
	}
	if err := os.WriteFile(fileTwo, []byte(contentTwo), 0o600); err != nil {
		t.Fatalf("Write fileTwo: %v", err)
	}

	payload := map[string]any{
		"action": "PGP",
		"steps": []any{
			map[string]any{
				"id":             "generate",
				"operation":      "GENERATE_KEY",
				"alias":          "demo",
				"name":           "Demo User",
				"email":          "demo@example.com",
				"privateKeyPath": privPath,
				"publicKeyPath":  pubPath,
			},
			map[string]any{
				"id":          "encrypt1",
				"operation":   "ENCRYPT",
				"password":    "SymmetricPassword123",
				"messagePath": fileOne,
				"outputPath":  cipherOne,
				"armor":       true,
				"fileName":    "uno.txt",
			},
			map[string]any{
				"id":          "encrypt2",
				"operation":   "ENCRYPT",
				"password":    "SymmetricPassword123",
				"messagePath": fileTwo,
				"outputPath":  cipherTwo,
				"armor":       true,
				"fileName":    "dos.txt",
			},
			map[string]any{
				"id":             "decrypt1",
				"operation":      "DECRYPT",
				"messagePath":    cipherOne,
				"passwords":      []string{"SymmetricPassword123"},
				"outputEncoding": "utf8",
			},
			map[string]any{
				"id":          "decrypt2",
				"operation":   "DECRYPT",
				"messagePath": cipherTwo,
				"passwords":   []string{"SymmetricPassword123"},
				"outputPath":  plainTwo,
			},
		},
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("Marshal payload: %v", err)
	}

	result, err := Action{}.Execute(context.Background(), raw, &registry.ExecutionContext{})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.Type != flow.ResultTypeJSON {
		t.Fatalf("unexpected result type: %s", result.Type)
	}

	data, err := json.Marshal(result.Value)
	if err != nil {
		t.Fatalf("Marshal result: %v", err)
	}

	var decoded struct {
		Steps []struct {
			ID        string          `json:"id"`
			Operation string          `json:"operation"`
			Result    json.RawMessage `json:"result"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal decoded result: %v", err)
	}
	if len(decoded.Steps) != 5 {
		t.Fatalf("unexpected step count: %d", len(decoded.Steps))
	}

	var gen generateKeyResult
	if err := json.Unmarshal(decoded.Steps[0].Result, &gen); err != nil {
		t.Fatalf("Unmarshal generate result: %v", err)
	}
	if gen.Alias != "demo" {
		t.Fatalf("unexpected alias: %q", gen.Alias)
	}
	if gen.PrivateKey.Value == "" || gen.PublicKey.Value == "" {
		t.Fatalf("expected exported keys in generate result")
	}
	if gen.PrivateKeyPath != privPath {
		t.Fatalf("unexpected private key path: %q", gen.PrivateKeyPath)
	}
	if gen.PublicKeyPath != pubPath {
		t.Fatalf("unexpected public key path: %q", gen.PublicKeyPath)
	}
	if _, err := os.Stat(privPath); err != nil {
		t.Fatalf("expected generated private key file: %v", err)
	}
	if _, err := os.Stat(pubPath); err != nil {
		t.Fatalf("expected generated public key file: %v", err)
	}

	var enc1, enc2 encryptResult
	if err := json.Unmarshal(decoded.Steps[1].Result, &enc1); err != nil {
		t.Fatalf("Unmarshal encrypt1 result: %v", err)
	}
	if err := json.Unmarshal(decoded.Steps[2].Result, &enc2); err != nil {
		t.Fatalf("Unmarshal encrypt2 result: %v", err)
	}
	if !enc1.Armored || !enc2.Armored {
		t.Fatalf("expected armored ciphertexts")
	}
	if !enc1.Symmetric || !enc2.Symmetric {
		t.Fatalf("expected symmetric encryption flag")
	}
	if len(enc1.Recipients) != 0 || len(enc2.Recipients) != 0 {
		t.Fatalf("symmetric encryption should not list recipients")
	}
	if enc1.Ciphertext.Value == "" || enc2.Ciphertext.Value == "" {
		t.Fatalf("ciphertexts must not be empty")
	}
	if _, err := os.Stat(cipherOne); err != nil {
		t.Fatalf("expected ciphertext file 1: %v", err)
	}
	if _, err := os.Stat(cipherTwo); err != nil {
		t.Fatalf("expected ciphertext file 2: %v", err)
	}

	var dec1, dec2 decryptResult
	if err := json.Unmarshal(decoded.Steps[3].Result, &dec1); err != nil {
		t.Fatalf("Unmarshal decrypt1 result: %v", err)
	}
	if err := json.Unmarshal(decoded.Steps[4].Result, &dec2); err != nil {
		t.Fatalf("Unmarshal decrypt2 result: %v", err)
	}
	if !dec1.UsedPassword || !dec2.UsedPassword {
		t.Fatalf("expected decrypt steps to report used password")
	}
	if dec1.Plaintext.Value != contentOne {
		t.Fatalf("unexpected decrypt1 plaintext: %q", dec1.Plaintext.Value)
	}
	if dec2.Plaintext.Value != contentTwo {
		t.Fatalf("unexpected decrypt2 plaintext: %q", dec2.Plaintext.Value)
	}
	if dec2.OutputPath != plainTwo {
		t.Fatalf("unexpected decrypt2 output path: %q", dec2.OutputPath)
	}
	recoveredTwo, err := os.ReadFile(plainTwo)
	if err != nil {
		t.Fatalf("Read decrypt2 output: %v", err)
	}
	if string(recoveredTwo) != contentTwo {
		t.Fatalf("decrypt2 file content mismatch: %q", string(recoveredTwo))
	}
}

func exportEntity(t *testing.T, entity *openpgp.Entity) (string, string) {
	t.Helper()

	var privBuf bytes.Buffer
	privWriter, err := armor.Encode(&privBuf, openpgp.PrivateKeyType, nil)
	if err != nil {
		t.Fatalf("armor.Encode private: %v", err)
	}
	if err := entity.SerializePrivate(privWriter, nil); err != nil {
		t.Fatalf("SerializePrivate: %v", err)
	}
	if err := privWriter.Close(); err != nil {
		t.Fatalf("close private armor: %v", err)
	}

	var pubBuf bytes.Buffer
	pubWriter, err := armor.Encode(&pubBuf, openpgp.PublicKeyType, nil)
	if err != nil {
		t.Fatalf("armor.Encode public: %v", err)
	}
	if err := entity.Serialize(pubWriter); err != nil {
		t.Fatalf("Serialize public: %v", err)
	}
	if err := pubWriter.Close(); err != nil {
		t.Fatalf("close public armor: %v", err)
	}

	return privBuf.String(), pubBuf.String()
}
