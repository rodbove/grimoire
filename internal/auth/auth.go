package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	cookieName = "grimoire_session"
	sessionTTL = 30 * 24 * time.Hour
	keyPass    = "password_hash"
)

var ErrBadPassword = errors.New("wrong password")

type Auth struct {
	DB *sql.DB
}

func hashPassword(pw string) string {
	salt := make([]byte, 16)
	rand.Read(salt)
	key := argon2.IDKey([]byte(pw), salt, 2, 32*1024, 2, 32)
	return fmt.Sprintf("argon2id$2$32768$2$%s$%s", base64.RawStdEncoding.EncodeToString(salt), base64.RawStdEncoding.EncodeToString(key))
}

func verify(pw, stored string) bool {
	parts := strings.Split(stored, "$")
	if len(parts) != 6 || parts[0] != "argon2id" {
		return false
	}
	var t, m uint32
	var p uint8
	fmt.Sscan(parts[1], &t)
	fmt.Sscan(parts[2], &m)
	fmt.Sscan(parts[3], &p)
	salt, err1 := base64.RawStdEncoding.DecodeString(parts[4])
	want, err2 := base64.RawStdEncoding.DecodeString(parts[5])
	if err1 != nil || err2 != nil {
		return false
	}
	got := argon2.IDKey([]byte(pw), salt, t, m, p, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}

func (a *Auth) IsSetUp() bool {
	var v string
	return a.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, keyPass).Scan(&v) == nil
}

func (a *Auth) SetPassword(pw string) error {
	if len(pw) < 6 {
		return errors.New("password must have 6 or more characters")
	}
	_, err := a.DB.Exec(`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, keyPass, hashPassword(pw))
	if err == nil {
		a.DB.Exec(`DELETE FROM sessions`)
	}
	return err
}

func (a *Auth) Check(pw string) bool {
	var stored string
	if a.DB.QueryRow(`SELECT value FROM settings WHERE key = ?`, keyPass).Scan(&stored) != nil {
		return false
	}
	return verify(pw, stored)
}

func tokenKey(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

func (a *Auth) Login(w http.ResponseWriter, pw string) error {
	if !a.Check(pw) {
		time.Sleep(500 * time.Millisecond)
		return ErrBadPassword
	}
	return a.startSession(w)
}

func (a *Auth) startSession(w http.ResponseWriter) error {
	b := make([]byte, 32)
	rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	exp := time.Now().Add(sessionTTL)
	if _, err := a.DB.Exec(`INSERT INTO sessions (token, expires_at) VALUES (?, ?)`, tokenKey(tok), exp.UnixMilli()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: tok, Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteStrictMode})
	return nil
}

func (a *Auth) Setup(w http.ResponseWriter, pw string) error {
	if a.IsSetUp() {
		return errors.New("password already set")
	}
	if err := a.SetPassword(pw); err != nil {
		return err
	}
	return a.startSession(w)
}

func (a *Auth) Logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		a.DB.Exec(`DELETE FROM sessions WHERE token = ?`, tokenKey(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode})
}

func (a *Auth) Valid(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	if err != nil || c.Value == "" {
		return false
	}
	var exp int64
	if a.DB.QueryRow(`SELECT expires_at FROM sessions WHERE token = ?`, tokenKey(c.Value)).Scan(&exp) != nil {
		return false
	}
	return exp > time.Now().UnixMilli()
}
