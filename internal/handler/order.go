package handler

import (
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/Vortex-art-01/bonus-system/internal/model"
)

const maxOrderNumberSize = 1 << 10

type orderResponse struct {
	Number     string   `json:"number"`
	Status     string   `json:"status"`
	Accrual    *float64 `json:"accrual,omitempty"`
	UploadedAt string   `json:"uploaded_at"`
}

func (h *Handler) uploadOrder(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxOrderNumberSize))
	if err != nil {
		http.Error(w, "cannot read request body", http.StatusBadRequest)
		return
	}
	number := strings.TrimSpace(string(body))
	if number == "" {
		http.Error(w, "order number is required", http.StatusBadRequest)
		return
	}

	err = h.orders.Upload(r.Context(), userID, number)
	switch {
	case errors.Is(err, model.ErrInvalidOrderNumber):
		http.Error(w, "invalid order number", http.StatusUnprocessableEntity)
	case errors.Is(err, model.ErrOrderAlreadyUploaded):
		w.WriteHeader(http.StatusOK)
	case errors.Is(err, model.ErrOrderUploadedByAnother):
		http.Error(w, "order was uploaded by another user", http.StatusConflict)
	case err != nil:
		h.internalError(w, r, err)
	default:
		w.WriteHeader(http.StatusAccepted)
	}
}

func (h *Handler) listOrders(w http.ResponseWriter, r *http.Request) {
	userID, ok := h.userID(w, r)
	if !ok {
		return
	}

	orders, err := h.orders.List(r.Context(), userID)
	if err != nil {
		h.internalError(w, r, err)
		return
	}
	if len(orders) == 0 {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	resp := make([]orderResponse, 0, len(orders))
	for _, o := range orders {
		resp = append(resp, orderResponse{
			Number:     o.Number,
			Status:     string(o.Status),
			Accrual:    o.Accrual,
			UploadedAt: o.UploadedAt.Format(time.RFC3339),
		})
	}
	h.writeJSON(w, http.StatusOK, resp)
}
