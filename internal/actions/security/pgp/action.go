package pgp

import (
	"bytes"
	"context"
	"crypto"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/crypto/openpgp"
	"golang.org/x/crypto/openpgp/armor"
	"golang.org/x/crypto/openpgp/packet"
	_ "golang.org/x/crypto/ripemd160"

	"flowk/internal/actions/registry"
	"flowk/internal/flow"
)

func init() {
	registry.Register(&Action{})
}

// Action implements OpenPGP utilities for FlowK.
type Action struct{}

// Name returns the registry identifier for the PGP action.
func (Action) Name() string {
	return "PGP"
}

// Execute runs the requested sequence of PGP operations.
func (Action) Execute(ctx context.Context, payload json.RawMessage, execCtx *registry.ExecutionContext) (registry.Result, error) {
	var spec payloadSpec
	if err := json.Unmarshal(payload, &spec); err != nil {
		return registry.Result{}, fmt.Errorf("pgp: decode payload: %w", err)
	}
	if err := spec.validate(); err != nil {
		return registry.Result{}, err
	}

	state := newActionState(execCtx)
	outcomes := make([]stepOutcome, 0, len(spec.Steps))

	for idx, raw := range spec.Steps {
		select {
		case <-ctx.Done():
			return registry.Result{}, ctx.Err()
		default:
		}

		outcome, err := state.executeStep(ctx, idx, raw)
		if err != nil {
			return registry.Result{}, err
		}
		outcomes = append(outcomes, outcome)
	}

	return registry.Result{Value: map[string]any{
		"steps": outcomes,
	}, Type: flow.ResultTypeJSON}, nil
}

type payloadSpec struct {
	Steps []json.RawMessage `json:"steps"`
}

func (p *payloadSpec) validate() error {
	if len(p.Steps) == 0 {
		return errors.New("pgp: at least one step must be declared")
	}
	return nil
}

type baseStep struct {
	ID        string `json:"id"`
	Operation string `json:"operation"`
}

type stepOutcome struct {
	ID        string `json:"id,omitempty"`
	Operation string `json:"operation"`
	Result    any    `json:"result,omitempty"`
}

type actionState struct {
	aliases     map[string]*keyEntry
	entityAlias map[*openpgp.Entity]string
	execCtx     *registry.ExecutionContext
}

type keyEntry struct {
	Alias           string
	Entities        openpgp.EntityList
	Armored         bool
	Source          string
	ContainsPrivate bool
	Fingerprints    []string
}

type encodedData struct {
	Value    string `json:"value,omitempty"`
	Encoding string `json:"encoding,omitempty"`
}

type importKeyResult struct {
	Alias        string            `json:"alias"`
	Armored      bool              `json:"armored"`
	Source       string            `json:"source"`
	EntityCount  int               `json:"entityCount"`
	Private      bool              `json:"containsPrivate"`
	Fingerprints []string          `json:"fingerprints"`
	Identities   map[string]string `json:"identities,omitempty"`
}

type encryptResult struct {
	Ciphertext        encodedData        `json:"ciphertext,omitempty"`
	OutputPath        string             `json:"outputPath,omitempty"`
	Armored           bool               `json:"armored"`
	Recipients        []recipientSummary `json:"recipients"`
	SignerAlias       string             `json:"signerAlias,omitempty"`
	SignerFingerprint string             `json:"signerFingerprint,omitempty"`
	SignerKeyID       string             `json:"signerKeyId,omitempty"`
	Symmetric         bool               `json:"symmetric"`
}

type recipientSummary struct {
	Alias        string `json:"alias"`
	Fingerprint  string `json:"fingerprint"`
	PrimaryKeyID string `json:"primaryKeyId"`
}

type decryptResult struct {
	Plaintext          encodedData `json:"plaintext,omitempty"`
	OutputPath         string      `json:"outputPath,omitempty"`
	Verified           bool        `json:"verified"`
	SignerAlias        string      `json:"signerAlias,omitempty"`
	SignerFingerprint  string      `json:"signerFingerprint,omitempty"`
	SignerKeyID        string      `json:"signerKeyId,omitempty"`
	LiteralFileName    string      `json:"literalFileName,omitempty"`
	LiteralIsBinary    bool        `json:"literalIsBinary,omitempty"`
	LiteralModUnixTime int64       `json:"literalModUnixTime,omitempty"`
	UsedPassword       bool        `json:"usedPassword"`
}

type signResult struct {
	Signature         encodedData `json:"signature,omitempty"`
	OutputPath        string      `json:"outputPath,omitempty"`
	Armored           bool        `json:"armored"`
	SignerAlias       string      `json:"signerAlias"`
	SignerFingerprint string      `json:"signerFingerprint"`
	SignerKeyID       string      `json:"signerKeyId"`
}

