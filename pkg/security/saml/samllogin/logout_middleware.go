package samllogin

import (
	"bytes"
	"compress/flate"
	"context"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/idp"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security/saml/saml_util"
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/web"
	"encoding/base64"
	"encoding/gob"
	"github.com/beevik/etree"
	"github.com/crewjam/saml"
	"github.com/gin-gonic/gin"
	"net/http"
	"net/url"
)

const (
	SLOInitiated SLOState = 1 << iota
	SLOCompletedFully
	SLOCompletedPartially
	SLOFailed
	SLOCompleted = SLOCompletedFully | SLOCompletedPartially | SLOFailed
)

type SLOState int

func (s SLOState) Is(mask SLOState) bool {
	return s&mask != 0 || mask == 0 && s == 0
}

const (
	kDetailsSLOState = "SP.SLOState"
)

func init() {
	gob.Register(SLOState(0))
}

type SPLogoutMiddleware struct {
	SPMetadataMiddleware
	bindings           []string // supported SLO bindings, can be saml.HTTPPostBinding or saml.HTTPRedirectBinding. Order indicates preference
	successHandler     security.AuthenticationSuccessHandler
}

func NewLogoutMiddleware(sp saml.ServiceProvider,
	idpManager idp.IdentityProviderManager,
	clientManager *CacheableIdpClientManager,
	successHandler security.AuthenticationSuccessHandler) *SPLogoutMiddleware {

	return &SPLogoutMiddleware{
		SPMetadataMiddleware: SPMetadataMiddleware{
			internal:      sp,
			idpManager:    idpManager,
			clientManager: clientManager,
		},
		bindings:           []string{saml.HTTPRedirectBinding, saml.HTTPPostBinding},
		successHandler:     successHandler,
	}
}

// MakeSingleLogoutRequest initiate SLO at IdP by sending logout request with supported binding
func (m *SPLogoutMiddleware) MakeSingleLogoutRequest(ctx context.Context, r *http.Request, w http.ResponseWriter) error {
	// resolve SP client
	client, e := m.resolveIdpClient(ctx)
	if e != nil {
		return e
	}

	// resolve binding
	location, binding := m.resolveBinding(m.bindings, client.GetSLOBindingLocation)
	if location == "" {
		return security.NewExternalSamlAuthenticationError("idp does not have supported SLO bindings.")
	}

	// create and send SLO request.
	nameId, format := m.resolveNameId(ctx)
	// Note 1: MakeLogoutRequest doesn't handle Redirect properly as of crewjam/saml, we wrap it with a temporary fix
	// Note 2: SLO specs don't requires RelayState
	sloReq, e := MakeFixedLogoutRequest(client, location, nameId)
	if e != nil {
		return security.NewExternalSamlAuthenticationError("cannot make SLO request to binding location", e)
	}
	sloReq.NameID.Format = format

	switch binding {
	case saml.HTTPRedirectBinding:
		if e := m.redirectBindingExecutor(sloReq, "", client)(w, r); e != nil {
			return security.NewExternalSamlAuthenticationError("cannot make SLO request with HTTP redirect binding", e)
		}
	case saml.HTTPPostBinding:
		if e := m.postBindingExecutor(sloReq, "")(w, r); e != nil {
			return security.NewExternalSamlAuthenticationError("cannot post SLO request", e)
		}
	}
	return nil
}

// LogoutHandlerFunc returns the handler function that handles LogoutResponse/LogoutRequest sent by IdP.
// This is used to handle response of SP initiated SLO, if it's initiated by us.
// We need to continue our internal logout process
func (m *SPLogoutMiddleware) LogoutHandlerFunc() gin.HandlerFunc {
	return func(gc *gin.Context) {
		var req saml.LogoutRequest
		var resp saml.LogoutResponse
		reqR := saml_util.ParseSAMLObject(gc, &req)
		respR := saml_util.ParseSAMLObject(gc, &resp)
		switch {
		case reqR.Err != nil && respR.Err != nil || reqR.Err == nil && respR.Err == nil:
			m.handleError(gc, security.NewExternalSamlAuthenticationError("Error reading SAMLRequest/SAMLResponse", reqR.Err, respR.Err))
			return
		case respR.Err == nil:
			m.handleLogoutResponse(gc, &resp, respR.Binding, respR.Encoded)
		case reqR.Err == nil:
			m.handleLogoutRequest(gc, &req, reqR.Binding, reqR.Encoded)
		}
	}
}

// Commence implements security.AuthenticationEntryPoint. It's used when SP initiated SLO is required
func (m *SPLogoutMiddleware) Commence(ctx context.Context, r *http.Request, w http.ResponseWriter, _ error) {
	if e := m.MakeSingleLogoutRequest(ctx, r, w); e != nil {
		m.handleError(ctx, e)
		return
	}

	updateSLOState(ctx, func(current SLOState) SLOState {
		return current | SLOInitiated
	})
}

func (m *SPLogoutMiddleware) handleLogoutResponse(gc *gin.Context, resp *saml.LogoutResponse, binding, encoded string) {
	client, ok := m.clientManager.GetClientByEntityId(resp.Issuer.Value)
	if !ok {
		m.handleError(gc, security.NewExternalSamlAuthenticationError("cannot find idp metadata corresponding for logout response"))
		return
	}

	// perform validate, handle if success
	var e error
	if binding == saml.HTTPRedirectBinding {
		e = client.ValidateLogoutResponseRedirect(encoded)
	} else {
		e = client.ValidateLogoutResponseForm(encoded)
	}
	if e == nil {
		m.handleSuccess(gc)
		return
	}

	// handle error
	m.handleError(gc, e)
}

