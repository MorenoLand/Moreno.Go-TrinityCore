package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math/big"
	"strings"

	"github.com/MorenoLand/Moreno.Go-MorenoCore/engine/crypto"
)

func (s *Server) initializeTOTPSecrets(ctx context.Context, current, previous string) error {
	current = strings.TrimSpace(current)
	previous = strings.TrimSpace(previous)
	currentKey, err := parseTOTPMasterSecret(current)
	if err != nil {
		return err
	}
	previousKey, err := parseTOTPMasterSecret(previous)
	if err != nil {
		return err
	}
	if current == "" && previous == "" && (s.Store == nil || s.Store.DB == nil) {
		return nil
	}
	if s.Store == nil || s.Store.DB == nil {
		if previous != "" {
			return errors.New("auth database is required to transition TOTP master secret")
		}
		if current != "" {
			s.totpMasterKey, s.hasTotpMasterKey = currentKey, true
		}
		return nil
	}
	var digest string
	err = s.Store.DB.QueryRowContext(ctx, "SELECT digest FROM secret_digest WHERE id = ?", 0).Scan(&digest)
	hasDigest := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if hasDigest && current != "" && crypto.VerifyArgon2id(canonicalTOTPMasterSecret(currentKey), digest) {
		s.totpMasterKey, s.hasTotpMasterKey = currentKey, true
		return nil
	}
	if !hasDigest && current == "" {
		return nil
	}
	var oldKey *[crypto.AESKeySize]byte
	if hasDigest {
		if previous == "" || !crypto.VerifyArgon2id(canonicalTOTPMasterSecret(previousKey), digest) {
			return errors.New("TOTPMasterSecret does not match stored TOTP secret digest; provide the matching TOTPOldMasterSecret")
		}
		oldKey = &previousKey
	}
	newDigest := ""
	if current != "" {
		newDigest, err = crypto.HashArgon2id(canonicalTOTPMasterSecret(currentKey), nil, 0, 0)
		if err != nil {
			return err
		}
	}
	tx, err := s.Store.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, "SELECT id, totp_secret FROM account WHERE totp_secret IS NOT NULL")
	if err != nil {
		return err
	}
	type totpRow struct {
		id     uint64
		secret []byte
	}
	secrets := make([]totpRow, 0)
	for rows.Next() {
		var row totpRow
		if err := rows.Scan(&row.id, &row.secret); err != nil {
			rows.Close()
			return err
		}
		secrets = append(secrets, row)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, row := range secrets {
		secret := append([]byte(nil), row.secret...)
		if oldKey != nil {
			if err := crypto.DecryptWithTrailingIVAndTag(&secret, *oldKey); err != nil {
				return fmt.Errorf("cannot decrypt stored TOTP secret for account %d: %w", row.id, err)
			}
		}
		if current != "" {
			if err := crypto.EncryptWithRandomIV(&secret, currentKey); err != nil {
				return fmt.Errorf("cannot encrypt TOTP secret for account %d: %w", row.id, err)
			}
		}
		if _, err := tx.ExecContext(ctx, "UPDATE account SET totp_secret = ? WHERE id = ?", secret, row.id); err != nil {
			return err
		}
	}
	if hasDigest {
		if _, err := tx.ExecContext(ctx, "DELETE FROM secret_digest WHERE id = ?", 0); err != nil {
			return err
		}
	}
	if current != "" {
		if _, err := tx.ExecContext(ctx, "INSERT INTO secret_digest (id, digest) VALUES (?, ?)", 0, newDigest); err != nil {
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	if current != "" {
		s.totpMasterKey, s.hasTotpMasterKey = currentKey, true
	}
	return nil
}

func canonicalTOTPMasterSecret(key [crypto.AESKeySize]byte) string {
	return strings.ToUpper(new(big.Int).SetBytes(key[:]).Text(16))
}