type verifyResult struct {
	Verified          bool   `json:"verified"`
	SignerAlias       string `json:"signerAlias,omitempty"`
	SignerFingerprint string `json:"signerFingerprint,omitempty"`
	SignerKeyID       string `json:"signerKeyId,omitempty"`
}

type importKeyStep struct {
	baseStep
	Alias      string `json:"alias"`
	Key        string `json:"key"`
	KeyPath    string `json:"keyPath"`
	Passphrase string `json:"passphrase"`
}

type encryptStep struct {
	baseStep
	Recipients     []string `json:"recipients"`
	Message        string   `json:"message"`
	MessagePath    string   `json:"messagePath"`
	Armor          *bool    `json:"armor"`
	OutputPath     string   `json:"outputPath"`
	SignWith       string   `json:"signWith"`
	FileName       string   `json:"fileName"`
	Binary         *bool    `json:"binary"`
	ResultEncoding string   `json:"resultEncoding"`
	Password       string   `json:"password"`
}

type decryptStep struct {
	baseStep
	Message          string   `json:"message"`
	MessagePath      string   `json:"messagePath"`
	KeyAliases       []string `json:"keyAliases"`
	RequireSignature bool     `json:"requireSignature"`
	AllowedSigners   []string `json:"allowedSigners"`
	OutputPath       string   `json:"outputPath"`
	OutputEncoding   string   `json:"outputEncoding"`
	Passwords        []string `json:"passwords"`
}

type generateKeyStep struct {
	baseStep
	Alias          string `json:"alias"`
	Name           string `json:"name"`
	Email          string `json:"email"`
	Comment        string `json:"comment"`
	RSABits        int    `json:"rsaBits"`
	PrivateKeyPath string `json:"privateKeyPath"`
	PublicKeyPath  string `json:"publicKeyPath"`
}

type generateKeyResult struct {
	Alias          string            `json:"alias"`
	Armored        bool              `json:"armored"`
	Fingerprint    string            `json:"fingerprint"`
	KeyID          string            `json:"keyId"`
	PrivateKey     encodedData       `json:"privateKey"`
	PublicKey      encodedData       `json:"publicKey"`
	PrivateKeyPath string            `json:"privateKeyPath,omitempty"`
	PublicKeyPath  string            `json:"publicKeyPath,omitempty"`
	Identities     map[string]string `json:"identities,omitempty"`
}

type signDetachedStep struct {
	baseStep
	SignWith       string `json:"signWith"`
	Message        string `json:"message"`
	MessagePath    string `json:"messagePath"`
	Armor          *bool  `json:"armor"`
	OutputPath     string `json:"outputPath"`
	TextMode       *bool  `json:"textMode"`
	ResultEncoding string `json:"resultEncoding"`
}

type verifyDetachedStep struct {
	baseStep
	Message       string   `json:"message"`
	MessagePath   string   `json:"messagePath"`
	Signature     string   `json:"signature"`
	SignaturePath string   `json:"signaturePath"`
	KeyAliases    []string `json:"keyAliases"`
}

func newActionState(execCtx *registry.ExecutionContext) *actionState {
	return &actionState{
		aliases:     make(map[string]*keyEntry),
		entityAlias: make(map[*openpgp.Entity]string),
		execCtx:     execCtx,
	}
}

var defaultConfig = &packet.Config{DefaultHash: crypto.SHA256}

func (s *actionState) executeStep(ctx context.Context, idx int, raw json.RawMessage) (stepOutcome, error) {
	var base baseStep
	if err := json.Unmarshal(raw, &base); err != nil {
		return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: decode: %w", idx, err)
	}

	op := strings.ToUpper(strings.TrimSpace(base.Operation))
	if op == "" {
		return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: operation is required", idx)
	}

	var (
		result any
		err    error
	)

	switch op {
	case "GENERATE_KEY":
		var step generateKeyStep
		if err := json.Unmarshal(raw, &step); err != nil {
			return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: decode generate_key: %w", idx, err)
		}
		result, err = s.executeGenerateKey(step)
	case "IMPORT_KEY":
		var step importKeyStep
		if err := json.Unmarshal(raw, &step); err != nil {
			return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: decode import_key: %w", idx, err)
		}
		result, err = s.executeImportKey(step)
	case "ENCRYPT":
		var step encryptStep
		if err := json.Unmarshal(raw, &step); err != nil {
			return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: decode encrypt: %w", idx, err)
		}
		result, err = s.executeEncrypt(step)
	case "DECRYPT":
		var step decryptStep
		if err := json.Unmarshal(raw, &step); err != nil {
			return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: decode decrypt: %w", idx, err)
		}
		result, err = s.executeDecrypt(step)
	case "SIGN_DETACHED":
		var step signDetachedStep
		if err := json.Unmarshal(raw, &step); err != nil {
			return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: decode sign_detached: %w", idx, err)
		}
		result, err = s.executeSignDetached(step)
	case "VERIFY_DETACHED":
		var step verifyDetachedStep
		if err := json.Unmarshal(raw, &step); err != nil {
			return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: decode verify_detached: %w", idx, err)
		}
		result, err = s.executeVerifyDetached(step)
	default:
		return stepOutcome{}, fmt.Errorf("pgp: steps[%d]: unsupported operation %q", idx, base.Operation)
	}
	if err != nil {
		return stepOutcome{}, err
	}

	return stepOutcome{ID: strings.TrimSpace(base.ID), Operation: op, Result: result}, nil
}

