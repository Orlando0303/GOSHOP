package main

import (
	"fmt"
	"github.com/spf13/viper"
	"go.uber.org/zap"
	"shop_api/user_web/global"
	"shop_api/user_web/initialize"
	"shop_api/user_web/utils"
)

func main() {
	initialize.InitLogger()
	initialize.InitConfig()
	Router := initialize.Routers()
	_ = initialize.InitValidator("zh")
	initialize.InitSrvConn()

	viper.AutomaticEnv()
	//如果是本地开发环境端口号固定，线上环境启动获取端口号
	debug := viper.GetBool("SHOP_DEBUG")
	if !debug {
		port, err := utils.GetFreePort()
		if err == nil {
			global.ServerConfig.Port = port
		}
	}

	zap.S().Infof("启动web服务端口为：%d", global.ServerConfig.Port)
	if err := Router.Run(fmt.Sprintf(":%d", global.ServerConfig.Port)); err != nil {
		zap.S().Panic("启动失败: ", err.Error())
	}

}
