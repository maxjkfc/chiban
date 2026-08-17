package httpapi

import (
	"errors"
	"net/http"

	"github.com/google/uuid"

	"github.com/maxjkfc/chiban/apps/api/internal/auth"
	"github.com/maxjkfc/chiban/apps/api/internal/group"
)

type createGroupRequest struct {
	Name string `json:"name"`
}

type joinGroupRequest struct {
	Code string `json:"code"`
}

type groupResponse struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Role    string `json:"role"`
	IsOwner bool   `json:"is_owner"`
}

type memberResponse struct {
	UserID        string `json:"user_id"`
	DisplayName   string `json:"display_name"`
	AvatarMediaID string `json:"avatar_media_id,omitempty"`
	Role          string `json:"role"`
}

type inviteResponse struct {
	ID        string `json:"id"`
	Code      string `json:"code"`
	ExpiresAt string `json:"expires_at"`
}

func newGroupResponse(g group.Group, userID uuid.UUID) groupResponse {
	return groupResponse{
		ID:      g.ID.String(),
		Name:    g.Name,
		Role:    g.Role,
		IsOwner: g.OwnerID == userID,
	}
}

func createGroupHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req createGroupRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		userID := auth.UserFromContext(r.Context()).ID
		g, err := d.Group.Create(r.Context(), userID, req.Name)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}
		writeJSON(w, http.StatusCreated, newGroupResponse(g, userID))
	}
}

func listGroupsHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		userID := auth.UserFromContext(r.Context()).ID
		groups, err := d.Group.ListForUser(r.Context(), userID)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}

		out := make([]groupResponse, 0, len(groups))
		for _, g := range groups {
			out = append(out, newGroupResponse(g, userID))
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func getGroupHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, ok := pathUUID(w, r, "group_id")
		if !ok {
			return
		}

		userID := auth.UserFromContext(r.Context()).ID
		g, err := d.Group.Get(r.Context(), userID, groupID)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}
		writeJSON(w, http.StatusOK, newGroupResponse(g, userID))
	}
}

// listMembersHandler asks the profile domain for display names rather than
// joining across domains in one query.
func listMembersHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, ok := pathUUID(w, r, "group_id")
		if !ok {
			return
		}

		members, err := d.Group.Members(r.Context(), auth.UserFromContext(r.Context()).ID, groupID)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}

		userIDs := make([]uuid.UUID, 0, len(members))
		for _, m := range members {
			userIDs = append(userIDs, m.UserID)
		}
		summaries, err := d.Profile.Summaries(r.Context(), userIDs)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}

		out := make([]memberResponse, 0, len(members))
		for _, m := range members {
			summary := summaries[m.UserID]
			member := memberResponse{
				UserID:      m.UserID.String(),
				DisplayName: summary.DisplayName,
				Role:        m.Role,
			}
			if summary.HasAvatar() {
				member.AvatarMediaID = summary.AvatarMediaID.String()
			}
			out = append(out, member)
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func createInviteHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, ok := pathUUID(w, r, "group_id")
		if !ok {
			return
		}

		invite, err := d.Group.CreateInvite(r.Context(), auth.UserFromContext(r.Context()).ID, groupID)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}
		writeJSON(w, http.StatusCreated, inviteResponse{
			ID:        invite.ID.String(),
			Code:      invite.Code,
			ExpiresAt: invite.ExpiresAt.UTC().Format(timeFormat),
		})
	}
}

func listInvitesHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, ok := pathUUID(w, r, "group_id")
		if !ok {
			return
		}

		invites, err := d.Group.ListInvites(r.Context(), auth.UserFromContext(r.Context()).ID, groupID)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}

		out := make([]inviteResponse, 0, len(invites))
		for _, invite := range invites {
			out = append(out, inviteResponse{
				ID:        invite.ID.String(),
				Code:      invite.Code,
				ExpiresAt: invite.ExpiresAt.UTC().Format(timeFormat),
			})
		}
		writeJSON(w, http.StatusOK, out)
	}
}

func revokeInviteHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, ok := pathUUID(w, r, "group_id")
		if !ok {
			return
		}
		inviteID, ok := pathUUID(w, r, "invite_id")
		if !ok {
			return
		}

		err := d.Group.RevokeInvite(r.Context(), auth.UserFromContext(r.Context()).ID, groupID, inviteID)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func joinGroupHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req joinGroupRequest
		if !decodeJSON(w, r, &req) {
			return
		}

		userID := auth.UserFromContext(r.Context()).ID
		g, err := d.Group.Join(r.Context(), userID, req.Code)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}
		writeJSON(w, http.StatusOK, newGroupResponse(g, userID))
	}
}

func leaveGroupHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		groupID, ok := pathUUID(w, r, "group_id")
		if !ok {
			return
		}

		err := d.Group.Leave(r.Context(), auth.UserFromContext(r.Context()).ID, groupID)
		if err != nil {
			writeGroupError(w, d, err)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	}
}

func writeGroupError(w http.ResponseWriter, d Deps, err error) {
	var invalid group.InvalidInputError
	switch {
	case errors.As(err, &invalid):
		writeError(w, http.StatusBadRequest, invalid.Message, invalid.Field)
	case errors.Is(err, group.ErrNotMember):
		// Same answer for "no such group" and "not your group": otherwise the
		// API confirms which group IDs exist.
		writeError(w, http.StatusForbidden, "you are not a member of this group")
	case errors.Is(err, group.ErrNotOwner):
		writeError(w, http.StatusForbidden, "only the group owner can do this")
	case errors.Is(err, group.ErrOwnerCannotLeave):
		writeError(w, http.StatusConflict, "the owner cannot leave the group")
	case errors.Is(err, group.ErrInviteInvalid):
		writeError(w, http.StatusNotFound, "this invite is no longer valid")
	default:
		d.Logger.Error("group request failed", "error", err)
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