func (s *actionState) executeGenerateKey(step generateKeyStep) (generateKeyResult, error) {
	alias := strings.TrimSpace(step.Alias)
	if alias == "" {
		return generateKeyResult{}, errors.New("pgp: generate_key.alias must be provided")
	}
	if _, exists := s.aliases[strings.ToUpper(alias)]; exists {
		return generateKeyResult{}, fmt.Errorf("pgp: alias %q already defined", alias)
	}

	name := strings.TrimSpace(step.Name)
	email := strings.TrimSpace(step.Email)
	comment := strings.TrimSpace(step.Comment)
	if name == "" && email == "" {
		return generateKeyResult{}, errors.New("pgp: generate_key requires at least a name or email")
	}

	rsaBits := step.RSABits
	if rsaBits < 0 {
		return generateKeyResult{}, fmt.Errorf("pgp: generate_key[%s]: rsaBits cannot be negative", alias)
	}
	if rsaBits > 0 && rsaBits < 2048 {
		return generateKeyResult{}, fmt.Errorf("pgp: generate_key[%s]: rsaBits must be at least 2048", alias)
	}

	cfg := *defaultConfig
	if rsaBits > 0 {
		cfg.RSABits = rsaBits
	}

	entity, err := openpgp.NewEntity(name, comment, email, &cfg)
	if err != nil {
		return generateKeyResult{}, fmt.Errorf("pgp: generate_key[%s]: create entity: %w", alias, err)
	}

	privData, err := exportPrivateKey(entity)
	if err != nil {
		return generateKeyResult{}, fmt.Errorf("pgp: generate_key[%s]: %w", alias, err)
	}
	pubData, err := exportPublicKey(entity)
	if err != nil {
		return generateKeyResult{}, fmt.Errorf("pgp: generate_key[%s]: %w", alias, err)
	}

	entry := &keyEntry{
		Alias:           alias,
		Entities:        openpgp.EntityList{entity},
		Armored:         true,
		Source:          "generated",
		ContainsPrivate: true,
		Fingerprints:    []string{fingerprintHex(entity.PrimaryKey)},
	}

	identities := make(map[string]string)
	for name, ident := range entity.Identities {
		if name == "" {
			continue
		}
		identities[name] = ident.UserId.Email
	}

	s.aliases[strings.ToUpper(alias)] = entry
	s.entityAlias[entity] = alias
	s.logf("PGP: generated key for alias %s", alias)

	privPath := strings.TrimSpace(step.PrivateKeyPath)
	if privPath != "" {
		if err := writeFile(privPath, privData); err != nil {
			return generateKeyResult{}, fmt.Errorf("pgp: generate_key[%s]: %w", alias, err)
		}
	}

	pubPath := strings.TrimSpace(step.PublicKeyPath)
	if pubPath != "" {
		if err := writeFile(pubPath, pubData); err != nil {
			return generateKeyResult{}, fmt.Errorf("pgp: generate_key[%s]: %w", alias, err)
		}
	}

	return generateKeyResult{
		Alias:       alias,
		Armored:     true,
		Fingerprint: fingerprintHex(entity.PrimaryKey),
		KeyID:       formatKeyID(entity.PrimaryKey.KeyId),
		PrivateKey: encodedData{
			Value:    string(privData),
			Encoding: "utf8",
		},
		PublicKey: encodedData{
			Value:    string(pubData),
			Encoding: "utf8",
		},
		PrivateKeyPath: privPath,
		PublicKeyPath:  pubPath,
		Identities:     identities,
	}, nil
}

