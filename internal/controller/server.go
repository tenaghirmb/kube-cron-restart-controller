package controller

import (
	"context"
	"encoding/json"
	"net/http"

	log "k8s.io/klog/v2"
)

type HTTPServer struct {
	cm      *CronManager
	address string
}

func NewHTTPServer(addr string, cm *CronManager) *HTTPServer {
	return &HTTPServer{
		address: addr,
		cm:      cm,
	}
}

func (s *HTTPServer) Start(ctx context.Context) error {
	mux := http.NewServeMux()

	mux.HandleFunc("/jobs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		tasks := s.cm.cronExecutor.GetTasks()
		if err := json.NewEncoder(w).Encode(tasks); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})

	srv := &http.Server{
		Addr:    s.address,
		Handler: mux,
	}

	go func() {
		log.Infof("starting server {\"name\": \"cron engine debug\", \"addr\": \"%s\"}", s.address)
		if err := srv.ListenAndServe(); err != http.ErrServerClosed {
			log.Errorf("HTTP server error: %v", err)
		}
	}()

	<-ctx.Done()
	return srv.Shutdown(context.Background())
}
