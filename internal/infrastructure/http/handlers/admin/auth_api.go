package admin

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/auth"
	"github.com/bobbyunknown/flamegate/internal/infrastructure/http/openapi"
)

// --- Login ---

type LoginInput struct {
	Body struct {
		Username string `json:"username,omitempty" doc:"Username (default: admin)"`
		Password string `json:"password" doc:"Password"`
	}
}

type LoginOutput struct {
	Body struct {
		OK                 bool `json:"ok"`
		UsingDefault       bool `json:"using_default"`
		OnboardingComplete bool `json:"onboarding_complete"`
	}
}

func (s *Handler) HumaLogin(ctx context.Context, input *LoginInput) (*LoginOutput, error) {
	username := input.Body.Username
	if username == "" {
		username = auth.DefaultUsername
	}

	_, ok, err := s.auth.VerifyPassword(ctx, username, input.Body.Password)
	if err != nil || !ok {
		s.log.WithField("username", username).Warn("auth: login failed")
		return nil, huma.Error401Unauthorized("invalid credentials")
	}

	token, err := s.auth.IssueSession(username)
	if err != nil {
		s.log.WithField("username", username).Error("auth: failed to issue session")
		return nil, huma.Error500InternalServerError("failed to create session")
	}

	w := openapi.ResponseWriterFromContext(ctx)
	http.SetCookie(w, &http.Cookie{
		Name:     "fg_session",
		Value:    token,
		Path:     "/",
		Expires:  time.Now().Add(s.auth.TTL()),
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})

	s.log.WithField("username", username).Info("auth: login ok")
	return &LoginOutput{
		Body: struct {
			OK                 bool `json:"ok"`
			UsingDefault       bool `json:"using_default"`
			OnboardingComplete bool `json:"onboarding_complete"`
		}{
			OK:                 true,
			UsingDefault:       s.auth.UsingDefaultPassword(ctx),
			OnboardingComplete: s.auth.OnboardingComplete(ctx),
		},
	}, nil
}

// --- Token (OAuth2 password flow) ---

type TokenInput struct {
	Body struct {
		GrantType string `json:"grant_type" form:"grant_type" doc:"OAuth2 grant type (use 'password')"`
		Username  string `json:"username" form:"username" doc:"Username"`
		Password  string `json:"password" form:"password" doc:"Password"`
	}
}

type TokenOutput struct {
	Body struct {
		AccessToken string `json:"access_token"`
		TokenType   string `json:"token_type"`
		ExpiresIn   int    `json:"expires_in"`
	}
}

func (s *Handler) HumaToken(ctx context.Context, input *TokenInput) (*TokenOutput, error) {
	grantType := strings.TrimSpace(input.Body.GrantType)
	username := strings.TrimSpace(input.Body.Username)
	password := input.Body.Password

	if grantType != "password" {
		return nil, huma.Error400BadRequest("unsupported_grant_type")
	}
	if username == "" {
		username = auth.DefaultUsername
	}
	if password == "" {
		return nil, huma.Error400BadRequest("password is required")
	}

	_, ok, err := s.auth.VerifyPassword(ctx, username, password)
	if err != nil || !ok {
		s.log.WithField("username", username).Warn("auth: token failed")
		return nil, huma.Error401Unauthorized("invalid_grant")
	}

	token, err := s.auth.IssueSession(username)
	if err != nil {
		s.log.WithField("username", username).Error("auth: failed to issue token")
		return nil, huma.Error500InternalServerError("failed to create session")
	}

	s.log.WithField("username", username).Info("auth: token issued")
	return &TokenOutput{
		Body: struct {
			AccessToken string `json:"access_token"`
			TokenType   string `json:"token_type"`
			ExpiresIn   int    `json:"expires_in"`
		}{
			AccessToken: token,
			TokenType:   "Bearer",
			ExpiresIn:   int(s.auth.TTL().Seconds()),
		},
	}, nil
}

// --- Auth Status ---

type AuthStatusInput struct{}

type AuthStatusOutput struct {
	Body struct {
		Authenticated      bool   `json:"authenticated"`
		Username           string `json:"username,omitempty"`
		UsingDefault       bool   `json:"using_default"`
		OnboardingComplete bool   `json:"onboarding_complete"`
	}
}

func (s *Handler) HumaAuthStatus(ctx context.Context, _ *AuthStatusInput) (*AuthStatusOutput, error) {
	r := openapi.RequestFromContext(ctx)
	authed := false
	var username string
	if c, err := r.Cookie("fg_session"); err == nil && c.Value != "" {
		if u, ok := s.auth.SessionUsername(c.Value); ok {
			authed = true
			username = u
		}
	}
	return &AuthStatusOutput{
		Body: struct {
			Authenticated      bool   `json:"authenticated"`
			Username           string `json:"username,omitempty"`
			UsingDefault       bool   `json:"using_default"`
			OnboardingComplete bool   `json:"onboarding_complete"`
		}{
			Authenticated:      authed,
			Username:           username,
			UsingDefault:       s.auth.UsingDefaultPassword(ctx),
			OnboardingComplete: s.auth.OnboardingComplete(ctx),
		},
	}, nil
}

// --- Logout ---

type LogoutInput struct{}

type LogoutOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (s *Handler) HumaLogout(ctx context.Context, _ *LogoutInput) (*LogoutOutput, error) {
	w := openapi.ResponseWriterFromContext(ctx)
	http.SetCookie(w, &http.Cookie{
		Name:     "fg_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   false,
		SameSite: http.SameSiteLaxMode,
	})
	s.log.Info("auth: logout")
	return &LogoutOutput{Body: struct {
		OK bool `json:"ok"`
	}{OK: true}}, nil
}

// --- Change Password ---

type ChangePasswordInput struct {
	Body struct {
		NewPassword string `json:"new_password" doc:"New password"`
	}
}

type ChangePasswordOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (s *Handler) HumaChangePassword(ctx context.Context, input *ChangePasswordInput) (*ChangePasswordOutput, error) {
	if err := s.auth.SetPassword(ctx, auth.DefaultUsername, input.Body.NewPassword); err != nil {
		s.log.WithError(err).Warn("auth: password change failed")
		return nil, huma.Error400BadRequest(sanitizeError(s.log, err, "password change failed"))
	}
	s.log.Info("auth: password changed")
	return &ChangePasswordOutput{Body: struct {
		OK bool `json:"ok"`
	}{OK: true}}, nil
}

// --- Complete Onboarding ---

type CompleteOnboardingInput struct{}

type CompleteOnboardingOutput struct {
	Body struct {
		OK bool `json:"ok"`
	}
}

func (s *Handler) HumaCompleteOnboarding(ctx context.Context, _ *CompleteOnboardingInput) (*CompleteOnboardingOutput, error) {
	if err := s.auth.CompleteOnboarding(ctx); err != nil {
		return nil, huma.Error500InternalServerError(sanitizeError(s.log, err, "onboarding completion failed"))
	}
	return &CompleteOnboardingOutput{Body: struct {
		OK bool `json:"ok"`
	}{OK: true}}, nil
}