func (s *actionState) executeImportKey(step importKeyStep) (importKeyResult, error) {
	alias := strings.TrimSpace(step.Alias)
	if alias == "" {
		return importKeyResult{}, errors.New("pgp: import_key.alias must be provided")
	}
	if _, exists := s.aliases[strings.ToUpper(alias)]; exists {
		return importKeyResult{}, fmt.Errorf("pgp: alias %q already defined", alias)
	}

	keyData, source, err := loadBytes(step.Key, step.KeyPath)
	if err != nil {
		return importKeyResult{}, fmt.Errorf("pgp: import_key[%s]: %w", alias, err)
	}

	entities, armored, err := parseKeyRing(keyData)
	if err != nil {
		return importKeyResult{}, fmt.Errorf("pgp: import_key[%s]: %w", alias, err)
	}
	if len(entities) == 0 {
		return importKeyResult{}, fmt.Errorf("pgp: import_key[%s]: no keys found", alias)
	}

	passphrase := []byte(step.Passphrase)
	containsPrivate := false
	for _, entity := range entities {
		if entity.PrivateKey != nil {
			if entity.PrivateKey.Encrypted {
				if len(passphrase) == 0 {
					return importKeyResult{}, fmt.Errorf("pgp: import_key[%s]: private key is encrypted but no passphrase supplied", alias)
				}
				if err := entity.PrivateKey.Decrypt(passphrase); err != nil {
					return importKeyResult{}, fmt.Errorf("pgp: import_key[%s]: decrypt private key: %w", alias, err)
				}
			}
			containsPrivate = true
		}
		for _, sub := range entity.Subkeys {
			if sub.PrivateKey != nil {
				if sub.PrivateKey.Encrypted {
					if len(passphrase) == 0 {
						return importKeyResult{}, fmt.Errorf("pgp: import_key[%s]: subkey is encrypted but no passphrase supplied", alias)
					}
					if err := sub.PrivateKey.Decrypt(passphrase); err != nil {
						return importKeyResult{}, fmt.Errorf("pgp: import_key[%s]: decrypt subkey: %w", alias, err)
					}
				}
				containsPrivate = true
			}
		}
	}

	entry := &keyEntry{
		Alias:           alias,
		Entities:        entities,
		Armored:         armored,
		Source:          source,
		ContainsPrivate: containsPrivate,
		Fingerprints:    make([]string, 0, len(entities)),
	}

	identities := make(map[string]string)
	for _, entity := range entities {
		fp := fingerprintHex(entity.PrimaryKey)
		entry.Fingerprints = append(entry.Fingerprints, fp)
		for name, ident := range entity.Identities {
			if name == "" {
				continue
			}
			identities[name] = ident.UserId.Email
		}
		s.entityAlias[entity] = alias
	}

	key := strings.ToUpper(alias)
	s.aliases[key] = entry
	s.logf("PGP: imported %d key entity(ies) into alias %s", len(entities), alias)

	return importKeyResult{
		Alias:        alias,
		Armored:      armored,
		Source:       source,
		EntityCount:  len(entities),
		Private:      containsPrivate,
		Fingerprints: entry.Fingerprints,
		Identities:   identities,
	}, nil
}

func (s *actionState) executeEncrypt(step encryptStep) (encryptResult, error) {
	message, _, err := loadBytes(step.Message, step.MessagePath)
	if err != nil {
		return encryptResult{}, fmt.Errorf("pgp: encrypt: %w", err)
	}

	password := strings.TrimSpace(step.Password)

	var (
		recipients openpgp.EntityList
		summaries  []recipientSummary
	)
	if len(step.Recipients) > 0 {
		recipients, summaries, err = s.entitiesForAliases(step.Recipients)
		if err != nil {
			return encryptResult{}, fmt.Errorf("pgp: encrypt: %w", err)
		}
	}

	if len(recipients) == 0 && password == "" {
		return encryptResult{}, errors.New("pgp: encrypt: recipients or password must be provided")
	}
	if len(recipients) > 0 && password != "" {
		return encryptResult{}, errors.New("pgp: encrypt: recipients and password are mutually exclusive")
	}

	armorEnabled := true
	if step.Armor != nil {
		armorEnabled = *step.Armor
	}

	signerAlias := strings.TrimSpace(step.SignWith)
	var signer *openpgp.Entity
	if password != "" && signerAlias != "" {
		return encryptResult{}, errors.New("pgp: encrypt: cannot sign when using password encryption")
	}
	if signerAlias != "" {
		entry, err := s.lookupAlias(signerAlias)
		if err != nil {
			return encryptResult{}, fmt.Errorf("pgp: encrypt: %w", err)
		}
		signer, err = selectSigningEntity(entry)
		if err != nil {
			return encryptResult{}, fmt.Errorf("pgp: encrypt: %w", err)
		}
	}

	var buf bytes.Buffer
	writer := io.Writer(&buf)
	var closer io.Closer
	if armorEnabled {
		aw, err := armor.Encode(&buf, "PGP MESSAGE", nil)
		if err != nil {
			return encryptResult{}, fmt.Errorf("pgp: encrypt: armor: %w", err)
		}
		writer = aw
		closer = aw
	}

	hints := &openpgp.FileHints{IsBinary: step.Binary != nil && *step.Binary, FileName: strings.TrimSpace(step.FileName)}

	var plaintext io.WriteCloser
	if password != "" {
		plaintext, err = openpgp.SymmetricallyEncrypt(writer, []byte(password), hints, defaultConfig)
	} else {
		plaintext, err = openpgp.Encrypt(writer, recipients, signer, hints, defaultConfig)
	}
	if err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return encryptResult{}, fmt.Errorf("pgp: encrypt: %w", err)
	}

	if _, err := plaintext.Write(message); err != nil {
		plaintext.Close()
		if closer != nil {
			_ = closer.Close()
		}
		return encryptResult{}, fmt.Errorf("pgp: encrypt: write message: %w", err)
	}

	if err := plaintext.Close(); err != nil {
		if closer != nil {
			_ = closer.Close()
		}
		return encryptResult{}, fmt.Errorf("pgp: encrypt: finalise message: %w", err)
	}

	if closer != nil {
		if err := closer.Close(); err != nil {
			return encryptResult{}, fmt.Errorf("pgp: encrypt: close armor: %w", err)
		}
	}

	ciphertext := buf.Bytes()
	resultData := encodedData{}
	if armorEnabled {
		resultData.Value = string(ciphertext)
		resultData.Encoding = "utf8"
	} else {
		resultData.Value = base64.StdEncoding.EncodeToString(ciphertext)
		resultData.Encoding = "base64"
	}

	if step.OutputPath != "" {
		if err := writeFile(step.OutputPath, ciphertext); err != nil {
			return encryptResult{}, fmt.Errorf("pgp: encrypt: %w", err)
		}
	}

	res := encryptResult{
		Ciphertext: resultData,
		OutputPath: strings.TrimSpace(step.OutputPath),
		Armored:    armorEnabled,
		Recipients: summaries,
		Symmetric:  password != "",
	}

	if signer != nil {
		res.SignerAlias, _ = s.aliasForEntity(signer)
		res.SignerFingerprint = fingerprintHex(signer.PrimaryKey)
		res.SignerKeyID = formatKeyID(signer.PrimaryKey.KeyId)
	}

	return res, nil
}

