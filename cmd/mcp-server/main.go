package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/dshmyz/moonlight-box/internal/config"
	"github.com/dshmyz/moonlight-box/internal/database"
	"github.com/dshmyz/moonlight-box/internal/mcp"
	"github.com/dshmyz/moonlight-box/internal/repository"
	"github.com/dshmyz/moonlight-box/internal/service"
	mcpserver "github.com/mark3labs/mcp-go/server"
)

func main() {
	transport := flag.String("transport", "", "传输方式: stdio (默认) 或 sse")
	port := flag.Int("port", 8081, "SSE 模式监听端口")
	configPath := flag.String("config", "", "配置文件路径")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("加载配置失败: %v", err)
	}

	// 初始化数据库
	if err := database.Initialize(cfg); err != nil {
		log.Fatalf("初始化数据库失败: %v", err)
	}
	db := database.GetDB()

	// 初始化服务层
	repoRepo := repository.NewRepositoryRepository(db)
	groupRepo := repository.NewGroupRepository(db)
	auditSvc := service.NewAuditService()
	blockRuleRepo := repository.NewBlockRuleRepository(db)
	blockSvc := service.NewBlockRuleService(blockRuleRepo, auditSvc)
	repoSvc := service.NewRepositoryService(repoRepo, groupRepo, db)
	roleRepo := repository.NewRoleRepository(db)
	permCacheSvc := service.NewPermissionCacheService(roleRepo, 5*time.Minute)

	mcpSrv := mcp.NewMCPServer(cfg, db, repoSvc, auditSvc, blockSvc, permCacheSvc)

	transportMode := "sse"
	if *transport != "" {
		transportMode = *transport
	}

	switch transportMode {
	case "sse":
		addr := fmt.Sprintf(":%d", *port)
		sseServer := mcpserver.NewSSEServer(mcpSrv.GetMCPServer())
		log.Printf("Moonlight Box MCP Server (SSE) listening on %s", addr)
		if err := sseServer.Start(addr); err != nil {
			log.Fatalf("SSE server error: %v", err)
		}
	default:
		log.SetOutput(os.Stderr)
		log.Println("Moonlight Box MCP Server (stdio) started")
		if err := mcpserver.ServeStdio(mcpSrv.GetMCPServer()); err != nil {
			log.Fatalf("stdio server error: %v", err)
		}
	}
}
