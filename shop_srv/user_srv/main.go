package main

import (
	"flag"
	"fmt"
	"github.com/hashicorp/consul/api"
	"github.com/nacos-group/nacos-sdk-go/inner/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"net"
	"os"
	"os/signal"
	"shop/user_srv/global"
	"shop/user_srv/handler"
	"shop/user_srv/initialize"
	"shop/user_srv/proto"
	"shop/user_srv/utils"
	"syscall"
)

func main() {
	IP := flag.String("ip", "0.0.0.0", "ip地址")
	Port := flag.Int("port", 0, "端口号")

	//初始化
	initialize.InitLogger()
	initialize.InitConfig()
	initialize.InitDB()
	zap.S().Info(global.ServerConfig)

	flag.Parse()
	zap.S().Info("ip: ", *IP)
	if *Port == 0 {
		*Port, _ = utils.GetFreePort()
	}

	zap.S().Info("port: ", *Port)

	server := grpc.NewServer()
	proto.RegisterUserServer(server, &handler.UserServer{})

	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", *IP, *Port))
	if err != nil {
		panic("failed to listen:" + err.Error())
	}

	grpc_health_v1.RegisterHealthServer(server, health.NewServer())

	//服务注册
	cfg := api.DefaultConfig()
	cfg.Address = fmt.Sprintf("%s:%d",
		global.ServerConfig.ConsulInfo.Host,
		global.ServerConfig.ConsulInfo.Port)

	client, err := api.NewClient(cfg)
	if err != nil {
		panic(err)
	}
	//生成对应的检查对象
	check := &api.AgentServiceCheck{
		GRPC:                           fmt.Sprintf("192.168.61.1:%d", *Port),
		Timeout:                        "10s",
		Interval:                       "10s",
		DeregisterCriticalServiceAfter: "15s",
		//GRPCUseTLS:                     false,
	}

	//生成注册对象
	registration := new(api.AgentServiceRegistration)
	registration.Name = global.ServerConfig.Name
	//使consul里面的serviceID唯一，但name可以重复
	serviceID, err := uuid.NewV4()
	if err != nil {
		panic(err)
	}
	registration.ID = serviceID.String()
	registration.Port = *Port
	registration.Tags = []string{"GOSHOP", "user", "srv"}
	registration.Address = "192.168.61.1"
	registration.Check = check

	err = client.Agent().ServiceRegister(registration)
	if err != nil {
		panic(err)
	}

	err = server.Serve(lis)
	if err != nil {
		panic("failed to start grpc:" + err.Error())
	}

	go func() {
		err = server.Serve(lis)
		if err != nil {
			panic("failed to start grpc:" + err.Error())
		}
	}()

	//接收终止信号
	quit := make(chan os.Signal)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	if err = client.Agent().ServiceDeregister(serviceID.String()); err != nil {
		zap.S().Info("注销失败")
	}
	zap.S().Info("注销成功")
}
