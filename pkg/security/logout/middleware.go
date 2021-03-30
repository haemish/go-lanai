package logout

import (
	"cto-github.cisco.com/NFV-BU/go-lanai/pkg/security"
	"github.com/gin-gonic/gin"
)

//goland:noinspection GoNameStartsWithPackageName
type LogoutMiddleware struct {
	successHandler security.AuthenticationSuccessHandler
	logoutHandlers []LogoutHandler
}

func NewLogoutMiddleware(successHandler security.AuthenticationSuccessHandler, logoutHandlers ...LogoutHandler) *LogoutMiddleware {
	return &LogoutMiddleware{
		successHandler: successHandler,
		logoutHandlers: logoutHandlers,
	}
}

func (mw *LogoutMiddleware) LogoutHandlerFunc() gin.HandlerFunc {
	return func(ctx *gin.Context) {
		before := security.Get(ctx)
		for _, handler := range mw.logoutHandlers {
			handler.HandleLogout(ctx, ctx.Request, ctx.Writer, before)
		}
		mw.handleSuccess(ctx, before)
	}
}

func (mw *LogoutMiddleware) handleSuccess(c *gin.Context, before security.Authentication) {
	mw.successHandler.HandleAuthenticationSuccess(c, c.Request, c.Writer, before, security.Get(c))
	if c.Writer.Written() {
		c.Abort()
	}
}