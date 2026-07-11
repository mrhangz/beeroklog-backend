package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"
	chimw "github.com/go-chi/chi/v5/middleware"
	"github.com/joho/godotenv"

	"github.com/mrhangz/beeroklog-backend/internal/config"
	"github.com/mrhangz/beeroklog-backend/internal/database"
	"github.com/mrhangz/beeroklog-backend/internal/handler"
	"github.com/mrhangz/beeroklog-backend/internal/middleware"
	"github.com/mrhangz/beeroklog-backend/internal/storage"
)

func main() {
	_ = godotenv.Load()

	cfg := config.Load()

	if cfg.JWTSecret == "" || cfg.JWTSecret == config.DefaultJWTSecret {
		if cfg.AppEnv == "production" {
			log.Fatal("JWT_SECRET must be set to a strong random value when APP_ENV=production")
		}
		log.Println("WARNING: using default JWT_SECRET; set JWT_SECRET before deploying")
	}

	db, err := database.Connect(context.Background(), cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("database: %v", err)
	}
	defer db.Close()

	if err := database.Migrate(cfg.DatabaseURL); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	s3, err := storage.NewS3(cfg)
	if err != nil {
		log.Fatalf("s3: %v", err)
	}

	authMW := middleware.NewAuth(cfg.JWTSecret)
	authH := handler.NewAuth(db, cfg)
	beerH := handler.NewBeer(db)
	reviewH := handler.NewReview(db, s3)
	feedH := handler.NewFeed(db)
	photoH := handler.NewPhoto(db, s3)

	r := chi.NewRouter()
	r.Use(chimw.Logger)
	r.Use(chimw.Recoverer)
	r.Use(chimw.RealIP)
	r.Use(chimw.RequestID)
	r.Use(middleware.CORS(cfg.CORSAllowedOrigins))

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("ok"))
	})

	r.Route("/api", func(r chi.Router) {
		// Read-only endpoints for the public web experience (no account
		// required to browse the community feed and beer pages).
		r.Route("/public", func(r chi.Router) {
			r.Get("/feed", feedH.Latest)
			r.Get("/feed/beer/{beerId}", feedH.ByBeer)
			r.Get("/beers", beerH.List)
			r.Get("/beers/{id}", beerH.Get)
		})

		r.Route("/auth", func(r chi.Router) {
			r.Post("/google", authH.Google)
			r.Post("/apple", authH.Apple)
			r.Post("/refresh", authH.Refresh)
			r.Group(func(r chi.Router) {
				r.Use(authMW.Verify)
				r.Get("/me", authH.Me)
				r.Patch("/me", authH.UpdateMe)
			})
		})

		r.Route("/beers", func(r chi.Router) {
			r.Use(authMW.Verify)
			r.Get("/", beerH.List)
			r.Get("/{id}", beerH.Get)
			r.Post("/", beerH.Create)
		})

		r.Route("/admin", func(r chi.Router) {
			r.Use(authMW.Verify)
			r.Use(middleware.RequireAdmin(authH))
			r.Get("/beers/duplicates", beerH.ListDuplicates)
			r.Post("/beers/merge", beerH.Merge)
			r.Post("/beers/dedupe-exact", beerH.DedupeExact)
			r.Patch("/beers/{id}", beerH.Update)
		})

		r.Route("/reviews", func(r chi.Router) {
			r.Use(authMW.Verify)
			r.Get("/", reviewH.List)
			r.Get("/{id}", reviewH.Get)
			r.Post("/", reviewH.Create)
			r.Put("/{id}", reviewH.Update)
			r.Delete("/{id}", reviewH.Delete)
		})

		r.Route("/feed", func(r chi.Router) {
			r.Use(authMW.Verify)
			r.Get("/", feedH.Latest)
			r.Get("/beer/{beerId}", feedH.ByBeer)
			r.Get("/user/{userId}", feedH.ByUser)
		})

		r.Route("/photos", func(r chi.Router) {
			r.Group(func(r chi.Router) {
				r.Use(authMW.Verify)
				r.Post("/upload", photoH.Upload)
			})
			// Reads are public so browser <img> tags (which cannot send
			// Authorization headers) can load review photos. Keys are
			// unguessable UUIDs and redirect to short-lived presigned URLs.
			r.Get("/{key}", photoH.Get)
			r.Get("/{key}/*", photoH.Get)
		})
	})

	srv := &http.Server{
		Addr:         ":" + cfg.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("listening on :%s", cfg.Port)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("shutdown: %v", err)
	}
	log.Println("server stopped")
}
