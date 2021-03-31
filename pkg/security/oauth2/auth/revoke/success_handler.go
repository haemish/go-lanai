package revoke

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/oauth2"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/oauth2/auth"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/redirect"
	"fmt"
	"net/http"
)

type SuccessOptions func(opt *SuccessOption)

type SuccessOption struct {
	ClientStore         oauth2.OAuth2ClientStore
	WhitelabelErrorPath string
}

// TokenRevokeSuccessHandler implements security.AuthenticationSuccessHandler
type TokenRevokeSuccessHandler struct {
	clientStore oauth2.OAuth2ClientStore
	fallback    security.AuthenticationErrorHandler
}

func NewTokenRevokeSuccessHandler(opts...SuccessOptions) *TokenRevokeSuccessHandler {
	opt := SuccessOption{}
	for _, f := range opts {
		f(&opt)
	}
	return &TokenRevokeSuccessHandler{
		clientStore: opt.ClientStore,
		fallback: redirect.NewRedirectWithURL(opt.WhitelabelErrorPath),
	}
}

func (h TokenRevokeSuccessHandler) HandleAuthenticationSuccess(ctx context.Context, r *http.Request, rw http.ResponseWriter, from, to security.Authentication) {
	switch r.Method {
	case http.MethodGet:
		fallthrough
	case http.MethodPost:
		h.redirect(ctx, r, rw, from, to)
	case http.MethodPut:
		fallthrough
	case http.MethodDelete:
		fallthrough
	default:
		h.status(ctx, rw)
	}
}

func (h TokenRevokeSuccessHandler) redirect(ctx context.Context, r *http.Request, rw http.ResponseWriter, from, to security.Authentication) {
	// Note: we don't have error handling alternatives (except for panic)
	redirectUri := r.FormValue(oauth2.ParameterRedirectUri)
	if redirectUri == "" {
		h.fallback.HandleAuthenticationError(ctx, r, rw, fmt.Errorf("missing %s", oauth2.ParameterRedirectUri))
		return
	}

	clientId := r.FormValue(oauth2.ParameterClientId)
	client, e := auth.LoadAndValidateClientId(ctx, clientId, h.clientStore)
	if e != nil {
		h.fallback.HandleAuthenticationError(ctx, r, rw, e)
		return
	}

	resolved, e := auth.ResolveRedirectUri(ctx, redirectUri, client)
	if e != nil {
		// TODO should we still respect global whitelist?
		h.fallback.HandleAuthenticationError(ctx, r, rw, e)
		return
	}

	// redirect
	http.Redirect(rw, r, resolved, http.StatusFound)
	_,_ = rw.Write([]byte{})
}

// In case of PUT, DELETE, PATCH etc, we don't clean authentication. Instead, we invalidate access token carried by header
func (h TokenRevokeSuccessHandler) status(ctx context.Context, rw http.ResponseWriter) {
	rw.WriteHeader(http.StatusOK)
	_, _ = rw.Write([]byte{})
}