func (s *actionState) executeDecrypt(step decryptStep) (decryptResult, error) {
	ciphertext, _, err := loadBytes(step.Message, step.MessagePath)
	if err != nil {
		return decryptResult{}, fmt.Errorf("pgp: decrypt: %w", err)
	}

	decoded, armored, err := maybeDecodeArmor(ciphertext, "PGP MESSAGE")
	if err != nil {
		return decryptResult{}, fmt.Errorf("pgp: decrypt: %w", err)
	}
	if armored {
		ciphertext = decoded
	}

	rawPasswords := make([][]byte, 0, len(step.Passwords))
	for _, pwd := range step.Passwords {
		trimmed := strings.TrimSpace(pwd)
		if trimmed == "" {
			continue
		}
		rawPasswords = append(rawPasswords, []byte(trimmed))
	}

	var keyRing openpgp.EntityList
	keyRing, err = s.keyRingForDecrypt(step.KeyAliases)
	if err != nil {
		return decryptResult{}, fmt.Errorf("pgp: decrypt: %w", err)
	}

	if len(keyRing) == 0 && len(rawPasswords) == 0 {
		return decryptResult{}, errors.New("pgp: decrypt: no private keys or passwords available")
	}

	var passIdx int
	var prompt openpgp.PromptFunction
	if len(rawPasswords) > 0 {
		prompt = func(keys []openpgp.Key, symmetric bool) ([]byte, error) {
			if !symmetric {
				return nil, nil
			}
			if passIdx >= len(rawPasswords) {
				return nil, errors.New("pgp: decrypt: provided passwords did not match message")
			}
			pass := rawPasswords[passIdx]
			passIdx++
			return pass, nil
		}
	}

	md, err := openpgp.ReadMessage(bytes.NewReader(ciphertext), keyRing, prompt, nil)
	if err != nil {
		return decryptResult{}, fmt.Errorf("pgp: decrypt: %w", err)
	}

	plaintext, err := io.ReadAll(md.UnverifiedBody)
	if err != nil {
		return decryptResult{}, fmt.Errorf("pgp: decrypt: read body: %w", err)
	}
	if md.SignatureError != nil {
		return decryptResult{}, fmt.Errorf("pgp: decrypt: signature verification failed: %w", md.SignatureError)
	}

	verified := md.IsSigned && md.SignedBy != nil
	signerAlias := ""
	signerFingerprint := ""
	signerKeyID := ""
	if verified {
		signerAlias, _ = s.aliasForEntity(md.SignedBy.Entity)
		signerFingerprint = fingerprintHex(md.SignedBy.PublicKey)
		signerKeyID = formatKeyID(md.SignedByKeyId)
	}

	if step.RequireSignature && !verified {
		return decryptResult{}, errors.New("pgp: decrypt: signature required but not present")
	}

	if len(step.AllowedSigners) > 0 {
		if !verified {
			return decryptResult{}, errors.New("pgp: decrypt: allowedSigners specified but message is not signed")
		}
		allowed := make(map[string]struct{}, len(step.AllowedSigners))
		for _, alias := range step.AllowedSigners {
			allowed[strings.ToUpper(strings.TrimSpace(alias))] = struct{}{}
		}
		if _, ok := allowed[strings.ToUpper(signerAlias)]; !ok {
			return decryptResult{}, fmt.Errorf("pgp: decrypt: signer %q not permitted", signerAlias)
		}
	}

	encoding := strings.ToLower(strings.TrimSpace(step.OutputEncoding))
	if encoding == "" {
		encoding = "utf8"
	}

	data := encodedData{}
	switch encoding {
	case "utf8", "string":
		data.Value = string(plaintext)
		data.Encoding = "utf8"
	case "base64":
		data.Value = base64.StdEncoding.EncodeToString(plaintext)
		data.Encoding = "base64"
	default:
		return decryptResult{}, fmt.Errorf("pgp: decrypt: unsupported outputEncoding %q", step.OutputEncoding)
	}

	if step.OutputPath != "" {
		if err := writeFile(step.OutputPath, plaintext); err != nil {
			return decryptResult{}, fmt.Errorf("pgp: decrypt: %w", err)
		}
	}

	var modTime int64
	var fileName string
	var isBinary bool
	if md.LiteralData != nil {
		fileName = md.LiteralData.FileName
		isBinary = md.LiteralData.IsBinary
		if md.LiteralData.Time > 0 {
			modTime = int64(md.LiteralData.Time)
		}
	}

	usedPassword := md.IsSymmetricallyEncrypted && md.DecryptedWith.Entity == nil

	return decryptResult{
		Plaintext:          data,
		OutputPath:         strings.TrimSpace(step.OutputPath),
		Verified:           verified,
		SignerAlias:        signerAlias,
		SignerFingerprint:  signerFingerprint,
		SignerKeyID:        signerKeyID,
		LiteralFileName:    fileName,
		LiteralIsBinary:    isBinary,
		LiteralModUnixTime: modTime,
		UsedPassword:       usedPassword,
	}, nil
}

