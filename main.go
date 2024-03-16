// @title Demo Template
// @version 1.0
// @description This is a  Demo Template API with Swagger documentation
// @host localhost:8080
// @BasePath /v1

package main

import (
	"context"
	
	
	config "gotemplate/config"
	"sync"

	"gotemplate/handler"
	"gotemplate/logger"
	repo "gotemplate/repo/postgres"
	r "gotemplate/route"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

var c config.Econfig
var log *logger.Logger
var validatorService *handler.ValidatorService



func LoggerInit() *logger.Logger {
	log := logger.New()
	return log
}

func validation() error {
	tagToNumber := handler.GetTagToNumberMap()
	errordbMap := handler.GetErrordbMap()
	var err error
	validatorService, err = handler.NewValidatorService(tagToNumber, errordbMap)
	if err != nil {
		return err
	}
	err = validatorService.RegisterCustomValidation("myvalidate", handler.Myvalidate, "must be equal to 10", "CST1")
	if err != nil {
		return err
	}

	err = validatorService.RegisterCustomValidation("hourvalidate", handler.HourValidate, "Pass correct hour", "CST2")
	if err != nil {
		return err
	}
	return nil
}
func main() {
	log = LoggerInit()
	c = config.Load(log)
	log.SetLevel(c.LogLevel())

	listenAddr := ":" + c.HttpPort()

	duration, err := time.ParseDuration(c.ShutDownTime())
	if err != nil {
		log.Error("Error in parsing duration", err.Error())
	}

	err = validation()
	if err != nil {
		log.Error("Error initialising validation:", err.Error())

	}
	shutdownctx, shutdowncancel := context.WithTimeout(context.Background(), duration)
	defer shutdowncancel()
	log.Debug("listenaddr:", listenAddr)

	ctx, contextCancel := context.WithTimeout(shutdownctx, 10*time.Second)
	defer contextCancel()

	db, err := repo.NewDB(ctx, c)
	if err != nil {
		log.Warn("error in db connection %s", err)
	}
	defer db.Close()
	log.Info("Successfully connected to the database %s", c.DBConnection())

	router, err := r.Routes(db, log, c, validatorService)

	if err != nil {
		log.Warn("Error initializing router %s", err.Error())
		os.Exit(1)
	}

	log.Info("Starting the HTTP server: %s", listenAddr)
	var wg sync.WaitGroup

	router.Use(func(c *gin.Context) {
		if c.Request.URL.Path != "/healthz" {
			wg.Add(1)
			defer wg.Done()
		}
		c.Next()
	})


	
	srv := &http.Server{
		Addr:    ":" + c.HttpPort(),
		Handler: router,
	}

	go func() {

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Info("listen: %s\n", err.Error())
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info("Received signal...%s", sig)
	handler.SetIsShuttingDown(true)

	go func() {
		if err := srv.Shutdown(shutdownctx); err != nil {
			log.Error("Server Shutdown error:", err.Error())
		}
	}()
	wg.Wait()
	db.Close()

	<-shutdownctx.Done()
	log.Info("Server exiting")

}
