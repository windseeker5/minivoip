package store

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
)

// Logos are the businesses' logos, shown to callers instead of initials.
// They're public at /logo/<name>; names are random.

// MaxLogoBytes caps one logo.
const MaxLogoBytes = 2 << 20

// ErrBadLogo: the file isn't a PNG, JPG or WebP picture, or is too big.
var ErrBadLogo = errors.New("Use a PNG, JPG or WebP picture of 2 MB or less.")

var logoName = regexp.MustCompile(`^logo-[0-9a-f]{32}\.(png|jpg|webp)$`)

var logoExt = map[string]string{"image/png": ".png", "image/jpeg": ".jpg", "image/webp": ".webp"}

func (s *Store) logoDir() string { return filepath.Join(s.Dir, "logos") }

// LogoPath is the file for a logo name ("" if the name isn't valid).
func (s *Store) LogoPath(name string) string {
	if !logoName.MatchString(name) {
		return ""
	}
	return filepath.Join(s.logoDir(), name)
}

// SetLogo saves a new logo for the account's line and deletes the old one.
// The file's own bytes say what it is, not its name.
func (s *Store) SetLogo(accountID int64, r io.Reader) error {
	b, err := io.ReadAll(io.LimitReader(r, MaxLogoBytes+1))
	if err != nil {
		return err
	}
	ext, err := logoType(b)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(s.logoDir(), 0o755); err != nil {
		return err
	}
	n := make([]byte, 16)
	rand.Read(n)
	name := "logo-" + hex.EncodeToString(n) + ext
	if err := os.WriteFile(filepath.Join(s.logoDir(), name), b, 0o644); err != nil {
		return err
	}
	if err := s.putLogo(accountID, name); err != nil {
		os.Remove(filepath.Join(s.logoDir(), name))
		return err
	}
	return nil
}

// CheckLogo says whether a file would make a good logo (ErrBadLogo if not).
func CheckLogo(b []byte) error {
	_, err := logoType(b)
	return err
}

// logoType is a logo's file extension, from its own bytes.
func logoType(b []byte) (string, error) {
	ext := logoExt[http.DetectContentType(b)]
	if len(b) == 0 || len(b) > MaxLogoBytes || ext == "" {
		return "", ErrBadLogo
	}
	if ext != ".webp" {
		if c, _, err := image.DecodeConfig(bytes.NewReader(b)); err != nil || c.Width == 0 || c.Height == 0 {
			return "", ErrBadLogo
		}
	}
	return ext, nil
}

// RemoveLogo takes the logo off the account's line: callers see initials again.
func (s *Store) RemoveLogo(accountID int64) error { return s.putLogo(accountID, "") }

func (s *Store) putLogo(accountID int64, name string) error {
	var old string
	if err := s.db.QueryRow("SELECT logo FROM line WHERE account_id = ?", accountID).Scan(&old); err != nil {
		return err
	}
	if _, err := s.db.Exec("UPDATE line SET logo = ? WHERE account_id = ?", name, accountID); err != nil {
		return err
	}
	if p := s.LogoPath(old); p != "" {
		os.Remove(p)
	}
	return nil
}
