package handlers

import (
	"errors"
	"net/http"

	"github.com/codexylab/alvex-backend/pkg/apierr"
	"github.com/codexylab/alvex-backend/pkg/middleware"
	"github.com/codexylab/alvex-backend/pkg/models"
	"github.com/codexylab/alvex-backend/pkg/repository"
	"github.com/codexylab/alvex-backend/pkg/response"
	"github.com/codexylab/alvex-backend/pkg/services"
)

// AuthHandler handles authentication-related endpoints.
// Delegates database queries to the UserService.
type AuthHandler struct {
	Service          *services.UserService
	OrganizationRepo repository.OrganizationRepository
}

// meResponse is the shape returned by GET /api/v1/auth/me.
type meResponse struct {
	UserID        string                          `json:"user_id"`
	Name          string                          `json:"name"`
	Email         string                          `json:"email"`
	Role          string                          `json:"role"`
	Organizations []models.OrganizationMembership `json:"organizations"`
}

// Me returns the currently authenticated user's profile.
//
// GET /api/v1/auth/me
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r)
	if userID == "" {
		response.Unauthorized(w)
		return
	}

	identity, ok := middleware.GetAuthenticatedUser(r)
	if !ok {
		response.Unauthorized(w)
		return
	}
	name := identity.Name
	if name == "" {
		name = "Alvex User"
	}
	if err := h.Service.Upsert(r.Context(), identity.ID, name, identity.Email); err != nil {
		response.InternalError(w)
		return
	}

	// Fetch from user service
	u, err := h.Service.GetByID(r.Context(), userID)
	if err != nil {
		var notFoundErr *apierr.NotFoundError
		if errors.As(err, &notFoundErr) {
			response.NotFound(w, "User")
			return
		}
		response.InternalError(w)
		return
	}
	memberships, err := h.OrganizationRepo.ListActiveMemberships(r.Context(), u.ID)
	if err != nil {
		response.InternalError(w)
		return
	}

	response.Success(w, meResponse{
		UserID:        u.ID,
		Name:          u.Name,
		Email:         u.Email,
		Role:          u.PlatformRole,
		Organizations: memberships,
	})
}
