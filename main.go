package main

import (
	"context"
	"fmt"
	"go-web-app/controller"
	"go-web-app/dao/mongodb"
	"go-web-app/dao/mysql"
	"go-web-app/dao/redis"
	"go-web-app/logger"
	"go-web-app/pkg/snowflake"
	"go-web-app/routes"
	"go-web-app/settings"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func main() {
	// 1. load config files
	if err := settings.Init(); err != nil {
		fmt.Printf("Init settings failed, err:%v\n", err)
		return
	}
	// 2. init uber/zap logger
	if err := logger.Init(settings.Conf.LogConfig, settings.Conf.Mode); err != nil {
		fmt.Printf("Init logger failed, err:%v\n", err)
		return
	}
	defer zap.L().Sync()
	zap.L().Debug("logger init success")
	// 3. init mysql
	if err := mysql.Init(settings.Conf.MySQLConfig); err != nil {
		fmt.Printf("Init mysql failed, err:%v\n", err)
		return
	}
	defer mysql.Close()
	// 4. init redis
	if err := redis.Init(settings.Conf.RedisConfig); err != nil {
		fmt.Printf("Init redis failed, err:%v\n", err)
		return
	}
	defer redis.Close()

	// 5. init mongodb
	if err := mongodb.Init(settings.Conf.MongodbConfig); err != nil {
		fmt.Printf("Init mongodb failed, err:%v\n", err)
		return
	}
	defer mongodb.Close()

	// 5. init snowflake
	if err := snowflake.Init(settings.Conf.StartTime, settings.Conf.MachineID); err != nil {
		fmt.Printf("Init snowflake failed, err:%v\n", err)
		return
	}

	if err := controller.InitValidator("en"); err != nil {
		fmt.Printf("init validator trans failed, err:%v\n", err)
		return
	}

	// 6. register routers
	r := routes.Setup(settings.Conf.Mode)
	// 7. setup shutdown gracefully
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", settings.Conf.Port),
		Handler: r,
	}

	go func() {
		// use another goroutine
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// 等待中断信号来优雅地关闭服务器，为关闭服务器操作设置一个5秒的超时
	// wait for kill signal
	quit := make(chan os.Signal, 1) // 创建一个接收信号的通道
	// kill 默认会发送 syscall.SIGTERM 信号
	// kill -2 发送 syscall.SIGINT 信号，我们常用的Ctrl+C就是触发系统SIGINT信号
	// kill -9 发送 syscall.SIGKILL 信号，但是不能被捕获，所以不需要添加它
	// signal.Notify把收到的 syscall.SIGINT或syscall.SIGTERM 信号转发给quit
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM) // 此处不会阻塞
	<-quit                                               // 阻塞在此，当接收到上述两种信号时才会往下执行
	zap.L().Info("Shutdown Server ...")
	// 创建一个5秒超时的context
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 5秒内优雅关闭服务（将未处理完的请求处理完再关闭服务），超过5秒就超时退出
	// shutdown gracefully in 5 seconds
	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Fatal("Server Shutdown: ", zap.Error(err))
	}

	zap.L().Info("Server exiting")
}
