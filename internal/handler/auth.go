package handler

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/mrhangz/beeroklog-backend/internal/config"
	"github.com/mrhangz/beeroklog-backend/internal/middleware"
	"github.com/mrhangz/beeroklog-backend/internal/model"
)

type AuthHandler struct {
	db   *pgxpool.Pool
	cfg  *config.Config
	auth *middleware.Auth
}

func NewAuth(db *pgxpool.Pool, cfg *config.Config) *AuthHandler {
	return &AuthHandler{
		db:   db,
		cfg:  cfg,
		auth: middleware.NewAuth(cfg.JWTSecret),
	}
}

// --- Google SSO ---

type googleTokenRequest struct {
	IDToken string `json:"id_token"`
}

func (h *AuthHandler) Google(w http.ResponseWriter, r *http.Request) {
	var req googleTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	payload, err := verifyGoogleIDToken(r.Context(), req.IDToken, h.cfg.GoogleClientIDs)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid google token: "+err.Error())
		return
	}

	user, err := h.findOrCreateUser(r.Context(), "google", payload.Sub, payload.Email, payload.Name, payload.Picture)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	h.issueTokens(w, user.ID)
}

type googlePayload struct {
	Sub     string `json:"sub"`
	Email   string `json:"email"`
	Name    string `json:"name"`
	Picture string `json:"picture"`
}