func (m *SPLogoutMiddleware) handleLogoutRequest(gc *gin.Context, req *saml.LogoutRequest, binding, encoded string) {
	// TODO Handle Logout Request for IDP-initiated SLO
}

func (m *SPLogoutMiddleware) resolveIdpClient(ctx context.Context) (*saml.ServiceProvider, error) {
	var entityId string
	auth := security.Get(ctx)
	if samlAuth, ok := auth.(*samlAssertionAuthentication); ok {
		entityId = samlAuth.Assertion.Issuer.Value
	}
	if sp, ok := m.clientManager.GetClientByEntityId(entityId); ok {
		return sp, nil
	}
	return nil, security.NewExternalSamlAuthenticationError("Unable to initiate SLO as SP: unknown SAML Issuer")
}

func (m *SPLogoutMiddleware) resolveNameId(ctx context.Context) (nameId, format string) {
	auth := security.Get(ctx)
	if samlAuth, ok := auth.(*samlAssertionAuthentication); ok &&
		samlAuth.Assertion != nil && samlAuth.Assertion.Subject != nil && samlAuth.Assertion.Subject.NameID != nil {
		nameId = samlAuth.Assertion.Subject.NameID.Value
		format = samlAuth.Assertion.Subject.NameID.Format
		//format = string(saml.EmailAddressNameIDFormat)
	}
	return
}

func (m *SPLogoutMiddleware) handleSuccess(ctx context.Context) {
	updateSLOState(ctx, func(current SLOState) SLOState {
		return current | SLOCompletedFully
	})
	gc := web.GinContext(ctx)
	auth := security.Get(ctx)
	m.successHandler.HandleAuthenticationSuccess(ctx, gc.Request, gc.Writer, auth, auth)
	if gc.Writer.Written() {
		gc.Abort()
	}
}

func (m *SPLogoutMiddleware) handleError(ctx context.Context, e error) {
	logger.WithContext(ctx).Infof("SAML Single Logout failed with error: %v", e)
	updateSLOState(ctx, func(current SLOState) SLOState {
		return current | SLOFailed
	})
	// We always let logout continues
	gc := web.GinContext(ctx)
	auth := security.Get(ctx)
	m.successHandler.HandleAuthenticationSuccess(ctx, gc.Request, gc.Writer, auth, auth)
	if gc.Writer.Written() {
		gc.Abort()
	}
}

/***********************
	Helper Funcs
 ***********************/

func currentAuthDetails(ctx context.Context) map[string]interface{} {
	auth := security.Get(ctx)
	switch m := auth.Details().(type) {
	case map[string]interface{}:
		return m
	default:
		return nil
	}
}

func currentSLOState(ctx context.Context) SLOState {
	details := currentAuthDetails(ctx)
	if details == nil {
		return 0
	}
	state, _ := details[kDetailsSLOState].(SLOState)
	return state
}

func updateSLOState(ctx context.Context, updater func(current SLOState) SLOState) {
	details := currentAuthDetails(ctx)
	if details == nil {
		return
	}
	state, _ := details[kDetailsSLOState].(SLOState)
	details[kDetailsSLOState] = updater(state)
}

/***********************
	Workaround
 ***********************/

type FixedLogoutRequest struct {
	saml.LogoutRequest
}

func MakeFixedLogoutRequest(sp *saml.ServiceProvider, idpURL, nameID string) (*FixedLogoutRequest, error) {
	req, e := sp.MakeLogoutRequest(idpURL, nameID)
	if e != nil {
		return nil, e
	}
	return &FixedLogoutRequest{*req}, nil
}

// Redirect this is copied from saml.AuthnRequest.Redirect.
// As of crewjam/saml 0.4.8, AuthnRequest's Redirect is fixed for properly setting Signature in redirect URL:
// 	https://github.com/crewjam/saml/pull/339
// However, saml.LogoutRequest.Redirect is not fixed. We need to do that by ourselves
// TODO revisit this part later when newer crewjam/saml library become available
func (req *FixedLogoutRequest) Redirect(relayState string, sp *saml.ServiceProvider) (*url.URL, error) {
	w := &bytes.Buffer{}
	w1 := base64.NewEncoder(base64.StdEncoding, w)
	w2, _ := flate.NewWriter(w1, 9)
	doc := etree.NewDocument()
	doc.SetRoot(req.Element())
	if _, err := doc.WriteTo(w2); err != nil {
		return nil, err
	}
	_ = w2.Close()
	_ = w1.Close()

	rv, _ := url.Parse(req.Destination)
	// We can't depend on Query().set() as order matters for signing
	query := rv.RawQuery
	if len(query) > 0 {
		query += "&SAMLRequest=" + url.QueryEscape(string(w.Bytes()))
	} else {
		query += "SAMLRequest=" + url.QueryEscape(string(w.Bytes()))
	}

	if relayState != "" {
		query += "&RelayState=" + relayState
	}
	if len(sp.SignatureMethod) > 0 {
		query += "&SigAlg=" + url.QueryEscape(sp.SignatureMethod)
		signingContext, err := saml.GetSigningContext(sp)

		if err != nil {
			return nil, err
		}

		sig, err := signingContext.SignString(query)
		if err != nil {
			return nil, err
		}
		query += "&Signature=" + url.QueryEscape(base64.StdEncoding.EncodeToString(sig))
	}

	rv.RawQuery = query

	return rv, nil
}