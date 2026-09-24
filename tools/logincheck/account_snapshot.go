package main

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strconv"
)

func snapshotAuthAccountOnline(db *sql.DB, accountID int64) (characterTableSnapshot, error) {
	snapshot := characterTableSnapshot{Columns: map[string]string{"online": ""}}
	if db == nil || accountID <= 0 {
		return snapshot, nil
	}
	var online int64
	err := db.QueryRow("SELECT online FROM account WHERE id = ?", accountID).Scan(&online)
	if err == sql.ErrNoRows {
		return snapshot, nil
	}
	if err != nil {
		return characterTableSnapshot{}, fmt.Errorf("snapshot auth account online state: %w", err)
	}
	value := strconv.FormatInt(online, 10)
	hash := sha256.Sum256([]byte(value))
	snapshot.Rows, snapshot.Digest, snapshot.Columns["online"] = 1, hex.EncodeToString(hash[:]), value
	return snapshot, nil
}
