// Command server is the QRforQA application: a sudoku-encoded questionnaire
// system serving its API and its frontend from a single binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"

	"qrforqa/internal/codec"
	"qrforqa/internal/config"
	"qrforqa/internal/database"
	"qrforqa/internal/handlers"
	"qrforqa/internal/middleware"
	"qrforqa/internal/services"
	"qrforqa/internal/vault"
	"qrforqa/internal/webui"
)

// Injected via -ldflags at build time.
var (
	version = "dev"
	commit  = "none"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "keygen":
			fmt.Println(config.GenerateSecret())
			return
		case "selftest":
			runSelfTest()
			return
		case "version":
			fmt.Printf("qrforqa %s (%s)\n", version, commit)
			return
		case "serve":
			// fall through
		default:
			fmt.Fprintf(os.Stderr, "unknown command %q\nusage: qrforqa [serve|keygen|selftest|version]\n", os.Args[1])
			os.Exit(2)
		}
	}
	if err := serve(); err != nil {
		log.Fatalf("fatal: %v", err)
	}
}

func runSelfTest() {
	if err := codec.SelfTest(); err != nil {
		fmt.Fprintf(os.Stderr, "codec self-test FAILED: %v\n", err)
		os.Exit(1)
	}
	if err := codec.DeepSelfTest(); err != nil {
		fmt.Fprintf(os.Stderr, "deep self-test FAILED: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("codec self-test OK\n  grids        %d (%s)\n  position sets %d (%s)\n",
		codec.GridCount, codec.GridsSHA256()[:16], codec.PosSetCount, codec.PosSetsSHA256()[:16])
}

func serve() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	if !cfg.Dev {
		gin.SetMode(gin.ReleaseMode)
	}

	// Refuse to start if the codec no longer derives the tables it was built
	// with. Every failure here would mean the browser and the server disagree,
	// which corrupts data silently rather than erroring at the point of the
	// bug — so it is a startup failure, not a warning.
	if err := codec.SelfTest(); err != nil {
		return fmt.Errorf("codec self-test failed, refusing to start: %w", err)
	}

	db, err := database.Open(cfg.DBPath, cfg.Dev)
	if err != nil {
		return err
	}
	v, err := vault.New(cfg.MasterKey)
	if err != nil {
		return err
	}
	svc := services.New(db, v, cfg)

	created, err := svc.EnsureAdmin()
	if err != nil {
		return fmt.Errorf("seed admin: %w", err)
	}
	if created {
		log.Printf("seeded administrator %q from QRFORQA_ADMIN_PASSWORD", cfg.AdminUser)
	}

	h := handlers.New(svc, cfg)
	router := gin.New()
	router.Use(gin.Recovery(), middleware.RequestLog())
	h.Register(router)
	webui.Register(router)

	srv := &http.Server{
		Addr:              cfg.Addr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       90 * time.Second,
		// Ciphertext bodies are ~4x plaintext and image uploads are the largest
		// legitimate request, so this is generous but bounded.
		WriteTimeout: 120 * time.Second,
	}

	errCh := make(chan error, 2)

	go func() {
		log.Printf("qrforqa %s (%s) listening on %s", version, commit, cfg.Addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("listen %s: %w", cfg.Addr, err)
		}
	}()

	// Optional second listener. The whole application is already reachable on
	// the primary port; this exists only for the separately-firewalled backend
	// port the deployment calls for.
	if cfg.AltAddr != "" {
		alt := &http.Server{
			Addr:              cfg.AltAddr,
			Handler:           router,
			ReadHeaderTimeout: 10 * time.Second,
			IdleTimeout:       90 * time.Second,
			WriteTimeout:      120 * time.Second,
		}
		go func() {
			log.Printf("additional listener on %s", cfg.AltAddr)
			if err := alt.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("listen %s: %w", cfg.AltAddr, err)
			}
		}()
	}

	// Uploads that were never attached to a submission are abandoned forms.
	go janitor(svc)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	select {
	case err := <-errCh:
		return err
	case sig := <-stop:
		log.Printf("received %s, shutting down", sig)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	log.Print("stopped cleanly")
	return nil
}

// janitor removes uploaded images that no response ever referenced.
func janitor(svc *services.Service) {
	ticker := time.NewTicker(6 * time.Hour)
	defer ticker.Stop()
	for {
		if n, err := svc.SweepOrphanImages(24 * time.Hour); err != nil {
			log.Printf("janitor: %v", err)
		} else if n > 0 {
			log.Printf("janitor: removed %d orphaned image(s)", n)
		}
		<-ticker.C
	}
}
