package oauth2

const (
	JsonFieldAccessTokenValue  = "access_token"
	JsonFieldTokenType         = "token_type"
	JsonFieldIssueTime         = "iat"
	JsonFieldExpiryTime        = "expiry"
	JsonFieldExpiresIn         = "expires_in"
	JsonFieldScope             = "scope"
	JsonFieldRefreshTokenValue = "refresh_token"
)

const (
	ParameterClientId = "client_id"
	ParameterResponseType = "response_type"
	ParameterRedirectUri = "redirect_uri"
	ParameterScope = "scope"
	ParameterState = "state"
	ParameterGrantType = "grant_type"
	//Parameter = ""
)

const (
	ExtensionAuthenticatedClient = "kAuthenticatedClient"
)

const (
	GrantTypeClientCredentials = "client_credentials"
	GrantTypePassword = "password"
	GrantTypeAuthCode = "authorization_code"
	GrantTypeImplicit = "implicit"
	GrantTypeRefresh = "refresh_token"
)