package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	conf "github.com/Salon-1C/record-service/internal/config"
	httpapi "github.com/Salon-1C/record-service/internal/http"
	"github.com/Salon-1C/record-service/internal/recordings"
	mysqlstore "github.com/Salon-1C/record-service/internal/storage/mysql"
	objectstore "github.com/Salon-1C/record-service/internal/storage/object"
)

func main() {
	cfg, err := conf.Load()
	if err != nil {
		log.Fatalf("config error: %v", err)
	}

	dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true", cfg.DBUser, cfg.DBPassword, cfg.DBHost, cfg.DBPort, cfg.DBName)
	repo, err := mysqlstore.New(dsn)
	if err != nil {
		log.Fatalf("mysql init error: %v", err)
	}
	obj, err := objectstore.New(context.Background(), objectstore.Config{
		Region:          cfg.S3Region,
		Bucket:          cfg.S3Bucket,
		Endpoint:        cfg.S3Endpoint,
		AccessKeyID:     cfg.S3AccessKeyID,
		SecretAccessKey: cfg.S3SecretAccessKey,
		UsePathStyle:    cfg.S3UsePathStyle,
		PublicBaseURL:   cfg.S3PublicBaseURL,
	})
	if err != nil {
		log.Fatalf("object storage init error: %v", err)
	}

	svc := recordings.NewService(repo, obj, cfg.RecordingsDir, cfg.StableWindow, cfg.MaxUploadFileBytes)
	router := httpapi.NewHandler(svc)

	go startReconcileLoop(svc, cfg.ScanInterval)

	log.Printf("record-service listening on %s", cfg.HTTPAddr)
	if err := http.ListenAndServe(cfg.HTTPAddr, router.Routes()); err != nil {
		log.Fatalf("http server error: %v", err)
	}
}

func startReconcileLoop(svc *recordings.Service, interval time.Duration) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		ctx, cancel := context.WithTimeout(context.Background(), interval)
		_, err := svc.Reconcile(ctx)
		cancel()
		if err != nil {
			log.Printf("reconcile loop error: %v", err)
		}
		<-t.C
	}
}