func (s *actionState) executeSignDetached(step signDetachedStep) (signResult, error) {
	signerAlias := strings.TrimSpace(step.SignWith)
	if signerAlias == "" {
		return signResult{}, errors.New("pgp: sign_detached.signWith must be provided")
	}
	entry, err := s.lookupAlias(signerAlias)
	if err != nil {
		return signResult{}, fmt.Errorf("pgp: sign_detached: %w", err)
	}
	signer, err := selectSigningEntity(entry)
	if err != nil {
		return signResult{}, fmt.Errorf("pgp: sign_detached: %w", err)
	}

	message, _, err := loadBytes(step.Message, step.MessagePath)
	if err != nil {
		return signResult{}, fmt.Errorf("pgp: sign_detached: %w", err)
	}

	armorEnabled := true
	if step.Armor != nil {
		armorEnabled = *step.Armor
	}

	textMode := false
	if step.TextMode != nil {
		textMode = *step.TextMode
	}

	var buf bytes.Buffer
	var signErr error
	if armorEnabled {
		if textMode {
			signErr = openpgp.ArmoredDetachSignText(&buf, signer, bytes.NewReader(message), defaultConfig)
		} else {
			signErr = openpgp.ArmoredDetachSign(&buf, signer, bytes.NewReader(message), defaultConfig)
		}
	} else {
		if textMode {
			signErr = openpgp.DetachSignText(&buf, signer, bytes.NewReader(message), defaultConfig)
		} else {
			signErr = openpgp.DetachSign(&buf, signer, bytes.NewReader(message), defaultConfig)
		}
	}
	if signErr != nil {
		return signResult{}, fmt.Errorf("pgp: sign_detached: %w", signErr)
	}

	signature := buf.Bytes()
	data := encodedData{}
	if armorEnabled {
		data.Value = string(signature)
		data.Encoding = "utf8"
	} else {
		encoding := strings.ToLower(strings.TrimSpace(step.ResultEncoding))
		switch encoding {
		case "", "base64":
			data.Value = base64.StdEncoding.EncodeToString(signature)
			data.Encoding = "base64"
		case "binary":
			data.Value = string(signature)
			data.Encoding = "binary"
		default:
			return signResult{}, fmt.Errorf("pgp: sign_detached: unsupported resultEncoding %q", step.ResultEncoding)
		}
	}

	if step.OutputPath != "" {
		if err := writeFile(step.OutputPath, signature); err != nil {
			return signResult{}, fmt.Errorf("pgp: sign_detached: %w", err)
		}
	}

	alias, _ := s.aliasForEntity(signer)
	return signResult{
		Signature:         data,
		OutputPath:        strings.TrimSpace(step.OutputPath),
		Armored:           armorEnabled,
		SignerAlias:       alias,
		SignerFingerprint: fingerprintHex(signer.PrimaryKey),
		SignerKeyID:       formatKeyID(signer.PrimaryKey.KeyId),
	}, nil
}

