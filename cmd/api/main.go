package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/martin/network-service/internal/application"
	"github.com/martin/network-service/internal/config"
	"github.com/martin/network-service/internal/driver"
	"github.com/martin/network-service/internal/registry"
	"github.com/martin/network-service/internal/repository"
	"github.com/martin/network-service/internal/service"
	transportHTTP "github.com/martin/network-service/internal/transport/http"
	natsTransport "github.com/martin/network-service/internal/transport/nats"
	"github.com/martin/network-service/internal/utils"
	"github.com/martin/network-service/pkg/database"
	"github.com/martin/network-service/pkg/logger"
	"github.com/nats-io/nats.go"
	"go.uber.org/zap"
)

func main() {
	// 1. Load Config
	cfg, err := config.Load()
	if err != nil {
		panic(fmt.Sprintf("Failed to load config: %v", err))
	}

	// 2. Init Logger
	logger.Init(cfg.Profile)
	defer zap.L().Sync()

	zap.L().Info("Starting Network Service",
		zap.String("service", cfg.Server.ServiceName),
		zap.String("profile", cfg.Profile),
	)

 

	// 3. Connect to NATS
	var nc *nats.Conn
	opts := []nats.Option{}
	if cfg.NATS.User != "" && cfg.NATS.Password != "" {
		opts = append(opts, nats.UserInfo(cfg.NATS.User, cfg.NATS.Password))
	}

	parsedURL, err := url.Parse(cfg.NATS.URL)
	if err != nil {
		zap.L().Fatal("Failed to parse NATS URL", zap.Error(err), zap.String("url", cfg.NATS.URL))
	}
	host := parsedURL.Host
	if !strings.Contains(host, ":") {
		host = host + ":4222"
	}

	for i := 0; i < cfg.NATS.MaxRetries; i++ {
		zap.L().Info("Checking NATS reachability...", zap.String("host", host), zap.Int("attempt", i+1))
		conn, err := net.DialTimeout("tcp", host, cfg.NATS.DialTimeout)
		if err == nil {
			conn.Close()
			break
		}
		zap.L().Warn("NATS not reachable yet", zap.String("host", host), zap.Int("attempt", i+1), zap.Error(err))
		if i == cfg.NATS.MaxRetries-1 {
			zap.L().Fatal("NATS unreachable after retries", zap.String("host", host))
		}
		time.Sleep(cfg.NATS.RetryInterval)
	}

	nc, err = nats.Connect(cfg.NATS.URL, opts...)
	if err != nil {
		zap.L().Fatal("Failed to connect to NATS", zap.Error(err), zap.String("url", cfg.NATS.URL))
	}
	defer nc.Close()
	zap.L().Info("Connected to NATS", zap.String("url", cfg.NATS.URL))

	// 4. Connect to PostgreSQL
	dbConfig := database.Config{
		Host:            cfg.DB.Host,
		Port:            cfg.DB.Port,
		User:            cfg.DB.User,
		Password:        cfg.DB.Password,
		Database:        cfg.DB.Database,
		SSLMode:         cfg.DB.SSLMode,
		MaxOpenConns:    cfg.DB.MaxOpenConns,
		MaxIdleConns:    cfg.DB.MaxIdleConns,
		ConnMaxLifetime: cfg.DB.ConnMaxLifetime,
		ConnMaxIdleTime: cfg.DB.ConnMaxIdleTime,
	}

	var db *database.DB
	for i := 0; i < cfg.DB.MaxRetries; i++ {
		zap.L().Info("Attempting to connect to PostgreSQL...", zap.Int("attempt", i+1))
		db, err = database.Connect(dbConfig)
		if err == nil {
			break
		}
		zap.L().Warn("Failed to connect to PostgreSQL", zap.Int("attempt", i+1), zap.Error(err))
		if i < cfg.DB.MaxRetries-1 {
			time.Sleep(cfg.DB.RetryInterval)
		}
	}

	if err != nil {
		zap.L().Warn("Could not connect to PostgreSQL after retries, falling back to SQLite")
		sqlitePath := getEnv("SQLITE_PATH", "network.db")
		db, err = database.ConnectSQLite(sqlitePath)
		if err != nil {
			zap.L().Fatal("Failed to connect to SQLite fallback", zap.Error(err))
		}
		zap.L().Info("Connected to SQLite fallback", zap.String("path", sqlitePath))
	} else {
		zap.L().Info("Successfully connected to PostgreSQL")
	}
	defer db.Close()

	migrationsPath := getEnv("MIGRATIONS_PATH", "internal/infrastructure/migrations")
	if err := db.RunMigrations(migrationsPath); err != nil {
		zap.L().Fatal("Failed to run database migrations", zap.Error(err))
	}
	zap.L().Info("Database migrations completed")

	// 5. Setup Repositories
	vpcRepo := repository.NewVPCRepository(db.DB)
	subnetRepo := repository.NewSubnetRepository(db.DB)
	igwRepo := repository.NewInternetGatewayRepository(db.DB)
	rtRepo := repository.NewRouteTableRepository(db.DB)
	routeRepo := repository.NewRouteRepository(db.DB)
	sgRepo := repository.NewSecurityGroupRepository(db.DB)
	cidrRepo := repository.NewCIDRRepository(db.DB)
	eipRepo := repository.NewElasticIPRepository(db.DB)
	resourceRepo := repository.NewResourceVPCRepository(db.DB)
	netAssignRepo := repository.NewResourceNetworkRepository(db.DB)
	rdsPortRepo := repository.NewRDSPortRepository(db.DB)
	computeReg := registry.NewComputeRegistry()

	// 5.5 Setup Drivers
	bridgeDriver := driver.NewBridgeDriver()
	iptablesDriver := driver.NewIptablesDriver()
	routingDriver := driver.NewRoutingDriver()
	dockerNetworkDriver := driver.NewDockerNetworkDriver()

	// 6. Setup Services
	netService := service.NewNetworkService(
		db.DB, vpcRepo, subnetRepo, igwRepo, rtRepo, routeRepo, sgRepo, cidrRepo, eipRepo,
		bridgeDriver, iptablesDriver, routingDriver, dockerNetworkDriver, resourceRepo, netAssignRepo, rdsPortRepo, computeReg,
		cfg.Server.PublicInterface,
	)

	// Startup Reconciliation
	go func() {
		zap.L().Info("Triggering background VPC reconciliation")
		if err := netService.ReconcileVPCs(context.Background()); err != nil {
			zap.L().Error("Startup reconciliation failed", zap.Error(err))
		}
		zap.L().Info("Triggering background RDS port reconciliation")
		if err := netService.ReconcileRDSPorts(context.Background()); err != nil {
			zap.L().Error("RDS port reconciliation failed", zap.Error(err))
		}
	}()

	// Compute Health Monitor
	go func() {
		zap.L().Info("Starting background compute health monitor")
		if err := netService.MonitorComputeHealth(context.Background()); err != nil {
			zap.L().Error("Compute health monitor stopped", zap.Error(err))
		}
	}()

	// 7. Register with Eureka
	for i := 0; i < 3; i++ {
		if err := registerWithEureka(cfg.Eureka); err != nil {
			zap.L().Warn("Eureka registration attempt failed",
				zap.Int("attempt", i+1),
				zap.Error(err),
			)
			time.Sleep(5 * time.Second)
		} else {
			break
		}
	}

	// Start Eureka heartbeat
	go sendHeartbeat(cfg.Eureka)

	// 8. Setup HTTP Handlers
	networkHandler := transportHTTP.NewNetworkHandler()
	docsHandler := transportHTTP.NewDocsHandler(
		application.NewDocsService(utils.GetEnv("DOCS_PATH", "./docs")),
	)

	// 9. Setup Gin Router
	r := gin.New()
	r.Use(gin.Recovery())
	transportHTTP.SetupRoutes(r, networkHandler, docsHandler)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: r,
	}

	// 10. Setup NATS Subscriber
	sub := natsTransport.NewSubscriber(nc, netService, cfg.NATS.SubjectPrefix)
	if err := sub.Subscribe(); err != nil {
		zap.L().Fatal("could not subscribe to NATS", zap.Error(err))
	}

	// 11. Start HTTP Server
	go func() {
		zap.L().Info("HTTP Server listening", zap.Int("port", cfg.Server.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	// 12. Graceful Shutdown
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	zap.L().Info("Shutting down Network Service...")

	// Deregister from Eureka before exit
	if err := deregisterFromEureka(cfg.Eureka); err != nil {
		zap.L().Warn("Failed to deregister from Eureka", zap.Error(err))
	}

	ctx, cancel := context.WithTimeout(context.Background(), cfg.Server.ShutdownTimeout)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Error("HTTP server forced to shutdown", zap.Error(err))
	}

	zap.L().Info("Network Service exited gracefully")
}

// registerWithEureka registers this service instance with the Eureka server.
func registerWithEureka(cfg config.EurekaConfig) error {
	instance := map[string]interface{}{
		"instance": map[string]interface{}{
			"instanceId": cfg.InstanceID,
			"hostName":   cfg.HostName,
			"app":        cfg.AppName,
			"ipAddr":     cfg.IPAddr,
			"vipAddress": cfg.VipAddress,
			"status":     "UP",
			"port": map[string]interface{}{
				"$":        cfg.Port,
				"@enabled": "true",
			},
			"dataCenterInfo": map[string]interface{}{
				"@class": "com.netflix.appinfo.InstanceInfo$DefaultDataCenterInfo",
				"name":   "MyOwn",
			},
			"healthCheckUrl": fmt.Sprintf("http://%s:%d/health", cfg.HostName, cfg.Port),
			"statusPageUrl":  fmt.Sprintf("http://%s:%d/health", cfg.HostName, cfg.Port),
			"homePageUrl":    fmt.Sprintf("http://%s:%d/", cfg.HostName, cfg.Port),
		},
	}

	jsonData, err := json.Marshal(instance)
	if err != nil {
		return fmt.Errorf("failed to marshal Eureka registration data: %w", err)
	}

	eurekaURL := fmt.Sprintf("%s/apps/%s", cfg.ServerURL, cfg.AppName)
	req, err := http.NewRequest(http.MethodPost, eurekaURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return fmt.Errorf("failed to create registration request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to register with Eureka: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("eureka registration failed with status %d", resp.StatusCode)
	}

	zap.L().Info("Successfully registered with Eureka", zap.String("url", eurekaURL))
	return nil
}

// sendHeartbeat sends periodic PUT heartbeats to Eureka to keep the instance alive.
func sendHeartbeat(cfg config.EurekaConfig) {
	ticker := time.NewTicker(cfg.HeartbeatInterval)
	defer ticker.Stop()

	eurekaURL := fmt.Sprintf("%s/apps/%s/%s", cfg.ServerURL, cfg.AppName, cfg.InstanceID)
	client := &http.Client{Timeout: 5 * time.Second}

	for range ticker.C {
		req, err := http.NewRequest(http.MethodPut, eurekaURL, nil)
		if err != nil {
			zap.L().Error("Failed to create heartbeat request", zap.Error(err))
			continue
		}

		resp, err := client.Do(req)
		if err != nil {
			zap.L().Error("Failed to send heartbeat to Eureka", zap.Error(err))
			continue
		}

		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			zap.L().Warn("Heartbeat failed", zap.Int("status", resp.StatusCode))
		} else {
			zap.L().Debug("Heartbeat sent successfully to Eureka")
		}
		resp.Body.Close()
	}
}

// deregisterFromEureka removes this instance from Eureka on graceful shutdown.
func deregisterFromEureka(cfg config.EurekaConfig) error {
	eurekaURL := fmt.Sprintf("%s/apps/%s/%s", cfg.ServerURL, cfg.AppName, cfg.InstanceID)
	req, err := http.NewRequest(http.MethodDelete, eurekaURL, nil)
	if err != nil {
		return fmt.Errorf("failed to create deregistration request: %w", err)
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to deregister from Eureka: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("deregistration failed with status %d", resp.StatusCode)
	}

	zap.L().Info("Successfully deregistered from Eureka")
	return nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}