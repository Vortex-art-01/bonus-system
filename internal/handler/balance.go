package handler

import (
	"errors"
	"net/http"
	"time"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

type balanceResponse struct {
	Current   model.Money `json:"current"`
	Withdrawn model.Money `json:"withdrawn"`
}

type withdrawRequest struct {
	Order string      `json:"order"`
	Sum   model.Money `json:"sum"`
}

type withdrawalResponse struct {
	Order       string      `json:"order"`
	Sum         model.Money `json:"sum"`
	ProcessedAt string      `json:"processed_at"`
}

func (h *Handler) getBalance(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	balance, err := h.balance.Get(r.Context(), userID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	h.writeJSON(w, http.StatusOK, balanceResponse{Current: balance.Current, Withdrawn: balance.Withdrawn})
}

func (h *Handler) withdraw(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	var req withdrawRequest
	if err := decodeJSON(w, r, &req); err != nil {
		http.Error(w, "malformed request body", http.StatusBadRequest)
		return
	}

	err := h.balance.Withdraw(r.Context(), userID, req.Order, req.Sum)
	switch {
	case errors.Is(err, model.ErrInvalidOrderNumber):
		http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
	case errors.Is(err, model.ErrInvalidWithdrawalSum):
		http.Error(w, "sum must be positive", http.StatusBadRequest)
	case errors.Is(err, model.ErrInsufficientFunds):
		http.Error(w, "insufficient funds", http.StatusPaymentRequired)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusOK)
	}
}

func (h *Handler) listWithdrawals(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	withdrawals, err := h.balance.ListWithdrawals(r.Context(), userID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if len(withdrawals) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]withdrawalResponse, 0, len(withdrawals))
	for _, wd := range withdrawals {
		resp = append(resp, withdrawalResponse{
			Order:       wd.Order,
			Sum:         wd.Sum,
			ProcessedAt: wd.ProcessedAt.Format(time.RFC3339),
		})
	}
	h.writeJSON(w, http.StatusOK, resp)
}
