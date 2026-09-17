package http

import "net/http"

type devLoginRequest struct {
	Email string `json:"email"`
	Name  string `json:"name"`
}

type devLoginResponse struct {
	Token  string `json:"token"`
	UserID string `json:"userId"`
}

// devLoginHandler issues a locally-signed token for local development only
// (§26). It is unauthenticated by construction (you need it to get a
// token) and returns 501 whenever PLATFORM_AUTH_MODE=oidc, since dev login
// is nonsensical against a real identity provider.
func devLoginHandler(d Deps) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var req devLoginRequest
		if err := decodeJSON(r, &req); err != nil {
			writeError(w, err)
			return
		}
		token, u, err := d.AuthService.DevLogin(r.Context(), req.Email, req.Name)
		if err != nil {
			writeError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, devLoginResponse{Token: token, UserID: u.ID.String()})
	}
}
