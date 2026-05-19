package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TokenStore manages the token file on disk.
type TokenStore struct {
	path string
}

// NewTokenStore creates a new TokenStore.
func NewTokenStore(path string) *TokenStore {
	return &TokenStore{path: path}
}

// Save stores a token as a JSON file.
// Creates the directory if needed and sets file permissions to 0600.
func (ts *TokenStore) Save(token *Token) error {
	dir := filepath.Dir(ts.path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("verzeichnis erstellen (%s): %w", dir, err)
	}

	data, err := json.MarshalIndent(token, "", "  ")
	if err != nil {
		return fmt.Errorf("JSON marshal: %w", err)
	}

	if err := os.WriteFile(ts.path, data, 0600); err != nil {
		return fmt.Errorf("datei schreiben (%s): %w", ts.path, err)
	}

	return nil
}

// Load reads a token from the JSON file.
// Returns (nil, error) if the file does not exist or is invalid.
func (ts *TokenStore) Load() (*Token, error) {
	data, err := os.ReadFile(ts.path)
	if err != nil {
		return nil, fmt.Errorf("datei lesen (%s): %w", ts.path, err)
	}

	var token Token
	if err := json.Unmarshal(data, &token); err != nil {
		return nil, fmt.Errorf("JSON unmarshal: %w", err)
	}

	return &token, nil
}

// Delete removes the token file (e.g. on logout).
func (ts *TokenStore) Delete() error {
	if err := os.Remove(ts.path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("datei löschen (%s): %w", ts.path, err)
	}
	return nil
}