func verifyGoogleIDToken(ctx context.Context, idToken string, clientIDs []string) (*googlePayload, error) {
	url := "https://oauth2.googleapis.com/tokeninfo?id_token=" + idToken
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("google verify request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("google returned %d: %s", resp.StatusCode, string(body))
	}

	var result struct {
		googlePayload
		Aud string `json:"aud"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode google response: %w", err)
	}

	if len(clientIDs) > 0 {
		matched := false
		for _, clientID := range clientIDs {
			if result.Aud == clientID {
				matched = true
				break
			}
		}
		if !matched {
			return nil, fmt.Errorf("audience mismatch: got %s", result.Aud)
		}
	}

	return &result.googlePayload, nil
}

// --- Apple SSO ---

type appleTokenRequest struct {
	IDToken string `json:"id_token"`
}

func (h *AuthHandler) Apple(w http.ResponseWriter, r *http.Request) {
	var req appleTokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	claims, err := verifyAppleIDToken(r.Context(), req.IDToken, h.cfg.AppleClientID)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid apple token: "+err.Error())
		return
	}

	email, _ := claims["email"].(string)
	sub, _ := claims["sub"].(string)

	user, err := h.findOrCreateUser(r.Context(), "apple", sub, email, "", "")
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create user")
		return
	}

	h.issueTokens(w, user.ID)
}

var (
	appleKeys     []appleJWK
	appleKeysMu   sync.Mutex
	appleKeysTime time.Time
)

type appleJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`
	E   string `json:"e"`
}

func fetchAppleKeys(ctx context.Context) ([]appleJWK, error) {
	appleKeysMu.Lock()
	defer appleKeysMu.Unlock()

	if time.Since(appleKeysTime) < time.Hour && len(appleKeys) > 0 {
		return appleKeys, nil
	}

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://appleid.apple.com/auth/keys", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	var result struct {
		Keys []appleJWK `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	appleKeys = result.Keys
	appleKeysTime = time.Now()
	return appleKeys, nil
}

func verifyAppleIDToken(ctx context.Context, idToken, clientID string) (jwt.MapClaims, error) {
	parser := jwt.NewParser(jwt.WithValidMethods([]string{"RS256"}))
	token, err := parser.Parse(idToken, func(t *jwt.Token) (interface{}, error) {
		kid, ok := t.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("missing kid header")
		}

		keys, err := fetchAppleKeys(ctx)
		if err != nil {
			return nil, fmt.Errorf("fetch apple keys: %w", err)
		}

		for _, k := range keys {
			if k.Kid == kid {
				return parseRSAPublicKey(k)
			}
		}
		return nil, fmt.Errorf("no matching key for kid=%s", kid)
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok || !token.Valid {
		return nil, fmt.Errorf("invalid token claims")
	}

	iss, _ := claims["iss"].(string)
	if iss != "https://appleid.apple.com" {
		return nil, fmt.Errorf("invalid issuer: %s", iss)
	}

	aud, _ := claims["aud"].(string)
	if clientID != "" && aud != clientID {
		return nil, fmt.Errorf("audience mismatch")
	}

	return claims, nil
}

func parseRSAPublicKey(k appleJWK) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(k.N)
	if err != nil {
		return nil, err
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(k.E)
	if err != nil {
		return nil, err
	}
	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)
	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// --- Shared ---

func (h *AuthHandler) findOrCreateUser(ctx context.Context, provider, providerID, email, name, avatarURL string) (*model.User, error) {
	var user model.User
	err := h.db.QueryRow(ctx,
		`SELECT id, email, display_name, avatar_url, is_admin, auth_provider, auth_provider_id, created_at
		 FROM users WHERE auth_provider = $1 AND auth_provider_id = $2`,
		provider, providerID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.IsAdmin, &user.AuthProvider, &user.AuthProviderID, &user.CreatedAt)

	if err == nil {
		if err := h.grantAdminIfConfigured(ctx, &user); err != nil {
			return nil, err
		}
		return &user, nil
	}

	if email == "" {
		email = providerID + "@" + provider + ".local"
	}
	displayName := name
	if displayName == "" {
		parts := strings.Split(email, "@")
		displayName = parts[0]
	}

	err = h.db.QueryRow(ctx,
		`INSERT INTO users (email, display_name, avatar_url, auth_provider, auth_provider_id)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (auth_provider, auth_provider_id) DO UPDATE SET email = EXCLUDED.email
		 RETURNING id, email, display_name, avatar_url, is_admin, auth_provider, auth_provider_id, created_at`,
		email, displayName, avatarURL, provider, providerID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.IsAdmin, &user.AuthProvider, &user.AuthProviderID, &user.CreatedAt)

	if err != nil {
		return nil, fmt.Errorf("insert user: %w", err)
	}
	if err := h.grantAdminIfConfigured(ctx, &user); err != nil {
		return nil, err
	}
	return &user, nil
}

// grantAdminIfConfigured sets is_admin when the email is listed in ADMIN_EMAILS.
// Never clears an existing admin flag.
func (h *AuthHandler) grantAdminIfConfigured(ctx context.Context, user *model.User) error {
	if user.IsAdmin {
		return nil
	}
	for _, email := range h.cfg.AdminEmails {
		if strings.EqualFold(strings.TrimSpace(email), strings.TrimSpace(user.Email)) {
			if _, err := h.db.Exec(ctx, `UPDATE users SET is_admin = true WHERE id = $1`, user.ID); err != nil {
				return fmt.Errorf("grant admin: %w", err)
			}
			user.IsAdmin = true
			return nil
		}
	}
	return nil
}

// IsAdmin implements middleware.AdminChecker.
func (h *AuthHandler) IsAdmin(ctx context.Context, userID string) (bool, error) {
	var isAdmin bool
	err := h.db.QueryRow(ctx, `SELECT is_admin FROM users WHERE id = $1`, userID).Scan(&isAdmin)
	return isAdmin, err
}

func (h *AuthHandler) issueTokens(w http.ResponseWriter, userID string) {
	tokens, err := h.auth.Issue(userID, h.cfg.JWTAccessTTL, h.cfg.JWTRefreshTTL)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to issue tokens")
		return
	}
	respondJSON(w, http.StatusOK, tokens)
}

// Refresh exchanges a refresh token for a new token pair.
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	userID, err := h.auth.ValidateRefresh(req.RefreshToken)
	if err != nil {
		respondError(w, http.StatusUnauthorized, "invalid refresh token")
		return
	}

	h.issueTokens(w, userID)
}

// Me returns the current user's profile.
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var user model.User
	err := h.db.QueryRow(r.Context(),
		`SELECT id, email, display_name, avatar_url, is_admin, auth_provider, auth_provider_id, created_at
		 FROM users WHERE id = $1`, userID,
	).Scan(&user.ID, &user.Email, &user.DisplayName, &user.AvatarURL, &user.IsAdmin, &user.AuthProvider, &user.AuthProviderID, &user.CreatedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found")
		return
	}
	if err := h.grantAdminIfConfigured(r.Context(), &user); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to sync admin flag")
		return
	}

	respondJSON(w, http.StatusOK, user)
}

// UpdateMe updates the current user's profile.
func (h *AuthHandler) UpdateMe(w http.ResponseWriter, r *http.Request) {
	userID := middleware.GetUserID(r.Context())

	var req struct {
		DisplayName *string `json:"display_name"`
		AvatarURL   *string `json:"avatar_url"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	if req.DisplayName != nil {
		if _, err := h.db.Exec(r.Context(), `UPDATE users SET display_name = $1 WHERE id = $2`, *req.DisplayName, userID); err != nil {
			respondError(w, http.StatusInternalServerError, "update failed")
			return
		}
	}
	if req.AvatarURL != nil {
		if _, err := h.db.Exec(r.Context(), `UPDATE users SET avatar_url = $1 WHERE id = $2`, *req.AvatarURL, userID); err != nil {
			respondError(w, http.StatusInternalServerError, "update failed")
			return
		}
	}

	h.Me(w, r)
}
