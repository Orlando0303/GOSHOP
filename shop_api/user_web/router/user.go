package router

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"shop_api/user_web/api"
	"shop_api/user_web/middlewares"
)

func InitUserRouter(Router *gin.RouterGroup) {
	UserRouter := Router.Group("user")
	zap.S().Info("配置用户相关的url")
	{
		UserRouter.GET("list", middlewares.JWTAuth(), middlewares.IsAdminAuth(), api.GetUserList)
		UserRouter.POST("pwd_login", api.PassWordLogin)
		UserRouter.POST("register", api.Register)
	}
}
