package app

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"crm/pkg/auth"
	"crm/pkg/model"
	"crm/pkg/store"
)

const (
	maxAuthBodyBytes = 64 << 10
	maxEmailBytes    = 254
	maxNameRunes     = 200
	tokenWriteBudget = 5 * time.Second

	msgInvalidBody    = "invalid request body"
	msgInvalidLogin   = "invalid email or password"
	msgInvalidRefresh = "invalid refresh token"
	msgUnauthorized   = "unauthorized"
	msgInternal       = "internal server error"
)

type authResponse struct {
	Token        string     `json:"token"`
	RefreshToken string     `json:"refresh_token"`
	User         model.User `json:"user"`
}

type registerRequest struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type refreshRequest struct {
	RefreshToken string `json:"refresh_token"`
}

// decodeJSON reads one JSON value from a size-limited body.
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxAuthBodyBytes))
	if err := dec.Decode(v); err != nil {
		return err
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("trailing data after JSON value")
	}
	return nil
}

func validEmail(email string) bool {
	if email == "" || len(email) > maxEmailBytes {
		return false
	}
	parsed, err := mail.ParseAddress(email)
	return err == nil && parsed.Address == email
}

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	var req registerRequest
	if err := decodeJSON(w, r, &req); err != nil {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return
	}
	email := strings.TrimSpace(req.Email)
	name := strings.TrimSpace(req.Name)
	switch {
	case !validEmail(email):
		errResp(w, http.StatusBadRequest, "a valid email is required")
		return
	case name == "" || utf8.RuneCountInString(name) > maxNameRunes:
		errResp(w, http.StatusBadRequest, "a name of at most 200 characters is required")
		return
	case len(req.Password) < auth.MinPasswordBytes || len(req.Password) > auth.MaxPasswordBytes:
		errResp(w, http.StatusBadRequest, "password must be between 8 and 72 bytes")
		return
	}

	hash, err := a.passwords.Hash(req.Password)
	if err != nil {
		log.Printf("register: hash password: %v", err)
		errResp(w, http.StatusInternalServerError, msgInternal)
		return
	}
	user, err := a.store.CreateUser(r.Context(), email, name, hash)
	if errors.Is(err, store.ErrEmailTaken) {
		errResp(w, http.StatusConflict, "email already registered")
		return
	}
	if err != nil {
		log.Printf("register: %v", err)
		errResp(w, http.StatusInternalServerError, msgInternal)
		return
	}
	a.startSession(w, r, http.StatusCreated, user)
}

// login answers a wrong password and an unknown email alike, in content and in
// time. An account check that depends on the user, such as a future disabled
// flag, must come after password verification for the same reason.
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := decodeJSON(w, r, &req); err != nil || req.Email == "" || req.Password == "" {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return
	}

	user, hash, err := a.store.UserCredentialsByEmail(r.Context(), strings.TrimSpace(req.Email))
	if errors.Is(err, store.ErrUserNotFound) {
		a.passwords.VerifyUnknown(req.Password)
		errResp(w, http.StatusUnauthorized, msgInvalidLogin)
		return
	}
	if err != nil {
		log.Printf("login: %v", err)
		errResp(w, http.StatusInternalServerError, msgInternal)
		return
	}
	if !a.passwords.Verify(hash, req.Password) {
		errResp(w, http.StatusUnauthorized, msgInvalidLogin)
		return
	}
	a.startSession(w, r, http.StatusOK, user)
}

func (a *App) startSession(w http.ResponseWriter, r *http.Request, status int, user model.User) {
	refreshToken, err := a.store.IssueRefreshToken(r.Context(), user.ID, a.refreshTTL)
	if err != nil {
		log.Printf("issue refresh token: %v", err)
		errResp(w, http.StatusInternalServerError, msgInternal)
		return
	}
	a.writeSession(w, status, user, refreshToken)
}

func (a *App) writeSession(w http.ResponseWriter, status int, user model.User, refreshToken string) {
	token, err := a.tokens.Generate(user.ID)
	if err != nil {
		log.Printf("generate access token: %v", err)
		errResp(w, http.StatusInternalServerError, msgInternal)
		return
	}
	dataResp(w, status, authResponse{Token: token, RefreshToken: refreshToken, User: user})
}

// detachedTokenContext keeps token writes running after the client disconnects.
// Otherwise a thief replaying a stolen token could hang up to cancel the family
// revocation it triggers.
func detachedTokenContext(r *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(r.Context()), tokenWriteBudget)
}

func (a *App) refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil || req.RefreshToken == "" {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return
	}

	ctx, cancel := detachedTokenContext(r)
	defer cancel()
	user, next, err := a.store.RotateRefreshToken(ctx, req.RefreshToken, a.refreshTTL, a.refreshGrace)
	if errors.Is(err, store.ErrInvalidRefreshToken) {
		errResp(w, http.StatusUnauthorized, msgInvalidRefresh)
		return
	}
	if err != nil {
		log.Printf("refresh: %v", err)
		errResp(w, http.StatusInternalServerError, msgInternal)
		return
	}
	a.writeSession(w, http.StatusOK, user, next)
}

// logout revokes the presented token's family. An unknown token gets the same
// answer as a known one.
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := decodeJSON(w, r, &req); err != nil || req.RefreshToken == "" {
		errResp(w, http.StatusBadRequest, msgInvalidBody)
		return
	}

	ctx, cancel := detachedTokenContext(r)
	defer cancel()
	if err := a.store.RevokeRefreshFamily(ctx, req.RefreshToken); err != nil {
		log.Printf("logout: %v", err)
		errResp(w, http.StatusInternalServerError, msgInternal)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type contextKey struct{}

// requireAuth rejects a request unless it carries a valid access token. Every
// path that does not verify a token ends in a 401.
func (a *App) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, token, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") || token == "" {
			unauthorized(w)
			return
		}
		userID, err := a.tokens.Verify(token)
		if err != nil {
			unauthorized(w)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), contextKey{}, userID)))
	})
}

func unauthorized(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	errResp(w, http.StatusUnauthorized, msgUnauthorized)
}

func (a *App) me(w http.ResponseWriter, r *http.Request) {
	userID, ok := r.Context().Value(contextKey{}).(string)
	if !ok {
		unauthorized(w)
		return
	}
	user, err := a.store.UserByID(r.Context(), userID)
	if errors.Is(err, store.ErrUserNotFound) {
		unauthorized(w)
		return
	}
	if err != nil {
		log.Printf("me: %v", err)
		errResp(w, http.StatusInternalServerError, msgInternal)
		return
	}
	dataResp(w, http.StatusOK, user)
}
