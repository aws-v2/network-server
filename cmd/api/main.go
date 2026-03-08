package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/martin/network-service/internal/config"
	"github.com/martin/network-service/internal/driver"
	"github.com/martin/network-service/internal/registry"
	"github.com/martin/network-service/internal/repository"
	"github.com/martin/network-service/internal/service"
	httpTransport "github.com/martin/network-service/internal/transport/http"
	natsTransport "github.com/martin/network-service/internal/transport/nats"
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

	zap.L().Info("Starting Network Service", zap.String("service", cfg.Server.ServiceName))

	// 3. Connect to NATS
	var nc *nats.Conn
	opts := []nats.Option{}
	if cfg.NATS.User != "" && cfg.NATS.Password != "" {
		opts = append(opts, nats.UserInfo(cfg.NATS.User, cfg.NATS.Password))
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
	maxRetries := 4

	for i := 0; i < maxRetries; i++ {
		zap.L().Info("Attempting to connect to PostgreSQL...", zap.Int("attempt", i+1))
		db, err = database.Connect(dbConfig)
		if err == nil {
			break
		}
		zap.L().Warn("Failed to connect to PostgreSQL", zap.Int("attempt", i+1), zap.Error(err))
		if i < maxRetries-1 {
			time.Sleep(2 * time.Second)
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

	// 7. Setup HTTP Handler
	mux := http.NewServeMux()
	h := httpTransport.NewHandler()
	httpTransport.MapRoutes(mux, h)

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: mux,
	}

	// 8. Setup NATS Subscriber
	sub := natsTransport.NewSubscriber(nc, netService)
	if err := sub.Subscribe(); err != nil {
		zap.L().Fatal("could not subscribe to NATS", zap.Error(err))
	}

	// 9. Graceful Shutdown
	go func() {
		zap.L().Info("HTTP Server listening", zap.Int("port", cfg.Server.HTTPPort))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			zap.L().Fatal("HTTP server failed", zap.Error(err))
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt, syscall.SIGTERM)
	<-quit

	zap.L().Info("Shutting down Network Service...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := srv.Shutdown(ctx); err != nil {
		zap.L().Error("HTTP server forced to shutdown", zap.Error(err))
	}

	zap.L().Info("Network Service exited gracefully")
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
