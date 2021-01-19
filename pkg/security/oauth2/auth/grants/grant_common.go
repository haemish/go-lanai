package grants

import (
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/oauth2/auth"
)

func CommonPreGrantValidation(c context.Context, client auth.OAuth2Client, request *auth.TokenRequest) error {
	// check scope
	if e := auth.ValidateGrant(c, client, request.GrantType); e != nil {
		return e
	}

	// check scope
	if e := auth.ValidateAllScopes(c, client, request.Scopes); e != nil {
		return e
	}
	return nil
}