func (s *actionState) executeVerifyDetached(step verifyDetachedStep) (verifyResult, error) {
	if len(step.KeyAliases) == 0 {
		return verifyResult{}, errors.New("pgp: verify_detached.keyAliases must list at least one alias")
	}
	message, _, err := loadBytes(step.Message, step.MessagePath)
	if err != nil {
		return verifyResult{}, fmt.Errorf("pgp: verify_detached: %w", err)
	}
	signature, _, err := loadBytes(step.Signature, step.SignaturePath)
	if err != nil {
		return verifyResult{}, fmt.Errorf("pgp: verify_detached: %w", err)
	}

	keyRing, _, err := s.entitiesForAliases(step.KeyAliases)
	if err != nil {
		return verifyResult{}, fmt.Errorf("pgp: verify_detached: %w", err)
	}
	if len(keyRing) == 0 {
		return verifyResult{}, errors.New("pgp: verify_detached: no keys available")
	}

	var entity *openpgp.Entity
	entity, err = openpgp.CheckDetachedSignature(keyRing, bytes.NewReader(message), bytes.NewReader(signature))
	if err != nil {
		block, blockErr := armor.Decode(bytes.NewReader(signature))
		if blockErr != nil {
			return verifyResult{}, fmt.Errorf("pgp: verify_detached: %w", err)
		}
		if block.Type != openpgp.SignatureType {
			return verifyResult{}, fmt.Errorf("pgp: verify_detached: expected %s block, got %s", openpgp.SignatureType, block.Type)
		}
		body, readErr := io.ReadAll(block.Body)
		if readErr != nil {
			return verifyResult{}, fmt.Errorf("pgp: verify_detached: read armored signature: %w", readErr)
		}
		entity, err = openpgp.CheckDetachedSignature(keyRing, bytes.NewReader(message), bytes.NewReader(body))
		if err != nil {
			return verifyResult{}, fmt.Errorf("pgp: verify_detached: %w", err)
		}
	}

	alias, _ := s.aliasForEntity(entity)
	return verifyResult{
		Verified:          true,
		SignerAlias:       alias,
		SignerFingerprint: fingerprintHex(entity.PrimaryKey),
		SignerKeyID:       formatKeyID(entity.PrimaryKey.KeyId),
	}, nil
}

func (s *actionState) lookupAlias(alias string) (*keyEntry, error) {
	key := strings.ToUpper(strings.TrimSpace(alias))
	if key == "" {
		return nil, errors.New("pgp: alias must be provided")
	}
	entry, ok := s.aliases[key]
	if !ok {
		return nil, fmt.Errorf("pgp: unknown alias %q", alias)
	}
	return entry, nil
}

func (s *actionState) entitiesForAliases(aliases []string) (openpgp.EntityList, []recipientSummary, error) {
	entities := make(openpgp.EntityList, 0, len(aliases))
	summaries := make([]recipientSummary, 0, len(aliases))
	seen := make(map[*openpgp.Entity]struct{})
	for _, alias := range aliases {
		entry, err := s.lookupAlias(alias)
		if err != nil {
			return nil, nil, err
		}
		for _, entity := range entry.Entities {
			if _, ok := seen[entity]; ok {
				continue
			}
			seen[entity] = struct{}{}
			entities = append(entities, entity)
			summaries = append(summaries, recipientSummary{
				Alias:        entry.Alias,
				Fingerprint:  fingerprintHex(entity.PrimaryKey),
				PrimaryKeyID: formatKeyID(entity.PrimaryKey.KeyId),
			})
		}
	}
	return entities, summaries, nil
}

func (s *actionState) keyRingForDecrypt(aliases []string) (openpgp.EntityList, error) {
	var ring openpgp.EntityList
	if len(aliases) == 0 {
		ring = make(openpgp.EntityList, 0, len(s.aliases))
		for _, entry := range s.aliases {
			ring = append(ring, entry.Entities...)
		}
	} else {
		entities, _, err := s.entitiesForAliases(aliases)
		if err != nil {
			return nil, err
		}
		ring = entities
	}
	filtered := make(openpgp.EntityList, 0, len(ring))
	for _, entity := range ring {
		if entity.PrivateKey != nil && entity.PrivateKey.PrivateKey != nil {
			filtered = append(filtered, entity)
			continue
		}
		for _, sub := range entity.Subkeys {
			if sub.PrivateKey != nil && sub.PrivateKey.PrivateKey != nil {
				filtered = append(filtered, entity)
				break
			}
		}
	}
	return filtered, nil
}

