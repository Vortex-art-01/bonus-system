package handler

import (
	"errors"
	"net/http"
	"unicode/utf8"

	"github.com/Vortex-art-01/bonus-system/internal/auth"
	"github.com/Vortex-art-01/bonus-system/internal/middleware"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

const maxLoginLength = 64

type credentialsRequest struct {
	Login    string `json:"login"`
	Password string `json:"password"`
}

func (c credentialsRequest) validate() error {
	switch {
	case c.Login == "":
		return errors.New("login is required")
	case utf8.RuneCountInString(c.Login) > maxLoginLength:
		return errors.New("login is too long")
	case c.Password == "":
		return errors.New("password is required")
	case len(c.Password) > auth.MaxPasswordLength:
		return errors.New("password is too long")
	}
	return nil
}

func (h *Handler) register(w http.ResponseWriter, r *http.Request) {
	creds, ok := h.readCredentials(w, r)
	if !ok {
		return
	}

	token, err := h.users.Register(r.Context(), creds.Login, creds.Password)
	switch {
	case errors.Is(err, model.ErrLoginTaken):
		http.Error(w, "login is already taken", http.StatusConflict)
	case err != nil:
		h.internalError(w, r, err)
	default:
		setAuthToken(w, token)
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) login(w http.ResponseWriter, r *http.Request) {
	creds, ok := h.readCredentials(w, r)
	if !ok {
		return
	}

	token, err := h.users.Login(r.Context(), creds.Login, creds.Password)
	switch {
	case errors.Is(err, model.ErrInvalidCredentials):
		http.Error(w, "invalid login or password", http.StatusUnauthorized)
	case err != nil:
		h.internalError(w, r, err)
	default:
		setAuthToken(w, token)
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) readCredentials(w http.ResponseWriter, r *http.Request) (credentialsRequest, bool) {
	var creds credentialsRequest
	if err := decodeJSON(w, r, &creds); err != nil {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return creds, false
	}
	if err := creds.validate(); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return creds, false
	}
	return creds, true
}

func setAuthToken(w http.ResponseWriter, token string) {
	w.Header().Set("Authorization", "Bearer "+token)
	http.SetCookie(w, &http.Cookie{
		Name:     middleware.CookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	})
}
