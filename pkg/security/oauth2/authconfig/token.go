package authconfig

import (
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/oauth2/auth/grants"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/oauth2/auth/token"
)

type TokenEndpointSecurityConfigurer struct {
	config *AuthorizationServerConfiguration
}

func (c *TokenEndpointSecurityConfigurer) Configure(ws security.WebSecurity) {
	// TODO Complete this
	// For Token endpoint
	ws.With(token.NewEndpoint().
		Path(c.config.Endpoints.Token).
		AddGranter(grants.NewClientCredentialsGranter()),
	)
	c.config.configureClientAuth(ws)
}
