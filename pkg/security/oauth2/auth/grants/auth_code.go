package grants

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/oauth2"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/oauth2/auth"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/utils"
	"fmt"
)

var (
	authCodeIgnoreParams = utils.NewStringSet(
		oauth2.ParameterScope,
		oauth2.ParameterClientSecret,
	)
)

// AuthorizationCodeGranter implements auth.TokenGranter
type AuthorizationCodeGranter struct {
	authService   auth.AuthorizationService
	authCodeStore auth.AuthorizationCodeStore
}

func NewAuthorizationCodeGranter(authService auth.AuthorizationService, authCodeStore auth.AuthorizationCodeStore) *AuthorizationCodeGranter {
	if authService == nil {
		panic(fmt.Errorf("cannot create AuthorizationCodeGranter without auth service."))
	}

	if authCodeStore == nil {
		panic(fmt.Errorf("cannot create AuthorizationCodeGranter without auth code service."))
	}

	return &AuthorizationCodeGranter{
		authService: authService,
		authCodeStore: authCodeStore,
	}
}

func (g *AuthorizationCodeGranter) Grant(ctx context.Context, request *auth.TokenRequest) (oauth2.AccessToken, error) {
	if oauth2.GrantTypeAuthCode != request.GrantType {
		return nil, nil
	}

	client := auth.RetrieveAuthenticatedClient(ctx)

	// common check
	if e := auth.ValidateGrant(ctx, client, request.GrantType); e != nil {
		return nil, e
	}

	// load authentication using auth code
	code, ok := request.Extensions[oauth2.ParameterAuthCode].(string)
	if !ok || code == "" {
		return nil, oauth2.NewInvalidTokenRequestError(fmt.Sprintf("missing required parameter %s", oauth2.ParameterAuthCode))
	}

	stored, e := g.authCodeStore.ConsumeAuhtorizationCode(ctx, code, true)
	if e != nil {
		return nil, e
	} else if !stored.OAuth2Request().Approved() || stored.UserAuthentication() == nil {
		return nil, oauth2.NewInvalidGrantError("original authorize request is invalid")
	}

	// check redirect uri
	if e := validateRedirectUri(stored.OAuth2Request(), request); e != nil {
		return nil, e
	}

	// check client ID
	if stored.OAuth2Request().ClientId() != client.ClientId() {
		return nil, oauth2.NewInvalidGrantError("client ID mismatch")
	}

	// create authentication from stored value
	oauthRequest, e := mergedOAuth2Request(stored.OAuth2Request(), request, authCodeIgnoreParams)
	if e != nil {
		return nil, e
	}

	oauth, e := g.authService.CreateAuthentication(ctx, oauthRequest, stored.UserAuthentication())
	if e != nil {
		return nil, oauth2.NewInvalidGrantError(e.Error(), e)
	}

	// create token
	token, e := g.authService.CreateAccessToken(ctx, oauth)
	if e != nil {
		return nil, oauth2.NewInvalidGrantError(e.Error(), e)
	}
	return token, nil
}

// https://tools.ietf.org/html/rfc6749#section-4.1.3
// if redirect_uri was provided in original request (not implied from client registrition).
// the same redirect_uri must be provided in token request
func validateRedirectUri(stored oauth2.OAuth2Request, request *auth.TokenRequest) error {
	origRedirect, ok := stored.Parameters()[oauth2.ParameterRedirectUri]
	if !ok || origRedirect == "" {
		// nothing wrong, redirect was not provided, probably implied from client registration
		return nil
	}

	reqRedirect, ok := request.Parameters[oauth2.ParameterRedirectUri]
	if !ok {
		return oauth2.NewInvalidTokenRequestError("redirect_uri is required because redirect URL was provided when obtaining the auth code")
	} else if reqRedirect != origRedirect {
		return oauth2.NewInvalidGrantError("redirect_uri doesn't match the original redirect URL used when obtaining the auth code")
	}

	return nil
}

func mergedOAuth2Request(src oauth2.OAuth2Request, request *auth.TokenRequest, ignoreParams utils.StringSet) (oauth2.OAuth2Request, error) {
	return src.NewOAuth2Request(func(opt *oauth2.RequestDetails) {
		opt.GrantType = request.GrantType
		for k, v := range request.Parameters {
			if ignoreParams.Has(k) {
				continue
			}
			opt.Parameters[k] = v
		}
		for k, v := range request.Extensions {
			if ignoreParams.Has(k) {
				continue
			}
			opt.Extensions[k] = v
		}
	}), nil
}