func (s *actionState) aliasForEntity(entity *openpgp.Entity) (string, bool) {
	if entity == nil {
		return "", false
	}
	if alias, ok := s.entityAlias[entity]; ok {
		return alias, true
	}
	fp := fingerprintHex(entity.PrimaryKey)
	for _, entry := range s.aliases {
		for _, candidate := range entry.Entities {
			if fingerprintHex(candidate.PrimaryKey) == fp {
				return entry.Alias, true
			}
		}
	}
	return "", false
}

func selectSigningEntity(entry *keyEntry) (*openpgp.Entity, error) {
	for _, entity := range entry.Entities {
		if entity.PrivateKey == nil {
			continue
		}
		if entity.PrivateKey.PrivateKey != nil && !entity.PrivateKey.Encrypted {
			return entity, nil
		}
		if entity.PrivateKey.PrivateKey != nil && entity.PrivateKey.Encrypted {
			return nil, fmt.Errorf("pgp: alias %q contains encrypted private key; provide passphrase during import", entry.Alias)
		}
	}
	return nil, fmt.Errorf("pgp: alias %q does not contain a signing-capable private key", entry.Alias)
}

func loadBytes(inline, path string) ([]byte, string, error) {
	trimmedInline := inline
	trimmedPath := strings.TrimSpace(path)
	if trimmedInline == "" && trimmedPath == "" {
		return nil, "", errors.New("no data provided")
	}
	if trimmedInline != "" && trimmedPath != "" {
		return nil, "", errors.New("both inline value and path provided")
	}
	if trimmedPath != "" {
		data, err := os.ReadFile(trimmedPath)
		if err != nil {
			return nil, "", fmt.Errorf("read file %s: %w", trimmedPath, err)
		}
		return data, trimmedPath, nil
	}
	return []byte(inline), "inline", nil
}

func parseKeyRing(data []byte) (openpgp.EntityList, bool, error) {
	if block, err := armor.Decode(bytes.NewReader(data)); err == nil {
		switch block.Type {
		case openpgp.PublicKeyType, openpgp.PrivateKeyType:
			body, err := io.ReadAll(block.Body)
			if err != nil {
				return nil, true, fmt.Errorf("read armored key: %w", err)
			}
			entities, err := openpgp.ReadKeyRing(bytes.NewReader(body))
			if err != nil {
				return nil, true, fmt.Errorf("parse armored key: %w", err)
			}
			return entities, true, nil
		default:
			return nil, true, fmt.Errorf("unsupported armored block type %q", block.Type)
		}
	}
	entities, err := openpgp.ReadKeyRing(bytes.NewReader(data))
	if err != nil {
		return nil, false, fmt.Errorf("parse key: %w", err)
	}
	return entities, false, nil
}

func exportPrivateKey(entity *openpgp.Entity) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := armor.Encode(&buf, openpgp.PrivateKeyType, nil)
	if err != nil {
		return nil, fmt.Errorf("armor private key: %w", err)
	}
	if err := entity.SerializePrivate(writer, defaultConfig); err != nil {
		return nil, fmt.Errorf("serialize private key: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close private armor: %w", err)
	}
	return buf.Bytes(), nil
}

func exportPublicKey(entity *openpgp.Entity) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := armor.Encode(&buf, openpgp.PublicKeyType, nil)
	if err != nil {
		return nil, fmt.Errorf("armor public key: %w", err)
	}
	if err := entity.Serialize(writer); err != nil {
		return nil, fmt.Errorf("serialize public key: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("close public armor: %w", err)
	}
	return buf.Bytes(), nil
}

func maybeDecodeArmor(data []byte, expected string) ([]byte, bool, error) {
	block, err := armor.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, false, nil
	}
	if expected != "" && block.Type != expected {
		return nil, true, fmt.Errorf("unexpected armored block type %q", block.Type)
	}
	body, err := io.ReadAll(block.Body)
	if err != nil {
		return nil, true, fmt.Errorf("read armored data: %w", err)
	}
	return body, true, nil
}

func writeFile(path string, data []byte) error {
	cleaned := filepath.Clean(path)
	if dir := filepath.Dir(cleaned); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("create directories for %s: %w", cleaned, err)
		}
	}
	if err := os.WriteFile(cleaned, data, 0o600); err != nil {
		return fmt.Errorf("write file %s: %w", cleaned, err)
	}
	return nil
}

func fingerprintHex(key *packet.PublicKey) string {
	if key == nil {
		return ""
	}
	return strings.ToUpper(hex.EncodeToString(key.Fingerprint[:]))
}

func formatKeyID(id uint64) string {
	if id == 0 {
		return ""
	}
	return strings.ToUpper(fmt.Sprintf("%016x", id))
}

func (s *actionState) logf(format string, args ...any) {
	if s.execCtx != nil && s.execCtx.Logger != nil {
		s.execCtx.Logger.Printf(format, args...)
	}
}
