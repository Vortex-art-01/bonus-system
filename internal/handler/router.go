package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"

	"github.com/Vortex-art-01/bonus-system/internal/middleware"
	"github.com/Vortex-art-01/bonus-system/internal/model"
)

const maxBodySize = 1 << 20

type UserService interface {
	Register(ctx context.Context, login, password string) (token string, err error)
	Login(ctx context.Context, login, password string) (token string, err error)
}

type OrderService interface {
	Upload(ctx context.Context, userID int64, number string) error
	List(ctx context.Context, userID int64) ([]model.Order, error)
}

type BalanceService interface {
	Get(ctx context.Context, userID int64) (*model.Balance, error)
	Withdraw(ctx context.Context, userID int64, order string, sum model.Money) error
	ListWithdrawals(ctx context.Context, userID int64) ([]model.Withdrawal, error)
}

type Handler struct {
	users   UserService
	orders  OrderService
	balance BalanceService
	log     *slog.Logger
}

func New(users UserService, orders OrderService, balance BalanceService, log *slog.Logger) *Handler {
	return &Handler{users: users, orders: orders, balance: balance, log: log}
}

func (h *Handler) Router(tokens middleware.TokenParser) http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.Logger(h.log))
	r.Use(chimw.Recoverer)
	r.Use(middleware.Gzip)

	r.Route("/api/user", func(r chi.Router) {
		r.Post("/register", h.register)
		r.Post("/login", h.login)

		r.Group(func(r chi.Router) {
			r.Use(middleware.Auth(tokens))
			r.Post("/orders", h.uploadOrder)
			r.Get("/orders", h.listOrders)
			r.Get("/balance", h.getBalance)
			r.Post("/balance/withdraw", h.withdraw)
			r.Get("/withdrawals", h.listWithdrawals)
		})
	})

	return r
}

func (h *Handler) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		h.log.Error("write response", slog.Any("error", err))
	}
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(w, r.Body, maxBodySize)
	return json.NewDecoder(r.Body).Decode(v)
}

func (h *Handler) internalError(w http.ResponseWriter, r *http.Request, err error) {
	h.log.Error("request failed",
		slog.String("method", r.Method),
		slog.String("path", r.URL.Path),
		slog.Any("error", err),
	)
	http.Error(w, "internal server error", http.StatusInternalServerError)
}

func (h *Handler) userID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	userID, ok := middleware.UserIDFromContext(r.Context())
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
	}
	return userID, ok
}
