package application

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"UalaTwitter/internal/user/domain"
	appjwt "UalaTwitter/pkg/jwt"
)

const jwtTTL = 7 * 24 * time.Hour

type AuthService struct {
	userRepo           domain.UserRepository
	jwtSecret          string
	googleClientID     string
	githubClientID     string
	githubClientSecret string
	appleClientID      string
	appleJWKSURL       string
}

func NewAuthService(
	userRepo domain.UserRepository,
	jwtSecret, googleClientID, githubClientID, githubClientSecret, appleClientID, appleJWKSURL string,
) *AuthService {
	jwksURL := appleJWKSURL
	if jwksURL == "" {
		jwksURL = "https://appleid.apple.com/auth/keys"
	}
	return &AuthService{
		userRepo:           userRepo,
		jwtSecret:          jwtSecret,
		googleClientID:     googleClientID,
		githubClientID:     githubClientID,
		githubClientSecret: githubClientSecret,
		appleClientID:      appleClientID,
		appleJWKSURL:       jwksURL,
	}
}

func (s *AuthService) IssueToken(user *domain.User) (string, error) {
	return appjwt.Issue(user.ID, user.Username, s.jwtSecret, jwtTTL)
}

func (s *AuthService) FindOrCreateUser(ctx context.Context, provider, sub, displayName, _ string) (*domain.User, error) {
	user, err := s.userRepo.FindByOAuth(ctx, provider, sub)
	if err == nil {
		return user, nil
	}
	base := safeUsername(displayName)
	if base == "" {
		base = provider
	}
	username, err := s.deduplicateUsername(ctx, base)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	user = &domain.User{
		ID:            uuid.New().String(),
		Username:      username,
		CreatedAt:     now,
		LastSeenAt:    now,
		OAuthProvider: provider,
		OAuthSub:      sub,
	}
	if err := s.userRepo.Create(ctx, user); err != nil {
		return nil, err
	}
	return user, nil
}

func (s *AuthService) LoginWithGoogle(ctx context.Context, idToken string) (*domain.User, error) {
	resp, err := http.Get("https://oauth2.googleapis.com/tokeninfo?id_token=" + url.QueryEscape(idToken))
	if err != nil {
		return nil, fmt.Errorf("google tokeninfo: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("google tokeninfo rejected token (status %d)", resp.StatusCode)
	}
	var info struct {
		Aud   string `json:"aud"`
		Sub   string `json:"sub"`
		Email string `json:"email"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("google tokeninfo decode: %w", err)
	}
	if s.googleClientID != "" && info.Aud != s.googleClientID {
		return nil, fmt.Errorf("google token audience mismatch")
	}
	displayName := info.Name
	if displayName == "" {
		if idx := strings.Index(info.Email, "@"); idx > 0 {
			displayName = info.Email[:idx]
		}
	}
	return s.FindOrCreateUser(ctx, "google", info.Sub, displayName, info.Email)
}

func (s *AuthService) LoginWithGitHub(ctx context.Context, code string) (*domain.User, error) {
	form := url.Values{
		"client_id":     {s.githubClientID},
		"client_secret": {s.githubClientSecret},
		"code":          {code},
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost,
		"https://github.com/login/oauth/access_token",
		strings.NewReader(form.Encode()),
	)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("github token exchange: %w", err)
	}
	defer resp.Body.Close()
	var tokenResp struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return nil, fmt.Errorf("github token decode: %w", err)
	}
	if tokenResp.Error != "" {
		return nil, fmt.Errorf("github auth error: %s", tokenResp.Error)
	}
	req2, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.github.com/user", nil)
	req2.Header.Set("Authorization", "Bearer "+tokenResp.AccessToken)
	req2.Header.Set("Accept", "application/vnd.github+json")
	resp2, err := http.DefaultClient.Do(req2)
	if err != nil {
		return nil, fmt.Errorf("github user info: %w", err)
	}
	defer resp2.Body.Close()
	var ghUser struct {
		ID    int    `json:"id"`
		Login string `json:"login"`
		Email string `json:"email"`
	}
	if err := json.NewDecoder(resp2.Body).Decode(&ghUser); err != nil {
		return nil, fmt.Errorf("github user decode: %w", err)
	}
	return s.FindOrCreateUser(ctx, "github", strconv.Itoa(ghUser.ID), ghUser.Login, ghUser.Email)
}

func (s *AuthService) LoginWithApple(ctx context.Context, idToken string) (*domain.User, error) {
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, s.appleJWKSURL, nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("apple jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	claims, err := verifyAppleToken(idToken, resp.Body, s.appleClientID)
	if err != nil {
		return nil, fmt.Errorf("apple token verification: %w", err)
	}
	return s.FindOrCreateUser(ctx, "apple", claims.sub, claims.email, claims.email)
}

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9_]`)

func safeUsername(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = nonAlnum.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if len(s) > 30 {
		s = s[:30]
	}
	return s
}

func (s *AuthService) deduplicateUsername(ctx context.Context, base string) (string, error) {
	candidate := base
	for i := 2; i <= 20; i++ {
		_, err := s.userRepo.FindByUsername(ctx, candidate)
		if err != nil {
			return candidate, nil
		}
		candidate = fmt.Sprintf("%s_%d", base, i)
	}
	return "", fmt.Errorf("could not find available username for base %q", base)
}
